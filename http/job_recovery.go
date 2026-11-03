package fbhttp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/filebrowser/filebrowser/v2/files"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type publicationIntent struct {
	Path, Stage, Digest string
	Before              files.FileVersion
}

func filesStagePath(dest, token string) string {
	return filepath.Join(filepath.Dir(dest), files.StagingDirectory, token+".part")
}
func (f *featureRuntime) recordPublication(j *Job, stage *files.StagedFile, draft any) error {
	fd, e := stage.Fs.Open(stage.Path)
	if e != nil {
		return e
	}
	h := sha256.New()
	_, e = io.Copy(h, fd)
	_ = fd.Close()
	if e != nil {
		return e
	}
	intent := &publicationIntent{stage.Destination, stage.Path, hex.EncodeToString(h.Sum(nil)), stage.Before}
	raw, _ := json.Marshal(draft)
	return f.change(j.ID, func(c *Job) error {
		if c.State == "cancelled" {
			return context.Canceled
		}
		c.Pending = intent
		c.Draft = raw
		c.Phase = "publishing"
		return nil
	})
}
func (f *featureRuntime) finishPublication(id, path string, version files.FileVersion) error {
	return f.change(id, func(j *Job) error {
		if j.Published == nil {
			j.Published = map[string]files.FileVersion{}
		}
		j.Published[path] = version
		j.Pending = nil
		if j.Kind == "extract" {
			j.Completed = int64(len(j.Published))
			j.Phase = "extracting"
		} else {
			j.State = "succeeded"
			j.Phase = "complete"
			j.Result = j.Draft
		}
		return nil
	})
}
func (f *featureRuntime) reconcilePublication(j Job) error {
	if j.Pending == nil {
		return nil
	}
	p := j.Pending
	if files.IsPrivatePath(p.Path) || filepath.Dir(p.Stage) != filepath.Join(filepath.Dir(p.Path), files.StagingDirectory) || !strings.HasSuffix(filepath.Base(p.Stage), ".part") || !strings.Contains(filepath.Base(p.Stage), j.ID) {
		return errors.New("invalid publication recovery path")
	}
	d, e := f.jobData(j)
	if e != nil {
		return e
	}
	current, e := files.Version(d.user.Fs, p.Path)
	if e != nil {
		return e
	}
	published := false
	if current != p.Before && current.Exists {
		fd, e := d.user.Fs.Open(p.Path)
		if e != nil {
			return e
		}
		h := sha256.New()
		_, e = io.Copy(h, fd)
		_ = fd.Close()
		if e != nil {
			return e
		}
		published = hex.EncodeToString(h.Sum(nil)) == p.Digest
	}
	if !published && current != p.Before {
		return files.ErrWriteConflict
	}
	if e = files.InternalFs(d.user.Fs).Remove(p.Stage); e != nil && !os.IsNotExist(e) {
		return e
	}
	return f.change(j.ID, func(c *Job) error {
		if published {
			if c.Published == nil {
				c.Published = map[string]files.FileVersion{}
			}
			c.Published[p.Path] = current
			if c.Kind != "extract" {
				c.State = "succeeded"
				c.Result = c.Draft
				c.Phase = "complete"
			} else {
				c.State = "queued"
				c.Phase = "extracting"
				c.Completed = int64(len(c.Published))
			}
		} else {
			c.State = "queued"
			c.Phase = "queued"
		}
		c.Pending = nil
		return nil
	})
}
func jobForContext(ctx context.Context) (*featureRuntime, string) {
	if p, ok := ctx.Value(progressContextKey{}).(*jobProgress); ok {
		return p.f, p.id
	}
	return nil, ""
}
func extractAlreadyPublished(ctx context.Context, d *data, path string) (bool, error) {
	f, id := jobForContext(ctx)
	if f == nil {
		return false, nil
	}
	j, e := f.get(id)
	if e != nil {
		return false, e
	}
	before, ok := j.Published[path]
	if !ok {
		return false, nil
	}
	current, e := files.Version(d.user.Fs, path)
	if e != nil || current != before {
		return false, files.ErrWriteConflict
	}
	return true, nil
}
func writeExtracted(ctx context.Context, d *data, path string, in io.Reader) error {
	f, id := jobForContext(ctx)
	if f == nil {
		_, e := writeFile(d.user.Fs, path, in, d.settings.FileMode, d.settings.DirMode)
		return e
	}
	j, e := f.get(id)
	if e != nil {
		return e
	}
	fresh, e := f.jobData(j)
	if e != nil {
		return e
	}
	if !fresh.user.Perm.Create || !fresh.CheckRules(path) {
		return os.ErrPermission
	}
	current, e := files.Version(fresh.user.Fs, path)
	if e != nil {
		return e
	}
	if current.Exists && !fresh.user.Perm.Modify {
		return os.ErrPermission
	}
	token := "extract-" + id + "-" + credentialHash(path)[:16]
	if e = files.InternalFs(fresh.user.Fs).Remove(filesStagePath(path, token)); e != nil && !os.IsNotExist(e) {
		return e
	}
	stage, out, e := files.NewStagedFile(fresh.user.Fs, path, token, d.settings.FileMode, d.settings.DirMode)
	if e != nil {
		return e
	}
	defer func() { _ = out.Close(); _ = stage.Abort() }()
	if _, e = io.Copy(out, contextReader{ctx, in}); e != nil {
		return e
	}
	if e = out.Sync(); e != nil {
		return e
	}
	if e = out.Close(); e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if e = f.recordPublication(&j, stage, nil); e != nil {
		return e
	}
	if _, e = stage.Commit(); e != nil {
		return e
	}
	v, e := files.Version(fresh.user.Fs, path)
	if e != nil {
		return e
	}
	return f.finishPublication(id, path, v)
}
func (f *featureRuntime) publishJob(ctx context.Context, j *Job, d *data, dest string, in io.Reader) error {
	fresh, e := f.jobData(*j)
	if e != nil {
		return e
	}
	if !fresh.user.Perm.Create || !fresh.Check(dest) {
		return os.ErrPermission
	}
	before, ok := j.Versions[dest]
	if !ok {
		return files.ErrWriteConflict
	}
	if before.Exists && !fresh.user.Perm.Modify {
		return os.ErrPermission
	}
	token := "job-" + j.ID
	if e = files.InternalFs(fresh.user.Fs).Remove(filesStagePath(dest, token)); e != nil && !os.IsNotExist(e) {
		return e
	}
	stage, out, e := files.NewStagedFile(fresh.user.Fs, dest, token, d.settings.FileMode, d.settings.DirMode)
	if e != nil {
		return e
	}
	defer func() { _ = out.Close(); _ = stage.Abort() }()
	if stage.Before != before {
		return files.ErrWriteConflict
	}
	if _, e = io.Copy(out, contextReader{ctx, in}); e != nil {
		return e
	}
	if e = out.Sync(); e != nil {
		return e
	}
	if e = out.Close(); e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	var p convertXConvertPlan
	_ = json.Unmarshal(j.Payload, &p)
	draft := convertXConvertResponse{JobID: j.ID, Status: "done", Source: p.Source, Destination: dest, ConvertTo: p.ConvertTo, Converter: p.Converter, Done: true}
	if e = f.recordPublication(j, stage, draft); e != nil {
		return e
	}
	if _, e = stage.Commit(); e != nil {
		return e
	}
	v, e := files.Version(fresh.user.Fs, dest)
	if e != nil {
		return e
	}
	return f.finishPublication(j.ID, dest, v)
}
