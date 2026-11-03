//go:build !linux

package files

import (
	"github.com/spf13/afero"
	"log"
	"os"
)

func publishNew(fs afero.Fs, src, dst string) error {
	if base := BasePath(fs); base != nil {
		if e := os.Link(afero.FullBaseFsPath(base, src), afero.FullBaseFsPath(base, dst)); e != nil {
			return e
		}
		_ = fs.Remove(src)
		return nil
	}
	return fs.Rename(src, dst)
}
func publicationMetadata(_ afero.Fs, _, _ string, _ bool) error { return nil }
func syncPublicationDirectory(fs afero.Fs, name string) {
	f, e := fs.Open(name)
	if e == nil {
		defer f.Close()
		if e = f.Sync(); e != nil {
			log.Printf("publication directory sync: %v", e)
		}
	}
}

func publicationPermissions(fs afero.Fs, stage, _ string, _ bool, mode os.FileMode) error {
	return fs.Chmod(stage, mode)
}
