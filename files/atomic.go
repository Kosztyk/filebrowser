package files

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/spf13/afero"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
)

const StagingDirectory = ".filebrowser-staging"

var ErrWriteConflict = errors.New("destination changed or has an active upload")

func IsPrivatePath(name string) bool {
	for _, p := range strings.Split(strings.ReplaceAll(name, "\\", "/"), "/") {
		if strings.EqualFold(p, StagingDirectory) {
			return true
		}
	}
	return false
}
func InternalFs(fs afero.Fs) afero.Fs {
	if s, ok := fs.(*ScopedFs); ok {
		c := *s
		c.private = true
		return &c
	}
	return fs
}
func Lstat(fs afero.Fs, name string) (os.FileInfo, bool, error) {
	if s, ok := fs.(afero.Lstater); ok {
		return s.LstatIfPossible(name)
	}
	i, e := fs.Stat(name)
	return i, false, e
}
func statNumber(i os.FileInfo, name string) int64 {
	v := reflect.Indirect(reflect.ValueOf(i.Sys()))
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return -1
	}
	f := v.FieldByName(name)
	if !f.IsValid() {
		return -1
	}
	switch f.Kind() {
	case reflect.Uint, reflect.Uint32, reflect.Uint64:
		return int64(f.Uint())
	case reflect.Int, reflect.Int32, reflect.Int64:
		return f.Int()
	}
	return -1
}
func DeviceID(i os.FileInfo) string { return fmt.Sprintf("%T:%d", i.Sys(), statNumber(i, "Dev")) }

type FileVersion struct {
	Exists   bool
	Size     int64
	Modified int64
	Mode     uint32
	Identity string
	UID, GID int
}

func Version(fs afero.Fs, name string) (FileVersion, error) {
	i, _, e := Lstat(fs, name)
	if os.IsNotExist(e) {
		return FileVersion{}, nil
	}
	if e != nil {
		return FileVersion{}, e
	}
	if !i.Mode().IsRegular() {
		return FileVersion{}, fmt.Errorf("destination must be a regular file: %w", ErrWriteConflict)
	}
	identity := ""
	if n := statNumber(i, "Ino"); n >= 0 {
		identity = fmt.Sprintf("%d:%d", statNumber(i, "Dev"), n)
	}
	return FileVersion{true, i.Size(), i.ModTime().UnixNano(), uint32(i.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)), identity, int(statNumber(i, "Uid")), int(statNumber(i, "Gid"))}, nil
}

var writes = struct {
	sync.Mutex
	paths map[string]string
}{paths: map[string]string{}}

func writeKey(fs afero.Fs, name string) string {
	if base := BasePath(fs); base != nil {
		name = afero.FullBaseFsPath(base, name)
		parent := filepath.Dir(name)
		tail := []string{filepath.Base(name)}
		for {
			resolved, err := filepath.EvalSymlinks(parent)
			if err == nil {
				for i := len(tail) - 1; i >= 0; i-- {
					resolved = filepath.Join(resolved, tail[i])
				}
				name = resolved
				break
			}
			if !os.IsNotExist(err) || filepath.Dir(parent) == parent {
				break
			}
			tail = append(tail, filepath.Base(parent))
			parent = filepath.Dir(parent)
		}
	} else {
		name = fmt.Sprintf("%p:%s", fs, name)
	}
	return strings.ToLower(filepath.Clean(name))
}
func overlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+string(filepath.Separator)) || strings.HasPrefix(b, a+string(filepath.Separator))
}
func ReserveWrite(fs afero.Fs, name, token string) error {
	writes.Lock()
	defer writes.Unlock()
	if e := checkLease(fs, name); e != nil {
		return e
	}
	key := writeKey(fs, name)
	for p, t := range writes.paths {
		if overlap(p, key) && t != token {
			return ErrWriteConflict
		}
	}
	writes.paths[key] = token
	return nil
}
func ReleaseWrite(fs afero.Fs, name, token string) {
	writes.Lock()
	defer writes.Unlock()
	key := writeKey(fs, name)
	if writes.paths[key] == token {
		delete(writes.paths, key)
	}
}
func CheckMutation(fs afero.Fs, name string) error {
	writes.Lock()
	defer writes.Unlock()
	if e := checkLease(fs, name); e != nil {
		return e
	}
	for p := range writes.paths {
		if overlap(p, writeKey(fs, name)) {
			return ErrWriteConflict
		}
	}
	return nil
}

// GuardMutation excludes creation of a new reservation until a destructive
// operation has completed. The callback must not call staged publication.
func GuardMutation(fs afero.Fs, names []string, action func() error) error {
	writes.Lock()
	defer writes.Unlock()
	for _, name := range names {
		if e := checkLease(fs, name); e != nil {
			return e
		}
		for p := range writes.paths {
			if overlap(p, writeKey(fs, name)) {
				return ErrWriteConflict
			}
		}
	}
	return action()
}

type StagedFile struct {
	Fs                       afero.Fs
	Path, Destination, Token string
	Before                   FileVersion
	FileMode                 os.FileMode
}

