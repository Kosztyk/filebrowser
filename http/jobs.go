package fbhttp

// All asynchronous file actions share the upload journal. Work is admitted before
// returning 202, claimed by fixed workers, and reauthorized on every restart.
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/filebrowser/filebrowser/v2/runner"
	"github.com/gorilla/mux"
	"github.com/spf13/afero"
	bolt "go.etcd.io/bbolt"
)

var jobsBucket = []byte("jobs-v1")

type Job struct {
	Pending   *publicationIntent           `json:"-"`
	Published map[string]files.FileVersion `json:"-"`
	Draft     json.RawMessage              `json:"-"`
	ID        string                       `json:"id"`
	Kind      string                       `json:"kind"`
	Owner     uint                         `json:"owner"`
	Scope     string                       `json:"-"`
	State     string                       `json:"state"`
	Phase     string                       `json:"phase"`
	Message   string                       `json:"message,omitempty"`
	Completed int64                        `json:"completed"`
	Total     int64                        `json:"total"`
	Created   time.Time                    `json:"created"`
	Updated   time.Time                    `json:"updated"`
	Retryable bool                         `json:"retryable"`
	Result    json.RawMessage              `json:"result,omitempty"`
	Payload   json.RawMessage              `json:"-"`
	Key       string                       `json:"-"`
	Upstream  *convertXSubmitResponse      `json:"-"`
	Versions  map[string]files.FileVersion `json:"-"`
}

// diskJob keeps private execution data out of API responses.
type diskJob struct {
	Pending   *publicationIntent
	Published map[string]files.FileVersion
	Draft     json.RawMessage
	Job
	Scope    string
	Payload  json.RawMessage
	Key      string
	Upstream *convertXSubmitResponse
	Versions map[string]files.FileVersion
}

func encodeJob(j Job) ([]byte, error) {
	return json.Marshal(diskJob{Job: j, Scope: j.Scope, Payload: j.Payload, Key: j.Key, Upstream: j.Upstream, Versions: j.Versions, Pending: j.Pending, Published: j.Published, Draft: j.Draft})
}
func decodeJob(b []byte) (Job, error) {
	var d diskJob
	e := json.Unmarshal(b, &d)
	j := d.Job
	j.Pending = d.Pending
	j.Published = d.Published
	j.Draft = d.Draft
	j.Scope = d.Scope
	j.Payload = d.Payload
	j.Key = d.Key
	j.Upstream = d.Upstream
	j.Versions = d.Versions
	return j, e
}

type featureRuntime struct {
	artifacts sync.Mutex
	indexing  sync.Mutex
	office    sync.Mutex
	m         *UploadManager
	mu        sync.Mutex
	running   map[string]context.CancelFunc
	wake      chan struct{}
}

