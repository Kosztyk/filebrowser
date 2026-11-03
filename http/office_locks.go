package fbhttp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/spf13/afero"
	bolt "go.etcd.io/bbolt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type officeLockRecord struct {
	Pending        *publicationIntent
	Scope, Path    string
	FollowExternal bool
	Lease          files.Lease
}

func (f *featureRuntime) restoreOfficeLocks() error {
	records := []officeLockRecord{}
	e := f.m.journal.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("office-locks-v1")).ForEach(func(_, v []byte) error {
			var l officeLockRecord
			if e := json.Unmarshal(v, &l); e != nil {
				return e
			}
			records = append(records, l)
			return nil
		})
	})
	if e != nil {
		return e
	}
	for _, l := range records {
		fs := files.NewFs(afero.NewOsFs(), l.Scope, l.FollowExternal)
		if l.Pending != nil {
			p := l.Pending
			if p.Path != l.Path || filepath.Dir(p.Stage) != filepath.Join(filepath.Dir(p.Path), files.StagingDirectory) || !strings.HasPrefix(filepath.Base(p.Stage), "office-") {
				return errors.New("invalid office save journal")
			}
			current, e := files.Version(fs, l.Path)
			if e != nil {
				return e
			}
			if current != p.Before {
				fd, e := fs.Open(l.Path)
				if e != nil {
					return e
				}
				h := sha256.New()
				_, e = io.Copy(h, fd)
				_ = fd.Close()
				if e != nil {
					return e
				}
				if hex.EncodeToString(h.Sum(nil)) == p.Digest {
					l.Lease.Before = current
				}
			}
			_ = files.InternalFs(fs).Remove(p.Stage)
			l.Pending = nil
			v, e := json.Marshal(l)
			if e != nil {
				return e
			}
			key := credentialHash(afero.FullBaseFsPath(files.BasePath(fs), l.Path))
			if e = f.m.journal.Update(func(tx *bolt.Tx) error { return tx.Bucket([]byte("office-locks-v1")).Put([]byte(key), v) }); e != nil {
				return e
			}
		}
		if time.Now().Before(l.Lease.Expires) {
			if e = files.SetLease(fs, l.Path, l.Lease); e != nil {
				return e
			}
		}
	}
	return nil
}

