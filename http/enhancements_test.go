package fbhttp

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/filebrowser/filebrowser/v2/runner"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"github.com/spf13/afero"
	bolt "go.etcd.io/bbolt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func featureFixture(t *testing.T) (*featureRuntime, *data, string, string) {
	t.Helper()
	m, d, scope, journal := uploadFixture(t)
	d.user.Perm.Download = true
	d.user.Perm.Share = true
	d.user.Perm.Admin = true
	d.user.Perm.Delete = true
	d.user.Perm.Rename = true
	if e := d.store.Users.Save(d.user); e != nil {
		t.Fatal(e)
	}
	fs := files.NewFs(afero.NewOsFs(), scope, false)
	d.store.Users.(*customFSUser).fs = fs
	d.user.Fs = fs
	f, e := newFeatureRuntime(m, false)
	if e != nil {
		t.Fatal(e)
	}
	d.features = f
	d.Runner = &runner.Runner{Settings: d.settings}
	return f, d, scope, journal
}
func runJob(t *testing.T, f *featureRuntime, j Job) Job {
	t.Helper()
	if e := f.change(j.ID, func(j *Job) error { j.State = "running"; return nil }); e != nil {
		t.Fatal(e)
	}
	f.execute(j)
	got, e := f.get(j.ID)
	if e != nil {
		t.Fatal(e)
	}
	return got
}
func restartFeatures(t *testing.T, f *featureRuntime, d *data, journal string) *featureRuntime {
	t.Helper()
	f.m.Close()
	m, e := NewUploadManager(journal, d.store, d.server, false)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(m.Close)
	next, e := newFeatureRuntime(m, false)
	if e != nil {
		t.Fatal(e)
	}
	d.features = next
	return next
}
func TestPersistentJobDedupeRestartAndArtifact(t *testing.T) {
	f, d, scope, journal := featureFixture(t)
	fixtureFile(t, scope, "a", "contents")
	p := downloadWork{Items: []string{"/a"}, Name: "a.zip"}
	j, e := f.submit(d, "download", p, nil)
	if e != nil {
		t.Fatal(e)
	}
	same, e := f.submit(d, "download", p, nil)
	if e != nil || same.ID != j.ID {
		t.Fatal("duplicate job", e)
	}
	_ = f.change(j.ID, func(j *Job) error { j.State = "running"; j.Phase = "packaging"; return nil })
	f = restartFeatures(t, f, d, journal)
	restored, e := f.get(j.ID)
	if e != nil || restored.State != "queued" || restored.Owner != d.user.ID {
		t.Fatal(restored, e)
	}
	got := runJob(t, f, restored)
	if got.State != "succeeded" {
		t.Fatal(got.Message)
	}
	var a artifact
	_ = json.Unmarshal(got.Result, &a)
	if a.Size == 0 || len(a.SHA256) != 64 {
		t.Fatal(a)
	}
}
func TestJobStatusOwnerAndPrivatePayload(t *testing.T) {
	f, d, scope, _ := featureFixture(t)
	fixtureFile(t, scope, "a", "x")
	j, e := f.submit(d, "download", downloadWork{Items: []string{"/a"}, Name: "x.zip"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	other := *d.user
	other.ID = 0
	other.Username = "other"
	other.Perm.Admin = false
	if e = d.store.Users.Save(&other); e != nil {
		t.Fatal(e)
	}
	claims := authToken{User: userInfo{ID: other.ID}, RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	token, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(d.settings.Key)
	r := mux.SetURLVars(httptest.NewRequest("GET", "/jobs/"+j.ID, nil), map[string]string{"id": j.ID})
	r.Header.Set("X-Auth", token)
	w := httptest.NewRecorder()
	handle(jobControlHandler, "", d.store, d.server, f).ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal(w.Code, w.Body.String())
	}
	b, _ := json.Marshal(j)
	if strings.Contains(string(b), scope) || strings.Contains(string(b), "Payload") || strings.Contains(string(b), "Items") {
		t.Fatal("private job data exposed")
	}
}
func TestAmbiguousSubmissionRequiresReview(t *testing.T) {
	f, d, _, journal := featureFixture(t)
	j, e := f.submit(d, "convert", convertXConvertPlan{}, nil)
	if e != nil {
		t.Fatal(e)
	}
	_ = f.change(j.ID, func(j *Job) error { j.State = "running"; j.Phase = "submitting"; return nil })
	f = restartFeatures(t, f, d, journal)
	j, _ = f.get(j.ID)
	if j.State != "needs_review" {
		t.Fatal(j)
	}
}
func TestConvertRecoveryReusesUpstreamID(t *testing.T) {
	f, d, scope, _ := featureFixture(t)
	var submissions atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/convert":
			submissions.Add(1)
			t.Error("duplicate submission")
		case "/api/jobs/remote":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "status": "completed", "total": 1, "done": 1, "files": []map[string]string{{"status": "done", "output": "a.pdf", "downloadUrl": "/output"}}})
		case "/output":
			_, _ = w.Write([]byte("converted"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	fixtureFile(t, scope, "a.txt", "input")
	d.settings.ConvertX.Enabled = true
	d.settings.ConvertX.URL = upstream.URL
	_ = d.store.Settings.Save(d.settings)
	p := convertXConvertPlan{Source: "/a.txt", Destination: "/a.pdf", ConvertTo: "pdf"}
	j, e := f.submit(d, "convert", p, []string{p.Source, p.Destination})
	if e != nil {
		t.Fatal(e)
	}
	_ = f.change(j.ID, func(j *Job) error {
		j.Upstream = &convertXSubmitResponse{JobID: "remote", ExpectedOutput: "a.pdf"}
		j.Phase = "processing"
		return nil
	})
	j, _ = f.get(j.ID)
	got := runJob(t, f, j)
	if got.State != "succeeded" || submissions.Load() != 0 {
		t.Fatal(got.Message)
	}
	expectContent(t, scope, "a.pdf", "converted")
}
func TestPublishRejectsChangedDestination(t *testing.T) {
	f, d, scope, _ := featureFixture(t)
	fixtureFile(t, scope, "out", "original")
	j, e := f.submit(d, "convert", convertXConvertPlan{}, []string{"/out"})
	if e != nil {
		t.Fatal(e)
	}
	fixtureFile(t, scope, "out", "external save")
	if e = f.publishJob(context.Background(), &j, d, "/out", strings.NewReader("converted")); e == nil {
		t.Fatal("lost update")
	}
	expectContent(t, scope, "out", "external save")
}
func TestOfficeLeasePersistenceMutationConflictAndHistory(t *testing.T) {
	f, d, scope, journal := featureFixture(t)
	fixtureFile(t, scope, "office.docx", "old")
	v, _ := files.Version(d.user.Fs, "/office.docx")
	lease := files.Lease{Value: "token", Expires: time.Now().Add(time.Hour), Before: v}
	if e := f.persistOfficeLock(d, "/office.docx", lease); e != nil {
		t.Fatal(e)
	}
	defer files.ClearLease(d.user.Fs, "/office.docx")
	if _, e := files.AtomicWrite(d.user.Fs, "/office.docx", strings.NewReader("bad"), 0640, 0750); e == nil {
		t.Fatal("upload ignored lock")
	}
	if e := files.GuardMutation(d.user.Fs, []string{"/"}, func() error { return nil }); e == nil {
		t.Fatal("ancestor mutation ignored lock")
	}
	files.ClearLease(d.user.Fs, "/office.docx")
	restartFeatures(t, f, d, journal)
	if _, ok := files.GetLease(d.user.Fs, "/office.docx"); !ok {
		t.Fatal("lost lease")
	}
	if _, e := files.AtomicWrite(files.WithLease(d.user.Fs, lease.Value), "/office.docx", strings.NewReader("saved"), 0640, 0750); e != nil {
		t.Fatal(e)
	}
	versions, e := files.ListVersions(d.user.Fs, "/office.docx")
	if e != nil || len(versions) != 1 {
		t.Fatal(versions, e)
	}
	old, e := afero.ReadFile(files.InternalFs(d.user.Fs), versions[0].Path)
	if e != nil || string(old) != "old" {
		t.Fatal("missing previous contents", e)
	}
}
func TestRevocationAndPasswordChange(t *testing.T) {
	f, d, _, journal := featureFixture(t)
	r := httptest.NewRequest("POST", "/login", nil)
	id, e := mintSession(r, d, d.user.ID, d.user.Password)
	if e != nil {
		t.Fatal(e)
	}
	tk := authToken{User: userInfo{ID: d.user.ID}, RegisteredClaims: jwt.RegisteredClaims{ID: id, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	if !sessionValid(r, d, &tk) {
		t.Fatal("new session rejected")
	}
	_ = f.m.journal.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("sessions-v1"))
		var s storedSession
		_ = json.Unmarshal(b.Get([]byte(id)), &s)
		s.Revoked = true
		v, _ := json.Marshal(s)
		return b.Put([]byte(id), v)
	})
	restartFeatures(t, f, d, journal)
	if sessionValid(r, d, &tk) {
		t.Fatal("revocation lost at restart")
	}
	d.sessionID = ""
	id, e = mintSession(r, d, d.user.ID, d.user.Password)
	if e != nil {
		t.Fatal(e)
	}
	tk.ID = id
	d.user.Password = "changed"
	if sessionValid(r, d, &tk) {
		t.Fatal("session survived credential change")
	}
}
func TestDirectoryPagesAndExternalInvalidation(t *testing.T) {
	f, d, scope, _ := featureFixture(t)
	for i := 0; i < 501; i++ {
		fixtureFile(t, scope, fmt.Sprintf("file-%d.txt", i), "x")
	}
	_ = os.Mkdir(filepath.Join(scope, ".filebrowser-staging"), 0700)
	r := httptest.NewRequest("GET", "/?paged=true&limit=200&asc=true", nil)
	w := httptest.NewRecorder()
	status, e := f.directoryPage(w, r, d)
	if e != nil || status != 0 || w.Code != 202 {
		t.Fatal(status, w.Code, e)
	}
	var pending struct {
		JobID string `json:"jobId"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &pending)
	j, _ := f.get(pending.JobID)
	got := runJob(t, f, j)
	if got.State != "succeeded" {
		t.Fatal(got.Message)
	}
	var snap directorySnapshot
	_ = json.Unmarshal(got.Result, &snap)
	if snap.Count != 501 {
		t.Fatal(snap)
	}
	seen := map[string]bool{}
	for offset := 0; offset < 501; offset += 200 {
		r = httptest.NewRequest("GET", fmt.Sprintf("/?paged=true&limit=200&asc=true&offset=%d&snapshot=%s", offset, j.ID), nil)
		w = httptest.NewRecorder()
		status, e = f.directoryPage(w, r, d)
		if e != nil || status != 0 {
			t.Fatal(status, e)
		}
		var page struct{ Items []files.FileInfo }
		_ = json.Unmarshal(w.Body.Bytes(), &page)
		for _, i := range page.Items {
			if seen[i.Name] {
				t.Fatal("duplicate page item")
			}
			seen[i.Name] = true
		}
	}
	if len(seen) != 501 {
		t.Fatal(len(seen))
	}
	fixtureFile(t, scope, "new", "x")
	r = httptest.NewRequest("GET", "/?paged=true&asc=true&snapshot="+j.ID, nil)
	status, _ = f.directoryPage(httptest.NewRecorder(), r, d)
	if status != 409 {
		t.Fatal("stale snapshot", status)
	}
}
func makeZip(t *testing.T, scope string, entries map[string]string) {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, body := range entries {
		w, e := z.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		_, _ = io.WriteString(w, body)
	}
	_ = z.Close()
	if e := os.WriteFile(filepath.Join(scope, "test.zip"), b.Bytes(), 0640); e != nil {
		t.Fatal(e)
	}
}
func TestArchiveBudgetsAndAtomicOverwrite(t *testing.T) {
	_, d, scope, _ := featureFixture(t)
	makeZip(t, scope, map[string]string{"huge.txt": strings.Repeat("x", 10000)})
	_ = os.Mkdir(filepath.Join(scope, "out"), 0750)
	fixtureFile(t, scope, "out/huge.txt", "original")
	d.settings.Work.ArchiveBytes = 100
	d.settings.Work.ReserveBytes = 1
	_, e := extractArchiveTo(context.Background(), d, "/test.zip", ".", "/out", true)
	if e == nil || !strings.Contains(e.Error(), "expanded-size") {
		t.Fatal(e)
	}
	expectContent(t, scope, "out/huge.txt", "original")
	d.settings.Work.ArchiveDepth = 1
	makeZip(t, scope, map[string]string{"a/b.txt": "x"})
	_, e = extractArchiveTo(context.Background(), d, "/test.zip", ".", "/deep", false)
	if e == nil || !strings.Contains(e.Error(), "depth") {
		t.Fatal(e)
	}
	d.settings.Work.ArchiveDepth = 64
	d.settings.Work.ArchiveEntries = 1
	makeZip(t, scope, map[string]string{"one": "x", "two": "y"})
	_, e = extractArchiveTo(context.Background(), d, "/test.zip", ".", "/count", false)
	if e == nil || !strings.Contains(e.Error(), "count") {
		t.Fatal(e)
	}
	d.settings.Work.ArchiveEntries = 10
	d.settings.Work.ReserveBytes = 1 << 62
	_, e = extractArchiveTo(context.Background(), d, "/test.zip", ".", "/space", false)
	if e == nil || !strings.Contains(e.Error(), "space") {
		t.Fatal(e)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, e = extractArchiveTo(ctx, d, "/test.zip", ".", "/time", false)
	if e == nil {
		t.Fatal("deadline ignored")
	}
}
func TestPreparedDownloadRangeAndPermissionReload(t *testing.T) {
	f, d, scope, _ := featureFixture(t)
	fixtureFile(t, scope, "a", "contents")
	j, e := f.submit(d, "download", downloadWork{Items: []string{"/a"}, Name: "x.zip"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	got := runJob(t, f, j)
	if got.State != "succeeded" {
		t.Fatal(got.Message)
	}
	r := mux.SetURLVars(httptest.NewRequest("GET", "/downloads/"+j.ID, nil), map[string]string{"id": j.ID})
	r.Header.Set("Range", "bytes=0-9")
	r.Header.Set("X-Auth", signToken(t, d.user.Perm, d.settings.Key))
	w := httptest.NewRecorder()
	handle(downloadArtifactHandler, "", d.store, d.server, f).ServeHTTP(w, r)
	if w.Code != 206 || w.Body.Len() != 10 || w.Header().Get("ETag") == "" {
		t.Fatal(w.Code, w.Body.String())
	}
	d.user.Perm.Download = false
	_ = d.store.Users.Save(d.user)
	w = httptest.NewRecorder()
	handle(downloadArtifactHandler, "", d.store, d.server, f).ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("permission not reloaded", w.Code)
	}
}
func TestCSVWindowsKeepCompleteRecords(t *testing.T) {
	_, d, scope, _ := featureFixture(t)
	fixtureFile(t, scope, "large.csv", "name,value\n\"first\nsecond\",value\nnext,abcdefghijk\n")
	offset := int64(0)
	rows := [][]string{}
	for i := 0; i < 5; i++ {
		r := httptest.NewRequest("GET", fmt.Sprintf("/large.csv?csv=true&bytes=31&offset=%d", offset), nil)
		r.Header.Set("X-Auth", signToken(t, d.user.Perm, d.settings.Key))
		w := httptest.NewRecorder()
		handle(textPreviewHandler, "", d.store, d.server, d.features).ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var chunk struct {
			Rows    [][]string
			Next    int64
			HasMore bool
		}
		_ = json.Unmarshal(w.Body.Bytes(), &chunk)
		rows = append(rows, chunk.Rows...)
		if !chunk.HasMore {
			break
		}
		if chunk.Next <= offset {
			t.Fatal("no progress")
		}
		offset = chunk.Next
	}
	if len(rows) != 3 || rows[1][0] != "first\nsecond" || rows[2][1] != "abcdefghijk" {
		t.Fatal(rows)
	}
}
func TestPublicationRecoveryAfterRename(t *testing.T) {
	f, d, _, journal := featureFixture(t)
	j, e := f.submit(d, "convert", convertXConvertPlan{Destination: "/output"}, []string{"/output"})
	if e != nil {
		t.Fatal(e)
	}
	s, fd, e := files.NewStagedFile(d.user.Fs, "/output", "job-"+j.ID, 0640, 0750)
	if e != nil {
		t.Fatal(e)
	}
	_, _ = fd.WriteString("converted")
	_ = fd.Sync()
	_ = fd.Close()
	if e = f.recordPublication(&j, s, convertXConvertResponse{JobID: j.ID, Status: "done", Done: true}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Commit(); e != nil {
		t.Fatal(e)
	}
	f = restartFeatures(t, f, d, journal)
	recovered, e := f.get(j.ID)
	if e != nil || recovered.State != "succeeded" || recovered.Pending != nil {
		t.Fatal(recovered, e)
	}
}
func TestExtractionRestartSkipsVerifiedEntries(t *testing.T) {
	f, d, scope, journal := featureFixture(t)
	makeZip(t, scope, map[string]string{"one.txt": "first", "two.txt": "second"})
	_ = os.Mkdir(filepath.Join(scope, "out"), 0750)
	j, e := f.submit(d, "extract", extractionWork{Path: "/test.zip", Inner: ".", Destination: "/out"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = writeExtracted(withJobProgress(context.Background(), f, j.ID), d, "/out/one.txt", strings.NewReader("first")); e != nil {
		t.Fatal(e)
	}
	_ = f.change(j.ID, func(j *Job) error { j.State = "running"; return nil })
	f = restartFeatures(t, f, d, journal)
	j, _ = f.get(j.ID)
	done := runJob(t, f, j)
	if done.State != "succeeded" {
		t.Fatal(done.Message)
	}
	expectContent(t, scope, "out/one.txt", "first")
	expectContent(t, scope, "out/two.txt", "second")
}
func TestRenewExpiredAccessOnlyForActiveSession(t *testing.T) {
	f, d, _, _ := featureFixture(t)
	r := httptest.NewRequest("POST", "/api/renew", nil)
	id, e := mintSession(r, d, d.user.ID, d.user.Password)
	if e != nil {
		t.Fatal(e)
	}
	claims := authToken{User: userInfo{ID: d.user.ID}, RegisteredClaims: jwt.RegisteredClaims{ID: id, ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute))}}
	token, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(d.settings.Key)
	r.Header.Set("X-Auth", token)
	w := httptest.NewRecorder()
	handle(renewHandler(time.Hour), "", d.store, d.server, f).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	r.Method = "GET"
	r.URL.Path = "/resources"
	w = httptest.NewRecorder()
	handle(withUser(func(http.ResponseWriter, *http.Request, *data) (int, error) { return 200, nil }), "", d.store, d.server, f).ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("expired token authorized file endpoint", w.Code)
	}
}
func TestBoundedWorkersShutdownWithQueuedJobs(t *testing.T) {
	f, d, scope, _ := featureFixture(t)
	var active, peak atomic.Int32
	block := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		defer active.Add(-1)
		select {
		case <-block:
		case <-r.Context().Done():
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = io.WriteString(w, `{"status":"clean"}`)
	}))
	defer upstream.Close()
	d.settings.ClamAV.URL = upstream.URL
	_ = d.store.Settings.Save(d.settings)
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("scan-%d", i)
		fixtureFile(t, scope, name, "x")
		if _, e := f.submit(d, "scan", struct{ Path string }{"/" + name}, nil); e != nil {
			t.Fatal(e)
		}
	}
	for i := 0; i < 2; i++ {
		f.m.workers.Add(1)
		go func() { defer f.m.workers.Done(); f.worker() }()
	}
	deadline := time.After(3 * time.Second)
	for peak.Load() < 2 {
		select {
		case <-deadline:
			t.Fatal("workers did not start")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	if peak.Load() > 2 {
		t.Fatal("unbounded workers")
	}
	close(block)
	done := make(chan struct{})
	go func() { f.m.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown blocked")
	}
}

func TestErugoFolderJobPackagesStreamsAndPersistsResult(t *testing.T) {
	f, d, scope, journal := featureFixture(t)
	_ = os.Mkdir(filepath.Join(scope, "folder"), 0750)
	fixtureFile(t, scope, "folder/one.txt", "one")
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.ContentLength <= 0 || r.Header.Get("Idempotency-Key") == "" {
			t.Error("missing exact length or job key")
		}
		reader, e := r.MultipartReader()
		if e != nil {
			t.Error(e)
			return
		}
		found := false
		for {
			part, e := reader.NextPart()
			if e == io.EOF {
				break
			}
			if e != nil {
				t.Error(e)
				return
			}
			b, _ := io.ReadAll(part)
			if part.FileName() != "" {
				z, e := zip.NewReader(bytes.NewReader(b), int64(len(b)))
				if e != nil {
					t.Error(e)
					return
				}
				for _, entry := range z.File {
					if strings.HasSuffix(entry.Name, "one.txt") {
						fd, _ := entry.Open()
						body, _ := io.ReadAll(fd)
						_ = fd.Close()
						found = string(body) == "one"
					}
				}
			}
		}
		if !found {
			t.Error("folder file missing")
		}
		_, _ = io.WriteString(w, `{"status":"success","share_url":"https://example.test/share/fixture"}`)
	}))
	defer upstream.Close()
	d.settings.Erugo.Enabled = true
	d.settings.Erugo.URL = upstream.URL
	d.settings.Erugo.APIKey = "fixture-key"
	_ = d.store.Settings.Save(d.settings)
	j, e := f.submit(d, "erugo", erugoWork{Path: "/folder"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	done := runJob(t, f, j)
	if done.State != "succeeded" || calls.Load() != 1 {
		t.Fatal(done.Message, calls.Load())
	}
	f = restartFeatures(t, f, d, journal)
	stored, e := f.get(j.ID)
	if e != nil || stored.State != "succeeded" || !strings.Contains(string(stored.Result), "example.test/share/fixture") {
		t.Fatal(stored, e)
	}
}
func TestOfficeSaveUpdatesPersistentVersionAndKeepsPrevious(t *testing.T) {
	f, d, scope, journal := featureFixture(t)
	fixtureFile(t, scope, "office.docx", "original")
	before, _ := files.Version(d.user.Fs, "/office.docx")
	if e := f.persistOfficeLock(d, "/office.docx", files.Lease{Value: "office-token", Expires: time.Now().Add(time.Hour), Before: before}); e != nil {
		t.Fatal(e)
	}
	defer files.ClearLease(d.user.Fs, "/office.docx")
	info, _ := d.user.Fs.Stat("/office.docx")
	r := httptest.NewRequest("POST", "/wopi", strings.NewReader("office save"))
	r.Header.Set("X-WOPI-Lock", "office-token")
	w := httptest.NewRecorder()
	status, e := wopiPutFile(w, r, d, &wopiFileContext{Claims: wopiTokenClaims{Path: "/office.docx", CanWrite: true}, Info: info})
	if status != 0 || e != nil {
		t.Fatal(status, e)
	}
	expectContent(t, scope, "office.docx", "office save")
	files.ClearLease(d.user.Fs, "/office.docx")
	restartFeatures(t, f, d, journal)
	lease, ok := files.GetLease(d.user.Fs, "/office.docx")
	current, _ := files.Version(d.user.Fs, "/office.docx")
	if !ok || lease.Before != current {
		t.Fatal("office version did not survive restart")
	}
	fixtureFile(t, scope, "office.docx", "external edit")
	r = httptest.NewRequest("POST", "/wopi", strings.NewReader("stale office save"))
	r.Header.Set("X-WOPI-Lock", "office-token")
	status, e = wopiPutFile(httptest.NewRecorder(), r, d, &wopiFileContext{Claims: wopiTokenClaims{Path: "/office.docx", CanWrite: true}, Info: info})
	if status != 409 || e != nil {
		t.Fatal("external version conflict ignored", status, e)
	}
}
func TestDiagnosticsRejectHTMLAndRedactSecrets(t *testing.T) {
	_, d, _, _ := featureFixture(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<html>not the conversion API</html>")
	}))
	defer upstream.Close()
	cfg := d.settings.ConvertX
	cfg.URL = upstream.URL
	if e := testConvertXConnection(context.Background(), cfg); e == nil {
		t.Fatal("HTML accepted as integration health")
	}
	d.settings.ConvertX.APIKey = "do-not-return-this"
	message := redactDiagnostic("upstream echoed do-not-return-this", d)
	if strings.Contains(message, d.settings.ConvertX.APIKey) {
		t.Fatal("secret exposed")
	}
}

func TestReviewActionsRequireAmbiguityAndPreserveDestination(t *testing.T) {
	f, d, scope, _ := featureFixture(t)
	fixtureFile(t, scope, "keep", "existing")
	j, e := f.submit(d, "erugo", erugoWork{Path: "/keep"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	_ = f.change(j.ID, func(j *Job) error { j.State = "needs_review"; j.Phase = "submitting"; return nil })
	r := mux.SetURLVars(httptest.NewRequest("POST", "/jobs/"+j.ID+"?action=review_retry", nil), map[string]string{"id": j.ID})
	r.Header.Set("X-Auth", signToken(t, d.user.Perm, d.settings.Key))
	w := httptest.NewRecorder()
	handle(jobControlHandler, "", d.store, d.server, f).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	stored, _ := f.get(j.ID)
	if stored.State != "queued" || stored.Phase != "queued" {
		t.Fatal(stored)
	}
	_ = f.change(j.ID, func(j *Job) error { j.State = "running"; j.Phase = "publishing"; return nil })
	r.URL.RawQuery = "action=cancel"
	w = httptest.NewRecorder()
	handle(jobControlHandler, "", d.store, d.server, f).ServeHTTP(w, r)
	if w.Code != 409 {
		t.Fatal("publication cancellation was reported as successful", w.Code)
	}
	expectContent(t, scope, "keep", "existing")
}

func TestCompletedActionReplaysOnlyWithExplicitOperationKey(t *testing.T) {
	f, d, _, _ := featureFixture(t)
	payload := struct{ Path string }{"/example"}
	j, e := f.submit(d, "scan", payload, nil)
	if e != nil {
		t.Fatal(e)
	}
	_ = f.change(j.ID, func(j *Job) error { j.State = "succeeded"; return nil })
	next, e := f.submit(d, "scan", payload, nil)
	if e != nil || next.ID == j.ID {
		t.Fatal("a deliberate rescan reused stale results", e)
	}
	d.idempotency = "fixture-operation"
	first, e := f.submit(d, "erugo", payload, nil)
	if e != nil {
		t.Fatal(e)
	}
	_ = f.change(first.ID, func(j *Job) error { j.State = "succeeded"; return nil })
	replayed, e := f.submit(d, "erugo", payload, nil)
	if e != nil || replayed.ID != first.ID {
		t.Fatal("same request key created duplicate action", e)
	}
}
