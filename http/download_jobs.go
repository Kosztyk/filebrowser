package fbhttp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/gorilla/mux"
	"github.com/mholt/archives"
	"github.com/shirou/gopsutil/v4/disk"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type artifact struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	SHA256  string    `json:"sha256"`
	Expires time.Time `json:"expires"`
}
type downloadWork struct {
	Items []string `json:"items"`
	Name  string   `json:"name"`
}

func (f *featureRuntime) artifactDir() string { return f.m.journal.Path() + ".artifacts" }
func (f *featureRuntime) artifactPath(id string) string {
	return filepath.Join(f.artifactDir(), id+".zip")
}
func (f *featureRuntime) artifactSpace(limit int64) error {
	dir := f.artifactDir()
	if i, e := os.Lstat(dir); e == nil && (!i.IsDir() || i.Mode()&os.ModeSymlink != 0) {
		return os.ErrPermission
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	if e := os.Chmod(dir, 0700); e != nil {
		return e
	}
	list, e := os.ReadDir(dir)
	if e != nil {
		return e
	}
	used := int64(0)
	for _, i := range list {
		info, e := i.Info()
		if e != nil {
			return e
		}
		if time.Since(info.ModTime()) > 24*time.Hour {
			_ = os.Remove(filepath.Join(dir, i.Name()))
			continue
		}
		used += info.Size()
	}
	if used >= limit {
		return errors.New("prepared-download capacity reached")
	}
	return nil
}

type artifactWriter struct {
	ctx     context.Context
	file    *os.File
	limit   int64
	reserve int64
	written int64
	f       *featureRuntime
	id      string
	last    time.Time
}

func (w *artifactWriter) Write(p []byte) (int, error) {
	if e := w.ctx.Err(); e != nil {
		return 0, e
	}
	if int64(len(p)) > w.limit-w.written {
		return 0, errors.New("archive artifact size limit exceeded")
	}
	usage, e := disk.Usage(filepath.Dir(w.file.Name()))
	if e != nil {
		return 0, e
	}
	if usage.Free < uint64(len(p))+uint64(w.reserve) {
		return 0, errors.New("artifact storage free-space reserve reached")
	}
	n, e := w.file.Write(p)
	w.written += int64(n)
	if time.Since(w.last) > time.Second {
		w.f.progress(w.id, w.written, 0)
		w.last = time.Now()
	}
	return n, e
}
func (f *featureRuntime) buildArtifact(ctx context.Context, j *Job, d *data, p downloadWork) (artifact, error) {
	f.artifacts.Lock()
	defer f.artifacts.Unlock()
	if e := ctx.Err(); e != nil {
		return artifact{}, e
	}
	limits := d.settings.Work.Effective()
	path := f.artifactPath(j.ID)
	meta := path + ".json"
	if b, e := os.ReadFile(meta); e == nil {
		var a artifact
		if json.Unmarshal(b, &a) == nil && a.ID == j.ID && time.Now().Before(a.Expires) {
			if i, e := os.Stat(path); e == nil && i.Size() == a.Size {
				return a, nil
			}
		}
	}
	if e := f.artifactSpace(limits.ArtifactBytes); e != nil {
		return artifact{}, e
	}
	used := int64(0)
	entries, e := os.ReadDir(f.artifactDir())
	if e != nil {
		return artifact{}, e
	}
	for _, entry := range entries {
		i, e := entry.Info()
		if e != nil {
			return artifact{}, e
		}
		if entry.Name() != j.ID+".zip" && entry.Name() != j.ID+".zip.part" && entry.Name() != j.ID+".zip.json" {
			used += i.Size()
		}
	}
	available := limits.ArtifactBytes - used
	if available <= 0 {
		return artifact{}, errors.New("prepared-download capacity reached")
	}
	if e := f.phase(j, "packaging"); e != nil {
		return artifact{}, e
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(limits.ArchiveSeconds)*time.Second)
	defer cancel()
	all, e := collectArchiveFiles(ctx, d, p.Items, archiveCreateCommonDir(p.Items))
	if e != nil {
		return artifact{}, e
	}

	if len(all) == 0 {
		return artifact{}, os.ErrNotExist
	}
	out, e := os.OpenFile(path+".part", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return artifact{}, e
	}
	defer out.Close()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(path + ".part")
		}
	}()
	writer := &artifactWriter{ctx: ctx, file: out, limit: available, reserve: limits.ReserveBytes, f: f, id: j.ID}
	if e = (archives.Zip{}).Archive(ctx, writer, all); e != nil {
		return artifact{}, e
	}
	if e = out.Sync(); e != nil {
		return artifact{}, e
	}
	if e = out.Close(); e != nil {
		return artifact{}, e
	}
	if e = os.Rename(path+".part", path); e != nil {
		return artifact{}, e
	}
	fd, e := os.Open(path)
	if e != nil {
		return artifact{}, e
	}
	h := sha256.New()
	_, e = io.Copy(h, contextReader{ctx, fd})
	_ = fd.Close()
	if e != nil {
		return artifact{}, e
	}
	a := artifact{ID: j.ID, Name: filepath.Base(p.Name), Size: writer.written, SHA256: hex.EncodeToString(h.Sum(nil)), Expires: time.Now().Add(24 * time.Hour)}
	if a.Name == "" || a.Name == "." {
		a.Name = "download.zip"
	}
	b, _ := json.Marshal(a)
	if e = os.WriteFile(meta, b, 0600); e != nil {
		return artifact{}, e
	}
	ok = true
	return a, nil
}
func (f *featureRuntime) prepareDownload(ctx context.Context, j *Job, d *data) (any, error) {
	if !d.user.Perm.Download {
		return nil, os.ErrPermission
	}
	var p downloadWork
	if e := json.Unmarshal(j.Payload, &p); e != nil {
		return nil, e
	}
	return f.buildArtifact(ctx, j, d, p)
}

var downloadPrepareHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	if !d.user.Perm.Download {
		return http.StatusForbidden, nil
	}
	if d.features == nil {
		return http.StatusServiceUnavailable, nil
	}
	var p downloadWork
	if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&p); e != nil {
		return http.StatusBadRequest, e
	}
	paths, e := cleanArchiveCreateItems(p.Items, d)
	if e != nil {
		return errToStatus(e), e
	}
	p.Items = paths
	if !strings.HasSuffix(strings.ToLower(p.Name), ".zip") {
		p.Name = "download.zip"
	}
	j, e := d.features.submit(d, "download", p, nil)
	if e != nil {
		return errToStatus(e), e
	}
	w.WriteHeader(http.StatusAccepted)
	return renderJSON(w, r, j)
})
var downloadArtifactHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	if d.features == nil || !d.user.Perm.Download {
		return http.StatusForbidden, nil
	}
	j, e := d.features.get(mux.Vars(r)["id"])
	if e != nil || j.Kind != "download" || j.Owner != d.user.ID && !d.user.Perm.Admin {
		return http.StatusNotFound, nil
	}
	owner, e := d.features.jobData(j)
	if e != nil {
		return http.StatusForbidden, nil
	}
	var work downloadWork
	_ = json.Unmarshal(j.Payload, &work)
	for _, p := range work.Items {
		if !owner.Check(p) {
			return http.StatusForbidden, nil
		}
		if e := checkDescendants(owner, p, ""); e != nil {
			return http.StatusForbidden, nil
		}
	}
	var a artifact
	if j.State != "succeeded" || json.Unmarshal(j.Result, &a) != nil {
		return http.StatusConflict, nil
	}
	if time.Now().After(a.Expires) {
		return http.StatusGone, nil
	}
	file, e := os.Open(d.features.artifactPath(j.ID))
	if e != nil {
		return http.StatusGone, nil
	}
	defer file.Close()
	w.Header().Set("ETag", "\""+a.SHA256+"\"")
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+strings.ReplaceAll(a.Name, "\"", "")+"\"")
	http.ServeContent(w, r, a.Name, j.Updated, file)
	return 0, nil
})