func officeKey(d *data, path string) string {
	return credentialHash(afero.FullBaseFsPath(files.BasePath(d.user.Fs), path))
}
func (f *featureRuntime) persistOfficeLock(d *data, path string, l files.Lease) error {
	old, had := files.GetLease(d.user.Fs, path)
	if e := files.SetLease(d.user.Fs, path, l); e != nil {
		return e
	}
	e := f.m.journal.Update(func(tx *bolt.Tx) error {
		v, e := json.Marshal(officeLockRecord{Scope: afero.FullBaseFsPath(files.BasePath(d.user.Fs), "/"), Path: path, FollowExternal: d.server.FollowExternalSymlinks, Lease: l})
		if e != nil {
			return e
		}
		return tx.Bucket([]byte("office-locks-v1")).Put([]byte(officeKey(d, path)), v)
	})
	if e != nil {
		files.ClearLease(d.user.Fs, path)
		if had {
			_ = files.SetLease(d.user.Fs, path, old)
		}
	}
	return e
}
func (f *featureRuntime) officeLock(w http.ResponseWriter, r *http.Request, d *data, c *wopiFileContext) (int, error) {
	f.office.Lock()
	defer f.office.Unlock()
	path := c.Claims.Path
	old, locked := files.GetLease(d.user.Fs, path)
	requested := r.Header.Get("X-WOPI-Lock")
	action := strings.ToUpper(r.Header.Get("X-WOPI-Override"))
	if action == "GET_LOCK" {
		w.Header().Set("X-WOPI-Lock", old.Value)
		return 0, nil
	}
	if action == "UNLOCK" {
		if !locked || old.Value != requested {
			return wopiLockMismatch(w, old.Value, "lock mismatch")
		}
		e := f.m.journal.Update(func(tx *bolt.Tx) error {
			return tx.Bucket([]byte("office-locks-v1")).Delete([]byte(officeKey(d, path)))
		})
		if e != nil {
			return http.StatusInternalServerError, e
		}
		files.ClearLease(d.user.Fs, path)
		return 0, nil
	}
	if action != "LOCK" && action != "REFRESH_LOCK" {
		return http.StatusNotImplemented, nil
	}
	if requested == "" || len(requested) > 1024 {
		return http.StatusBadRequest, errors.New("invalid X-WOPI-Lock")
	}
	expected := requested
	if r.Header.Get("X-WOPI-OldLock") != "" {
		expected = r.Header.Get("X-WOPI-OldLock")
	}
	if locked && old.Value != expected || !locked && (action == "REFRESH_LOCK" || r.Header.Get("X-WOPI-OldLock") != "") {
		return wopiLockMismatch(w, old.Value, "lock mismatch")
	}
	ttl, status, e := wopiLockTTL(r)
	if e != nil {
		return status, e
	}
	before, e := files.Version(d.user.Fs, path)
	if e != nil {
		return errToStatus(e), e
	}
	if locked {
		before = old.Before
	}
	e = f.persistOfficeLock(d, path, files.Lease{Value: requested, Expires: time.Now().Add(ttl), Before: before})
	if e != nil {
		return http.StatusConflict, e
	}
	return 0, nil
}
func (f *featureRuntime) saveOffice(r *http.Request, d *data, path string) error {
	lease, locked := files.GetLease(d.user.Fs, path)
	if !locked {
		_, e := writeFile(d.user.Fs, path, contextReader{r.Context(), r.Body}, d.settings.FileMode, d.settings.DirMode)
		return e
	}
	token := "office-" + officeKey(d, path)[:32]
	if e := files.InternalFs(d.user.Fs).Remove(filesStagePath(path, token)); e != nil && !os.IsNotExist(e) {
		return e
	}
	stage, out, e := files.NewStagedFile(d.user.Fs, path, token, d.settings.FileMode, d.settings.DirMode)
	if e != nil {
		return e
	}
	defer func() { _ = out.Close(); _ = stage.Abort() }()
	if stage.Before != lease.Before {
		return files.ErrWriteConflict
	}
	h := sha256.New()
	if _, e = io.Copy(io.MultiWriter(out, h), contextReader{r.Context(), r.Body}); e != nil {
		return e
	}
	if e = out.Sync(); e != nil {
		return e
	}
	if e = out.Close(); e != nil {
		return e
	}
	if e = r.Context().Err(); e != nil {
		return e
	}
	u, e := d.store.Users.Get(d.server.Root, d.server.FollowExternalSymlinks, d.user.ID)
	if e != nil || !u.Perm.Modify || afero.FullBaseFsPath(files.BasePath(u.Fs), "/") != afero.FullBaseFsPath(files.BasePath(d.user.Fs), "/") {
		return os.ErrPermission
	}
	intent := &publicationIntent{Path: path, Stage: stage.Path, Digest: hex.EncodeToString(h.Sum(nil)), Before: stage.Before}
	e = f.m.journal.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("office-locks-v1"))
		key := []byte(officeKey(d, path))
		var record officeLockRecord
		if e := json.Unmarshal(b.Get(key), &record); e != nil {
			return e
		}
		record.Pending = intent
		v, e := json.Marshal(record)
		if e != nil {
			return e
		}
		return b.Put(key, v)
	})
	if e != nil {
		return e
	}
	if _, e = stage.Commit(); e != nil {
		return e
	}
	lease.Before, e = files.Version(d.user.Fs, path)
	if e != nil {
		return e
	}
	return f.persistOfficeLock(d, path, lease)
}
