package files

import (
	"errors"
	"github.com/spf13/afero"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type interruptedReader struct{ sent bool }

func (r *interruptedReader) Read(p []byte) (int, error) {
	if r.sent {
		return 0, io.ErrUnexpectedEOF
	}
	r.sent = true
	return copy(p, "partial"), nil
}
func TestAtomicReplacementPreservesPrevious(t *testing.T) {
	root := t.TempDir()
	fs := NewFs(afero.NewOsFs(), root, false)
	name := filepath.Join(root, "file")
	if e := os.WriteFile(name, []byte("old"), 0640); e != nil {
		t.Fatal(e)
	}
	if _, e := AtomicWrite(fs, "/file", &interruptedReader{}, 0644, 0755); e == nil {
		t.Fatal("interrupted write accepted")
	}
	b, _ := os.ReadFile(name)
	if string(b) != "old" {
		t.Fatal("previous file lost")
	}
	if _, e := AtomicWrite(fs, "/file", strings.NewReader("new"), 0644, 0755); e != nil {
		t.Fatal(e)
	}
	i, _ := os.Stat(name)
	if i.Mode().Perm() != 0640 {
		t.Fatal("mode lost")
	}
}
func TestStagingReservationAndChangedDestination(t *testing.T) {
	root := t.TempDir()
	fs := NewFs(afero.NewOsFs(), root, false)
	s, f, e := NewStagedFile(fs, "/folder/file", "one", 0644, 0755)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = s.Abort() }()
	if _, e = f.WriteString("new"); e != nil {
		t.Fatal(e)
	}
	_ = f.Close()
	if _, _, e = NewStagedFile(fs, "/folder/file", "two", 0644, 0755); !errors.Is(e, ErrWriteConflict) {
		t.Fatal("concurrent write allowed", e)
	}
	if e = CheckMutation(fs, "/folder"); !errors.Is(e, ErrWriteConflict) {
		t.Fatal("ancestor deletion allowed")
	}
	if e = os.WriteFile(filepath.Join(root, "folder/file"), []byte("external"), 0644); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Commit(); !errors.Is(e, ErrWriteConflict) {
		t.Fatal("newer file overwritten", e)
	}
}
func TestPrivateStagingBlocksDirectAndAliasReads(t *testing.T) {
	for _, follow := range []bool{false, true} {
		root := t.TempDir()
		fs := NewFs(afero.NewOsFs(), root, follow)
		s, f, e := NewStagedFile(fs, "/file", "secret", 0644, 0755)
		if e != nil {
			t.Fatal(e)
		}
		_ = f.Close()
		defer func() { _ = s.Abort() }()
		if e = os.Symlink(filepath.Join(root, StagingDirectory), filepath.Join(root, "alias")); e != nil {
			t.Fatal(e)
		}
		for _, name := range []string{s.Path, "/alias/secret.part"} {
			if _, e = fs.Open(name); !os.IsPermission(e) {
				t.Fatal("private stage accessible", name, e)
			}
		}
		entries, e := afero.ReadDir(fs, "/")
		if e != nil {
			t.Fatal(e)
		}
		for _, i := range entries {
			if i.Name() == StagingDirectory {
				t.Fatal("staging listed")
			}
		}
	}
}

type syncFailureFs struct{ afero.Fs }
type syncFailureFile struct{ afero.File }

func (f *syncFailureFile) Sync() error { return errors.New("simulated storage sync failure") }
func (f syncFailureFs) OpenFile(n string, flag int, mode os.FileMode) (afero.File, error) {
	v, e := f.Fs.OpenFile(n, flag, mode)
	if e != nil {
		return nil, e
	}
	return &syncFailureFile{v}, nil
}
func TestFailedSyncPreservesPrevious(t *testing.T) {
	base := afero.NewMemMapFs()
	if e := afero.WriteFile(base, "/file", []byte("old"), 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := AtomicWrite(syncFailureFs{base}, "/file", strings.NewReader("new"), 0644, 0755); e == nil {
		t.Fatal("sync failure ignored")
	}
	b, _ := afero.ReadFile(base, "/file")
	if string(b) != "old" {
		t.Fatal("previous file lost")
	}
}
