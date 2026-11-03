//go:build linux

package files

import (
	"errors"
	"github.com/spf13/afero"
	"golang.org/x/sys/unix"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func publishNew(fs afero.Fs, src, dst string) error {
	base := BasePath(fs)
	if base == nil {
		return fs.Rename(src, dst)
	}
	if _, e := fs.Stat(src); e != nil {
		return e
	}
	if _, e := fs.Stat(filepath.Dir(dst)); e != nil {
		return e
	}
	a, b := afero.FullBaseFsPath(base, src), afero.FullBaseFsPath(base, dst)
	e := unix.Renameat2(unix.AT_FDCWD, a, unix.AT_FDCWD, b, unix.RENAME_NOREPLACE)
	if errors.Is(e, unix.ENOSYS) || errors.Is(e, unix.EINVAL) || errors.Is(e, unix.EOPNOTSUPP) {
		if e = os.Link(a, b); e == nil {
			_ = os.Remove(a)
		}
	}
	return e
}
func publicationMetadata(fs afero.Fs, src, dst string, exists bool) error {
	base := BasePath(fs)
	if base == nil {
		return nil
	}
	source := afero.FullBaseFsPath(base, dst)
	if !exists {
		source = filepath.Dir(source)
	}
	n, e := unix.Listxattr(source, nil)
	if errors.Is(e, unix.ENOTSUP) {
		return nil
	}
	if e != nil {
		return e
	}
	buf := make([]byte, n)
	n, e = unix.Listxattr(source, buf)
	if e != nil {
		return e
	}
	for _, name := range strings.Split(string(buf[:n]), "\x00") {
		target := name
		if exists {
			if !strings.HasPrefix(name, "user.") && name != "system.posix_acl_access" {
				continue
			}
		} else {
			if name != "system.posix_acl_default" {
				continue
			}
			target = "system.posix_acl_access"
		}
		n, e = unix.Getxattr(source, name, nil)
		if e != nil {
			return e
		}
		v := make([]byte, n)
		n, e = unix.Getxattr(source, name, v)
		if e != nil {
			return e
		}
		if e = unix.Setxattr(afero.FullBaseFsPath(base, src), target, v[:n], 0); e != nil {
			return e
		}
	}
	return nil
}
func syncPublicationDirectory(fs afero.Fs, name string) {
	f, e := fs.Open(name)
	if e != nil {
		log.Printf("publication directory open: %v", e)
		return
	}
	defer f.Close()
	if e = f.Sync(); e != nil {
		log.Printf("publication directory sync: %v", e)
	}
}

// New-file permissions are intersected with the parent's default POSIX ACL,
// matching creation in that directory. Chmod must not widen inherited access.
func publicationPermissions(fs afero.Fs, stage, dest string, exists bool, mode os.FileMode) error {
	if !exists {
		if base := BasePath(fs); base != nil {
			parent := filepath.Dir(afero.FullBaseFsPath(base, dest))
			n, err := unix.Getxattr(parent, "system.posix_acl_default", nil)
			if err == nil && n > 0 {
				info, e := fs.Stat(stage)
				if e != nil {
					return e
				}
				mode = (mode &^ os.ModePerm) | (mode.Perm() & info.Mode().Perm())
			} else if err != nil && !errors.Is(err, unix.ENODATA) && !errors.Is(err, unix.ENOTSUP) {
				return err
			}
		}
	}
	return fs.Chmod(stage, mode)
}