func newFeatureRuntime(m *UploadManager, start ...bool) (*featureRuntime, error) {
	f := &featureRuntime{m: m, running: map[string]context.CancelFunc{}, wake: make(chan struct{}, 1)}
	e := m.journal.Update(func(tx *bolt.Tx) error {
		for _, name := range [][]byte{jobsBucket, []byte("sessions-v1"), []byte("office-locks-v1"), []byte("diagnostics-v1")} {
			if _, e := tx.CreateBucketIfNotExists(name); e != nil {
				return e
			}
		}
		b := tx.Bucket(jobsBucket)
		return b.ForEach(func(k, v []byte) error {
			j, e := decodeJob(v)
			if e != nil {
				return e
			}
			if j.State == "running" {
				if (j.Phase == "submitting" && j.Upstream == nil) || (j.Phase == "publishing" && j.Pending == nil) {
					j.State = "needs_review"
					j.Message = "Upstream submission outcome is unknown. Check the external service before starting another operation."
				} else {
					j.State = "queued"
				}
				j.Updated = time.Now()
				v, e = encodeJob(j)
				if e != nil {
					return e
				}
				return b.Put(k, v)
			}
			return nil
		})
	})
	if e != nil {
		return nil, e
	}
	for _, j := range f.list() {
		if j.Pending != nil && !jobTerminal(j.State) {
			if e = f.reconcilePublication(j); e != nil {
				_ = f.change(j.ID, func(j *Job) error {
					j.State = "needs_review"
					j.Message = "Publication recovery found changed files; review the destination"
					return nil
				})
			}
		}
	}
	if e = f.restoreOfficeLocks(); e != nil {
		return nil, e
	}
	set, e := m.store.Settings.Get()
	if e != nil {
		return nil, e
	}
	for i := 0; (len(start) == 0 || start[0]) && i < set.Work.Effective().Workers; i++ {
		m.workers.Add(1)
		go func() { defer m.workers.Done(); f.worker() }()
	}
	if len(start) == 0 || start[0] {
		m.workers.Add(1)
		go func() { defer m.workers.Done(); f.worker("index") }()
	}
	return f, nil
}
func (f *featureRuntime) get(id string) (Job, error) {
	var j Job
	e := f.m.journal.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(jobsBucket).Get([]byte(id))
		if b == nil {
			return os.ErrNotExist
		}
		var e error
		j, e = decodeJob(b)
		return e
	})
	return j, e
}
func (f *featureRuntime) change(id string, fn func(*Job) error) error {
	return f.m.journal.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(jobsBucket)
		v := b.Get([]byte(id))
		if v == nil {
			return os.ErrNotExist
		}
		j, e := decodeJob(v)
		if e != nil {
			return e
		}
		if e = fn(&j); e != nil {
			return e
		}
		j.Updated = time.Now()
		v, e = encodeJob(j)
		if e != nil {
			return e
		}
		return b.Put([]byte(id), v)
	})
}
func (f *featureRuntime) list() (out []Job) {
	_ = f.m.journal.View(func(tx *bolt.Tx) error {
		return tx.Bucket(jobsBucket).ForEach(func(_, v []byte) error {
			j, e := decodeJob(v)
			if e == nil {
				out = append(out, j)
			}
			return e
		})
	})
	return
}
func (f *featureRuntime) submit(d *data, kind string, payload any, paths []string) (Job, error) {
	raw, e := json.Marshal(payload)
	if e != nil {
		return Job{}, e
	}
	scope := afero.FullBaseFsPath(files.BasePath(d.user.Fs), "/")
	sum := sha256.Sum256(append([]byte(fmt.Sprintf("%d:%s:%s:%s:", d.user.ID, scope, kind, d.idempotency)), raw...))
	key := hex.EncodeToString(sum[:])
	f.mu.Lock()
	defer f.mu.Unlock()
	all := f.list()
	active := 0
	for _, old := range all {
		if d.idempotency != "" && old.Key == key && old.State == "succeeded" {
			return old, nil
		}
	}
	for _, old := range all {
		if old.State == "queued" || old.State == "running" || old.State == "needs_review" {
			active++
			if old.Key == key {
				return old, nil
			}
		}
	}
	if active >= d.settings.Work.Effective().MaxJobs {
		return Job{}, errors.New("background queue capacity reached")
	}
	j := Job{ID: newFileActionJobID(), Kind: kind, Owner: d.user.ID, Scope: scope, State: "queued", Phase: "queued", Payload: raw, Key: key, Created: time.Now(), Updated: time.Now(), Versions: map[string]files.FileVersion{}}
	for _, p := range paths {
		v, e := files.Version(d.user.Fs, p)
		if e != nil {
			return Job{}, e
		}
		j.Versions[p] = v
	}
	e = f.m.journal.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(jobsBucket)
		c := b.Cursor()
		count := b.Stats().KeyN
		for k, v := c.First(); k != nil; k, v = c.Next() {
			old, e := decodeJob(v)
			if e != nil {
				return e
			}
			if jobTerminal(old.State) && (old.Updated.Before(time.Now().Add(-7*24*time.Hour)) || count >= 10000) {
				if e = c.Delete(); e != nil {
					return e
				}
				count--
			}
		}
		if count >= 10000 {
			return errors.New("job history capacity reached")
		}
		v, e := encodeJob(j)
		if e != nil {
			return e
		}
		return b.Put([]byte(j.ID), v)
	})
	if e == nil {
		select {
		case f.wake <- struct{}{}:
		default:
		}
	}
	return j, e
}
func jobTerminal(s string) bool {
	return s == "succeeded" || s == "failed" || s == "cancelled" || s == "closed"
}
func (f *featureRuntime) jobData(j Job) (*data, error) {
	set, e := f.m.store.Settings.Get()
	if e != nil {
		return nil, e
	}
	u, e := f.m.store.Users.Get(f.m.server.Root, f.m.server.FollowExternalSymlinks, j.Owner)
	if e != nil {
		return nil, e
	}
	if filepath.Clean(afero.FullBaseFsPath(files.BasePath(u.Fs), "/")) != filepath.Clean(j.Scope) {
		return nil, os.ErrPermission
	}
	return &data{Runner: &runner.Runner{Enabled: f.m.server.EnableExec, Settings: set}, settings: set, server: f.m.server, store: f.m.store, user: u, features: f}, nil
}
func (f *featureRuntime) worker(kinds ...string) {
	indexWorker := len(kinds) > 0 && kinds[0] == "index"
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-f.m.ctx.Done():
			return
		case <-f.wake:
		case <-t.C:
		}
		for {
			if f.m.ctx.Err() != nil {
				return
			}
			var chosen Job
			f.mu.Lock()
			for _, j := range f.list() {
				if j.State == "queued" && ((j.Kind == "index") == indexWorker) {
					chosen = j
					break
				}
			}
			if chosen.ID != "" {
				if e := f.change(chosen.ID, func(j *Job) error { j.State = "running"; return nil }); e != nil {
					chosen = Job{}
				}
			}
			f.mu.Unlock()
			if chosen.ID == "" {
				break
			}
			f.execute(chosen)
		}
	}
}
func (f *featureRuntime) execute(j Job) {
	ctx, cancel := context.WithTimeout(f.m.ctx, 24*time.Hour)
	f.mu.Lock()
	f.running[j.ID] = cancel
	f.mu.Unlock()
	defer func() { cancel(); f.mu.Lock(); delete(f.running, j.ID); f.mu.Unlock() }()
	var result any
	var e error
	func() {
		defer func() {
			if recover() != nil {
				e = errors.New("job stopped unexpectedly; inspect server logs before retrying")
			}
		}()
		d, err := f.jobData(j)
		if err != nil {
			e = err
			return
		}
		for p, v := range j.Versions {
			if !d.CheckRules(p) {
				e = os.ErrPermission
				return
			}
			current, err := files.Version(d.user.Fs, p)
			if err != nil || current != v {
				e = files.ErrWriteConflict
				return
			}
		}
		result, e = f.run(ctx, &j, d)
	}()
	if f.m.ctx.Err() != nil {
		_ = f.change(j.ID, func(current *Job) error {
			if (current.Phase == "submitting" && current.Upstream == nil) || (current.Phase == "publishing" && current.Pending == nil) {
				current.State = "needs_review"
				current.Message = "Submission outcome unknown; check upstream before retrying"
			} else {
				current.State = "queued"
			}
			return nil
		})
		return
	}
	message := ""
	if e != nil {
		message = e.Error()
		if d, err := f.jobData(j); err == nil {
			message = redactDiagnostic(message, d)
		}
	}
	_ = f.change(j.ID, func(current *Job) error {
		if current.State == "cancelled" || current.State == "needs_review" {
			return nil
		}
		if e != nil {
			current.State = "failed"
			current.Message = "Operation failed: " + message
			current.Retryable = (current.Phase != "submitting" || current.Upstream != nil) && current.Pending == nil && current.Phase != "publishing" && !errors.Is(e, files.ErrWriteConflict) && !errors.Is(e, os.ErrPermission)
			if current.Phase == "submitting" && current.Upstream == nil {
				current.State = "needs_review"
			}
		} else {
			current.State = "succeeded"
			current.Phase = "complete"
			current.Result, _ = json.Marshal(result)
			current.Retryable = false
		}
		return nil
	})
}
func safeJobMessage(s string) string {
	if len(s) > 400 {
		return s[:400]
	}
	return s
}
func (f *featureRuntime) phase(j *Job, phase string) error {
	j.Phase = phase
	return f.change(j.ID, func(c *Job) error {
		if c.State == "cancelled" {
			return context.Canceled
		}
		c.Phase = phase
		return nil
	})
}
func (f *featureRuntime) progress(id string, n, total int64) {
	_ = f.change(id, func(j *Job) error { j.Completed = n; j.Total = total; return nil })
}
func (f *featureRuntime) run(ctx context.Context, j *Job, d *data) (any, error) {
	switch j.Kind {
	case "index":
		return f.indexDirectory(ctx, j, d)
	case "scan":
		var p struct{ Path string }
		if e := json.Unmarshal(j.Payload, &p); e != nil {
			return nil, e
		}
		if !d.user.Perm.Download || !d.Check(p.Path) {
			return nil, os.ErrPermission
		}
		n, threat, e := scanExistingPathWithClamAV(withJobProgress(ctx, f, j.ID), d, p.Path)
		f.progress(j.ID, int64(n), 0)
		r := cleanClamAVPathScanResponse(p.Path, n)
		r.JobID = j.ID
		if threat != nil {
			r.Status = "infected"
			r.Infected = true
			r.Threat = cleanClamAVMessage(threat.Threat, threat.FileName)
			r.Message = manualClamAVThreatMessage(threat)
		}
		return r, e
	case "archive":
		var req archiveCreateRequest
		if e := json.Unmarshal(j.Payload, &req); e != nil {
			return nil, e
		}
		if !d.user.Perm.Create || !d.user.Perm.Download {
			return nil, os.ErrPermission
		}
		p, _, e := prepareArchiveCreatePlan(req, d)
		if e != nil {
			return nil, e
		}
		r, e := runArchiveCreateSync(withJobProgress(ctx, f, j.ID), d, p)
		r.JobID = j.ID
		return r, e
	case "convert":
		var p convertXConvertPlan
		if e := json.Unmarshal(j.Payload, &p); e != nil {
			return nil, e
		}
		if !d.user.Perm.Create || !d.user.Perm.Download || !d.settings.ConvertX.Enabled || !d.Check(p.Source) || !d.Check(p.Destination) || (p.Overwrite && !d.user.Perm.Modify) {
			return nil, os.ErrPermission
		}
		e := f.convert(ctx, j, d, &p)
		return convertXConvertResponse{JobID: j.ID, Status: "done", Source: p.Source, Destination: p.Destination, ConvertTo: p.ConvertTo, Converter: p.Converter, Done: true}, e
	case "extract":
		var p extractionWork
		if e := json.Unmarshal(j.Payload, &p); e != nil {
			return nil, e
		}
		if !d.user.Perm.Create || (p.Override && !d.user.Perm.Modify) {
			return nil, os.ErrPermission
		}
		n, e := extractArchiveTo(withJobProgress(ctx, f, j.ID), d, p.Path, p.Inner, p.Destination, p.Override)
		return archiveExtractResponse{Destination: p.Destination, Extracted: n}, e
	case "erugo":
		return f.erugo(ctx, j, d)
	case "download":
		return f.prepareDownload(ctx, j, d)
	}
	return nil, errors.New("unknown job type")
}

var jobsListHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	out := []Job{}
	if d.features != nil {
		for _, j := range d.features.list() {
			if j.Owner == d.user.ID || d.user.Perm.Admin {
				out = append(out, j)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return renderJSON(w, r, out)
})
var jobControlHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	if d.features == nil {
		return http.StatusServiceUnavailable, nil
	}
	id := r.PathValue("id")
	if id == "" {
		id = muxJobID(r)
	}
	j, e := d.features.get(id)
	if e != nil || j.Owner != d.user.ID && !d.user.Perm.Admin {
		return http.StatusNotFound, nil
	}
	if r.Method == http.MethodGet {
		return renderJSON(w, r, j)
	}
	action := r.URL.Query().Get("action")
	d.features.mu.Lock()
	defer d.features.mu.Unlock()
	switch action {
	case "cancel":
		if jobTerminal(j.State) || j.Phase == "publishing" {
			return http.StatusConflict, nil
		}
		e = d.features.change(id, func(j *Job) error {
			if jobTerminal(j.State) || j.Phase == "publishing" {
				return ErrUploadState
			}
			j.State = "cancelled"
			j.Retryable = false
			if j.Kind == "extract" {
				j.Message = "Extraction cancelled; files already published remain."
			}
			if j.Kind == "erugo" && j.Phase == "submitting" {
				j.State = "needs_review"
				j.Message = "Cancelled locally; check Erugo before retrying."
			}
			return nil
		})
		if e == nil {
			if cancel := d.features.running[id]; cancel != nil {
				cancel()
			}
		}
	case "review_retry":
		e = d.features.change(id, func(j *Job) error {
			if j.State != "needs_review" || j.Phase != "submitting" || j.Upstream != nil || j.Pending != nil || (j.Kind != "convert" && j.Kind != "erugo") {
				return ErrUploadState
			}
			j.State = "queued"
			j.Phase = "queued"
			j.Message = ""
			return nil
		})
		if e == nil {
			select {
			case d.features.wake <- struct{}{}:
			default:
			}
		}
	case "dismiss":
		e = d.features.change(id, func(j *Job) error {
			if !jobTerminal(j.State) && j.State != "needs_review" {
				return ErrUploadState
			}
			j.State = "closed"
			j.Retryable = false
			j.Message = "Closed after review; published files and external actions were kept."
			return nil
		})
	case "retry":
		if j.State != "failed" || !j.Retryable {
			return http.StatusConflict, nil
		}
		e = d.features.change(id, func(j *Job) error {
			if j.State != "failed" || !j.Retryable {
				return ErrUploadState
			}
			j.State = "queued"
			j.Message = ""
			return nil
		})
		select {
		case d.features.wake <- struct{}{}:
		default:
		}
	default:
		return http.StatusBadRequest, nil
	}
	if errors.Is(e, ErrUploadState) {
		return http.StatusConflict, nil
	}
	return errToStatus(e), e
})

