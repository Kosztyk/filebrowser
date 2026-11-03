package fbhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/filebrowser/filebrowser/v2/files"
	bolt "go.etcd.io/bbolt"
	berrors "go.etcd.io/bbolt/errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type directoryWork struct {
	RefreshToken     string
	Path, By, Filter string
	Asc              bool
	Modified         int64
	Policy           string
}
type directorySnapshot struct {
	ID       string    `json:"id"`
	Count    int       `json:"count"`
	Dirs     int       `json:"dirs"`
	Files    int       `json:"files"`
	Modified int64     `json:"modified"`
	Expires  time.Time `json:"expires"`
}

var numberPattern = regexp.MustCompile(`[0-9]+`)

func naturalKey(s string) string {
	return numberPattern.ReplaceAllStringFunc(strings.ToLower(s), func(n string) string { n = strings.TrimLeft(n, "0"); return fmt.Sprintf("%08d:%s", len(n), n) })
}
func indexKey(i *files.FileInfo, by string) string {
	group := "1"
	if i.IsDir {
		group = "0"
	}
	var value string
	switch by {
	case "size":
		value = fmt.Sprintf("%020d", i.Size)
	case "modified":
		value = fmt.Sprintf("%020d", i.ModTime.UnixNano())
	default:
		value = naturalKey(i.Name)
	}
	return group + value + "\x00" + i.Name
}
func listingType(i *files.FileInfo) string {
	if i.IsDir {
		return ""
	}
	if files.IsArchiveName(i.Name) {
		return "archive"
	}
	t := mime.TypeByExtension(i.Extension)
	for _, prefix := range []string{"image", "audio", "video", "text"} {
		if strings.HasPrefix(t, prefix+"/") {
			return prefix
		}
	}
	if i.Extension == ".pdf" {
		return "pdf"
	}
	return "blob"
}
func directoryPolicy(d *data) string {
	b, _ := json.Marshal([]any{d.settings.Rules, d.user.Rules, d.user.HideDotfiles, d.server.FollowExternalSymlinks})
	return credentialHash(string(b))
}
func (f *featureRuntime) indexDirectory(ctx context.Context, j *Job, d *data) (any, error) {
	f.indexing.Lock()
	defer f.indexing.Unlock()
	var p directoryWork
	if e := json.Unmarshal(j.Payload, &p); e != nil {
		return nil, e
	}
	if !d.Check(p.Path) || p.Policy != directoryPolicy(d) {
		return nil, os.ErrPermission
	}
	dir, e := d.user.Fs.Open(p.Path)
	if e != nil {
		return nil, e
	}
	defer dir.Close()
	bucket := []byte("directory-" + j.ID)
	finished := []Job{}
	for _, job := range f.list() {
		if job.Kind == "index" && job.State == "succeeded" {
			finished = append(finished, job)
		}
	}
	sort.Slice(finished, func(i, j int) bool { return finished[i].Updated.After(finished[j].Updated) })
	if e = f.m.journal.Update(func(tx *bolt.Tx) error {
		for n, job := range finished {
			if n >= 7 || time.Since(job.Updated) > 10*time.Minute {
				if err := tx.DeleteBucket([]byte("directory-" + job.ID)); err != nil && !errors.Is(err, berrors.ErrBucketNotFound) {
					return err
				}
			}
		}
		return nil
	}); e != nil {
		return nil, e
	}

	e = f.m.journal.Update(func(tx *bolt.Tx) error { _ = tx.DeleteBucket(bucket); _, e := tx.CreateBucket(bucket); return e })
	if e != nil {
		return nil, e
	}
	complete := false
	defer func() {
		if !complete {
			_ = f.m.journal.Update(func(tx *bolt.Tx) error { return tx.DeleteBucket(bucket) })
		}
	}()
	snap := directorySnapshot{ID: j.ID, Modified: p.Modified, Expires: time.Now().Add(10 * time.Minute)}
	bytes := 0
	for {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		entries, readErr := dir.Readdir(256)
		batch := map[string][]byte{}
		for _, info := range entries {
			full := path.Join(p.Path, info.Name())
			if !d.Check(full) || !strings.Contains(strings.ToLower(info.Name()), strings.ToLower(p.Filter)) {
				continue
			}
			item, e := files.NewFileInfo(&files.FileOptions{Fs: d.user.Fs, Path: full, Checker: d})
			if e != nil {
				if os.IsNotExist(e) || os.IsPermission(e) {
					continue
				}
				return nil, e
			}
			item.Type = listingType(item)
			v, e := json.Marshal(item)
			if e != nil {
				return nil, e
			}
			bytes += len(v)
			if bytes > 128<<20 || snap.Count >= 1000000 {
				return nil, errors.New("directory index capacity exceeded; narrow the filter")
			}
			batch[indexKey(item, p.By)] = v
			snap.Count++
			if item.IsDir {
				snap.Dirs++
			} else {
				snap.Files++
			}
		}
		e = f.m.journal.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket(bucket)
			for k, v := range batch {
				if e := b.Put([]byte(k), v); e != nil {
					return e
				}
			}
			return nil
		})
		if e != nil {
			return nil, e
		}
		f.progress(j.ID, int64(snap.Count), 0)
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	info, e := d.user.Fs.Stat(p.Path)
	if e != nil {
		return nil, e
	}
	if info.ModTime().UnixNano() != p.Modified {
		return nil, errors.New("directory changed while indexing; refresh to retry")
	}
	complete = true
	return snap, nil
}
func (f *featureRuntime) directoryPage(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	info, e := d.user.Fs.Stat(r.URL.Path)
	if e != nil {
		return errToStatus(e), e
	}
	if !d.Check(r.URL.Path) {
		return http.StatusForbidden, nil
	}
	if !info.IsDir() {
		return -1, nil
	}
	q := r.URL.Query()
	by := q.Get("sort")
	if by == "" {
		by = d.user.Sorting.By
	}
	if by != "size" && by != "modified" {
		by = "name"
	}
	asc := q.Get("asc") != "false"
	if q.Get("asc") == "" {
		asc = d.user.Sorting.Asc
	}
	p := directoryWork{Path: r.URL.Path, By: by, Filter: q.Get("filter"), Asc: asc, Modified: info.ModTime().UnixNano(), Policy: directoryPolicy(d)}
	if q.Get("refresh") == "true" && q.Get("snapshot") == "" {
		p.RefreshToken = newFileActionJobID()
	}
	if len(p.Filter) > 200 {
		return http.StatusBadRequest, nil
	}
	id := q.Get("snapshot")
	if id == "" && q.Get("refresh") != "true" {
		for _, j := range f.list() {
			if j.Owner != d.user.ID || j.Kind != "index" || j.State != "succeeded" {
				continue
			}
			var old directoryWork
			var snap directorySnapshot
			if json.Unmarshal(j.Payload, &old) == nil && old == p && json.Unmarshal(j.Result, &snap) == nil && time.Now().Before(snap.Expires) {
				id = j.ID
				break
			}
		}
	}
	if id == "" {
		j, e := f.submit(d, "index", p, nil)
		if e != nil {
			return errToStatus(e), e
		}
		w.WriteHeader(http.StatusAccepted)
		return renderJSON(w, r, map[string]any{"indexing": true, "jobId": j.ID})
	}
	job, e := f.get(id)
	var original directoryWork
	var snap directorySnapshot
	if e != nil || job.Owner != d.user.ID || job.Kind != "index" || job.State != "succeeded" || json.Unmarshal(job.Payload, &original) != nil || json.Unmarshal(job.Result, &snap) != nil || time.Now().After(snap.Expires) {
		return http.StatusConflict, errors.New("directory snapshot expired or changed; refresh")
	}
	p.RefreshToken = original.RefreshToken
	if p != original {
		return http.StatusConflict, errors.New("directory snapshot changed; refresh")
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if offset < 0 || offset > snap.Count {
		return http.StatusBadRequest, nil
	}
	if limit < 1 || limit > 500 {
		limit = 200
	}
	items := []*files.FileInfo{}
	e = f.m.journal.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("directory-" + id))
		if b == nil {
			return os.ErrNotExist
		}
		// Reverse within each section, never the sections themselves. The
		// original listing uses Asc=false for A-Z names; retain saved preferences.
		forward := asc
		if by == "name" {
			forward = !asc
		}
		n := 0
		for _, group := range []byte{'0', '1'} {
			c := b.Cursor()
			k, v := c.Seek([]byte{group})
			step := c.Next
			if !forward {
				k, _ = c.Seek([]byte{group + 1})
				if k == nil {
					k, v = c.Last()
				} else {
					k, v = c.Prev()
				}
				step = c.Prev
			}
			for ; len(k) > 0 && k[0] == group; k, v = step() {
				if n < offset {
					n++
					continue
				}
				if len(items) >= limit {
					return nil
				}
				var i files.FileInfo
				if e := json.Unmarshal(v, &i); e != nil {
					return e
				}
				if d.Check(i.Path) {
					items = append(items, &i)
				}
			}
		}
		return nil
	})
	if e != nil {
		return http.StatusConflict, errors.New("directory snapshot no longer available; refresh")
	}
	dirs := 0
	for _, i := range items {
		if i.IsDir {
			dirs++
		}
	}
	return renderJSON(w, r, map[string]any{"path": r.URL.Path, "name": info.Name(), "isDir": true, "type": "", "items": items, "numDirs": dirs, "numFiles": len(items) - dirs, "sorting": map[string]any{"by": by, "asc": asc}, "page": map[string]any{"snapshot": id, "offset": offset, "limit": limit, "total": snap.Count, "numDirs": snap.Dirs, "numFiles": snap.Files, "filter": p.Filter, "sort": by, "asc": asc, "expires": snap.Expires}})
}
