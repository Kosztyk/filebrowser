//go:build linux

package files

import (
	"encoding/binary"
	"errors"
	"github.com/spf13/afero"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAtomicReplacementPreservesUserXattrs(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, "file")
	if e := os.WriteFile(name, []byte("old"), 0640); e != nil {
		t.Fatal(e)
	}
	if e := unix.Setxattr(name, "user.filebrowser-test", []byte("keep"), 0); errors.Is(e, unix.ENOTSUP) {
		t.Skip("xattrs unsupported")
	} else if e != nil {
		t.Fatal(e)
	}
	if _, e := AtomicWrite(NewFs(afero.NewOsFs(), root, false), "/file", strings.NewReader("new"), 0644, 0755); e != nil {
		t.Fatal(e)
	}
	v := make([]byte, 16)
	n, e := unix.Getxattr(name, "user.filebrowser-test", v)
	if e != nil || string(v[:n]) != "keep" {
		t.Fatal("xattr lost", e)
	}
}
func TestNewPublicationDoesNotWidenDefaultACL(t *testing.T) {
	root := t.TempDir()
	acl := make([]byte, 44)
	binary.LittleEndian.PutUint32(acl, 2)
	entries := []struct {
		tag, perm uint16
		id        uint32
	}{{1, 6, 0xffffffff}, {2, 4, uint32(os.Getuid())}, {4, 0, 0xffffffff}, {16, 4, 0xffffffff}, {32, 0, 0xffffffff}}
	for i, e := range entries {
		p := acl[4+i*8:]
		binary.LittleEndian.PutUint16(p, e.tag)
		binary.LittleEndian.PutUint16(p[2:], e.perm)
		binary.LittleEndian.PutUint32(p[4:], e.id)
	}
	if e := unix.Setxattr(root, "system.posix_acl_default", acl, 0); errors.Is(e, unix.ENOTSUP) {
		t.Skip("POSIX ACLs unsupported")
	} else if e != nil {
		t.Fatal(e)
	}
	fs := NewFs(afero.NewOsFs(), root, false)
	if _, e := AtomicWrite(fs, "/file", strings.NewReader("new"), 0644, 0755); e != nil {
		t.Fatal(e)
	}
	info, e := os.Stat(filepath.Join(root, "file"))
	if e != nil {
		t.Fatal(e)
	}
	if info.Mode().Perm() != 0640 {
		t.Fatalf("inherited ACL widened: %o", info.Mode().Perm())
	}
	value := make([]byte, 128)
	n, e := unix.Getxattr(filepath.Join(root, "file"), "system.posix_acl_access", value)
	if e != nil || string(value[:n]) != string(acl) {
		t.Fatal("inherited named ACL lost", e)
	}
	if _, e = AtomicWrite(fs, "/file", strings.NewReader("replacement"), 0666, 0755); e != nil {
		t.Fatal(e)
	}
	n, e = unix.Getxattr(filepath.Join(root, "file"), "system.posix_acl_access", value)
	if e != nil || string(value[:n]) != string(acl) {
		t.Fatal("replacement ACL lost", e)
	}
}
