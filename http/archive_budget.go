package fbhttp

import (
	"context"
	"errors"
	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/spf13/afero"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type extractionWork struct {
	Path, Inner, Destination string
	Override                 bool
}
type extractionBudget struct {
	ctx            context.Context
	limits         settings.Work
	bytes, entries int64
	d              *data
}

func (b *extractionBudget) entry(path string) error {
	if e := b.ctx.Err(); e != nil {
		return e
	}
	b.entries++
	if b.entries > b.limits.ArchiveEntries {
		return errors.New("archive entry-count budget exceeded")
	}
	if len(strings.Split(path, "/")) > b.limits.ArchiveDepth {
		return errors.New("archive depth budget exceeded")
	}
	return nil
}
func checkFreeSpace(d *data, path string, needed, reserve int64) error {
	base := files.BasePath(d.user.Fs)
	if base == nil {
		return nil
	}
	physicalPath := afero.FullBaseFsPath(base, path)
	for {
		_, e := os.Stat(physicalPath)
		if e == nil {
			break
		}
		if !os.IsNotExist(e) || filepath.Dir(physicalPath) == physicalPath {
			return e
		}
		physicalPath = filepath.Dir(physicalPath)
	}
	usage, e := disk.Usage(physicalPath)
	if e != nil {
		return e
	}
	if needed < 0 || uint64(needed) > usage.Free || uint64(reserve) > usage.Free-uint64(needed) {
		return errors.New("free-space reserve would be exceeded")
	}
	return nil
}

type extractionReader struct {
	b    *extractionBudget
	r    io.Reader
	path string
}

func (r extractionReader) Read(p []byte) (int, error) {
	if e := r.b.ctx.Err(); e != nil {
		return 0, e
	}
	remaining := r.b.limits.ArchiveBytes - r.b.bytes
	if remaining <= 0 {
		var one [1]byte
		n, e := r.r.Read(one[:])
		if n == 0 && e == io.EOF {
			return 0, e
		}
		return 0, errors.New("archive expanded-size budget exceeded")
	}
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}
	if e := checkFreeSpace(r.b.d, r.path, int64(len(p)), r.b.limits.ReserveBytes); e != nil {
		return 0, e
	}
	n, e := r.r.Read(p)
	r.b.bytes += int64(n)
	return n, e
}