func muxJobID(r *http.Request) string { return mux.Vars(r)["id"] }

func legacyJobStatus(w http.ResponseWriter, r *http.Request, d *data, id, kind string) (int, error) {
	j, e := d.features.get(id)
	if e != nil || j.Kind != kind || j.Owner != d.user.ID && !d.user.Perm.Admin {
		return http.StatusNotFound, nil
	}
	if j.State == "succeeded" {
		w.Header().Set("Content-Type", "application/json")
		_, e = w.Write(j.Result)
		return 0, e
	}
	result := map[string]any{"jobId": id, "status": "running", "message": j.Message, "done": false, "progress": j.Completed, "total": j.Total}
	if j.State != "running" && j.State != "queued" {
		result["status"] = "error"
		result["done"] = true
	}
	return renderJSON(w, r, result)
}

type progressContextKey struct{}
type jobProgress struct {
	f  *featureRuntime
	id string
	n  int64
}

func withJobProgress(ctx context.Context, f *featureRuntime, id string) context.Context {
	return context.WithValue(ctx, progressContextKey{}, &jobProgress{f: f, id: id})
}
func reportJobProgress(ctx context.Context, n, total int64) {
	if p, ok := ctx.Value(progressContextKey{}).(*jobProgress); ok {
		p.n += n
		p.f.progress(p.id, p.n, total)
	}
}

type contextReader struct {
	ctx context.Context
	r   interface{ Read([]byte) (int, error) }
}

func (c contextReader) Read(p []byte) (int, error) {
	if e := c.ctx.Err(); e != nil {
		return 0, e
	}
	return c.r.Read(p)
}
