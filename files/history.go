package files

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/spf13/afero"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Versions live beside the file on the same filesystem, behind the existing
// private-path protection. Old contents are copied, never hardlinked to a file
// that an external editor could still modify in place.
type PreviousVersion struct {
	ID      string      `json:"id"`
	Created time.Time   `json:"created"`
	Size    int64       `json:"size"`
	Version FileVersion `json:"version"`
	Path    string      `json:"-"`
}

const historyBudget int64 = 100 << 30

func historyPrefix(dest string) string {
	s := sha256.Sum256([]byte(filepath.Base(dest)))
	return hex.EncodeToString(s[:16]) + "-"
}
func historyDir(dest string) string {
	return filepath.Join(filepath.Dir(dest), StagingDirectory, "history")
}
func ListVersions(fs afero.Fs, dest string) ([]PreviousVersion, error) {
	fs = InternalFs(fs)
	dir := historyDir(dest)
	infos, e := afero.ReadDir(fs, dir)
	if os.IsNotExist(e) {
		return []PreviousVersion{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []PreviousVersion{}
	for _, i := range infos {
		if !strings.HasPrefix(i.Name(), historyPrefix(dest)) || !strings.HasSuffix(i.Name(), ".json") {
			continue
		}
		b, e := afero.ReadFile(fs, filepath.Join(dir, i.Name()))
		if e != nil {
			return nil, e
		}
		var v PreviousVersion
		if e = json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		v.Path = filepath.Join(dir, strings.TrimSuffix(i.Name(), ".json")+".data")
		if time.Since(v.Created) > 7*24*time.Hour {
			_ = fs.Remove(v.Path)
			_ = fs.Remove(filepath.Join(dir, i.Name()))
			continue
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}
func snapshotPrevious(fs afero.Fs, dest string, current FileVersion) error {
	if !current.Exists {
		return nil
	}
	fs = InternalFs(fs)
	dir := historyDir(dest)
	if i, _, e := Lstat(fs, dir); e == nil && (!i.IsDir() || i.Mode()&os.ModeSymlink != 0) {
		return os.ErrPermission
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	if e := fs.MkdirAll(dir, 0700); e != nil {
		return e
	}
	if e := fs.Chmod(dir, 0700); e != nil {
		return e
	}
	versions, e := ListVersions(fs, dest)
	if e != nil {
		return e
	}
	for n, v := range versions {
		if n >= 4 || time.Since(v.Created) > 7*24*time.Hour {
			if e = fs.Remove(v.Path); e != nil && !os.IsNotExist(e) {
				return e
			}
			if e = fs.Remove(strings.TrimSuffix(v.Path, ".data") + ".json"); e != nil && !os.IsNotExist(e) {
				return e
			}
		}
	}
	infos, e := afero.ReadDir(fs, dir)
	if e != nil {
		return e
	}
	used := int64(0)
	for _, i := range infos {
		used += i.Size()
	}
	if current.Size > historyBudget-used {
		return fmt.Errorf("previous-version capacity reached (%d bytes); remove old versions before replacing this file", historyBudget)
	}
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	name := filepath.Join(dir, historyPrefix(dest)+id)
	src, e := fs.Open(dest)
	if e != nil {
		return e
	}
	defer src.Close()
	out, e := fs.OpenFile(name+".data", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = fs.Remove(name + ".data")
			_ = fs.Remove(name + ".json")
		}
	}()
	n, e := io.Copy(out, src)
	if e != nil {
		return e
	}
	if n != current.Size {
		return ErrWriteConflict
	}
	if e = out.Sync(); e != nil {
		return e
	}
	if e = out.Close(); e != nil {
		return e
	}
	v := PreviousVersion{ID: id, Created: time.Now(), Size: n, Version: current}
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	meta, e := fs.OpenFile(name+".json", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = meta.Write(b)
	if e == nil {
		e = meta.Sync()
	}
	c := meta.Close()
	if e != nil {
		return e
	}
	if c != nil {
		return c
	}
	if before, e := Version(fs, dest); e != nil || before != current {
		return ErrWriteConflict
	}
	syncPublicationDirectory(fs, dir)
	ok = true
	return nil
}