func NewStagedFile(fs afero.Fs, dest, token string, mode, dirMode os.FileMode) (*StagedFile, afero.File, error) {
	if IsPrivatePath(dest) || token == "" || strings.ContainsAny(token, "/\\.") {
		return nil, nil, os.ErrPermission
	}
	if e := ReserveWrite(fs, dest, token); e != nil {
		return nil, nil, e
	}
	fail := func(e error) (*StagedFile, afero.File, error) { ReleaseWrite(fs, dest, token); return nil, nil, e }
	before, e := Version(fs, dest)
	if e != nil {
		return fail(e)
	}
	parent := filepath.Dir(dest)
	if e = fs.MkdirAll(parent, dirMode); e != nil {
		return fail(e)
	}
	internal := InternalFs(fs)
	directory := filepath.Join(parent, StagingDirectory)
	if i, _, err := Lstat(internal, directory); err == nil {
		if !i.IsDir() || i.Mode()&os.ModeSymlink != 0 {
			return fail(os.ErrPermission)
		}
	} else if !os.IsNotExist(err) {
		return fail(err)
	}
	if e = internal.MkdirAll(directory, 0700); e != nil {
		return fail(e)
	}
	i, e := internal.Stat(directory)
	if e != nil {
		return fail(e)
	}
	if e = internal.Chmod(directory, 0700|(i.Mode()&os.ModeSetgid)); e != nil {
		return fail(e)
	}
	path := filepath.Join(directory, token+".part")
	f, e := internal.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		return fail(e)
	}
	for _, name := range []string{directory, parent} {
		dir, err := internal.Open(name)
		if err == nil {
			err = dir.Sync()
			_ = dir.Close()
		}
		if err != nil {
			_ = f.Close()
			_ = internal.Remove(path)
			return fail(err)
		}
	}
	return &StagedFile{internal, path, dest, token, before, mode}, f, nil
}
func (s *StagedFile) Abort() error {
	defer ReleaseWrite(s.Fs, s.Destination, s.Token)
	e := s.Fs.Remove(s.Path)
	if os.IsNotExist(e) {
		return nil
	}
	return e
}
func (s *StagedFile) Commit() (os.FileInfo, error) {
	writes.Lock()
	err := func() error {
		if e := checkLease(s.Fs, s.Destination); e != nil {
			return e
		}
		if writes.paths[writeKey(s.Fs, s.Destination)] != s.Token {
			return ErrWriteConflict
		}
		return nil
	}()
	writes.Unlock()
	if err != nil {
		return nil, err
	}
	current, e := Version(s.Fs, s.Destination)
	if e != nil {
		return nil, e
	}
	if current != s.Before {
		return nil, ErrWriteConflict
	}
	mode := s.FileMode
	if current.Exists {
		mode = os.FileMode(current.Mode)
		i, err := s.Fs.Stat(s.Path)
		if err != nil {
			return nil, err
		}
		if current.UID >= 0 && current.GID >= 0 && (statNumber(i, "Uid") != int64(current.UID) || statNumber(i, "Gid") != int64(current.GID)) {
			if e = s.Fs.Chown(s.Path, current.UID, current.GID); e != nil {
				return nil, e
			}
		}
	}
	if e = publicationMetadata(s.Fs, s.Path, s.Destination, current.Exists); e != nil {
		return nil, e
	}
	if e = publicationPermissions(s.Fs, s.Path, s.Destination, current.Exists, mode); e != nil {
		return nil, e
	}
	f, e := s.Fs.OpenFile(s.Path, os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	e = f.Sync()
	closeErr := f.Close()
	if e != nil {
		return nil, e
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if current.Exists {
		if e = snapshotPrevious(s.Fs, s.Destination, current); e != nil {
			return nil, e
		}
	}
	writes.Lock()
	defer writes.Unlock()
	key := writeKey(s.Fs, s.Destination)
	if writes.paths[key] != s.Token {
		return nil, ErrWriteConflict
	}
	if e = checkLease(s.Fs, s.Destination); e != nil {
		return nil, e
	}
	latest, e := Version(s.Fs, s.Destination)
	if e != nil {
		return nil, e
	}
	if latest != current {
		return nil, ErrWriteConflict
	}
	if current.Exists {
		e = s.Fs.Rename(s.Path, s.Destination)
	} else {
		e = publishNew(s.Fs, s.Path, s.Destination)
	}
	if e != nil {
		return nil, e
	}
	delete(writes.paths, key)
	// A publication is already visible after rename. Report sync failures in logs
	// without deleting the committed destination or claiming cancellation succeeded.
	syncPublicationDirectory(s.Fs, filepath.Dir(s.Destination))
	return s.Fs.Stat(s.Destination)
}
func AtomicWrite(fs afero.Fs, dest string, in io.Reader, mode, dirMode os.FileMode) (os.FileInfo, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return nil, e
	}
	s, f, e := NewStagedFile(fs, dest, hex.EncodeToString(b), mode, dirMode)
	if e != nil {
		return nil, e
	}
	defer func() { _ = s.Abort() }()
	_, e = io.Copy(f, in)
	if e == nil {
		e = f.Sync()
	}
	c := f.Close()
	if e != nil {
		return nil, e
	}
	if c != nil {
		return nil, c
	}
	return s.Commit()
}
