package fbhttp

import (
	"context"
	"errors"
	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/mholt/archives"
	"github.com/spf13/afero"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type archiveReadBudget struct {
	ctx        context.Context
	bytes, max int64
}
type checkedArchiveReader struct {
	fs.File
	budget *archiveReadBudget
	before os.FileInfo
}

func (r *checkedArchiveReader) Read(p []byte) (int, error) {
	if e := r.budget.ctx.Err(); e != nil {
		return 0, e
	}
	remaining := r.budget.max - r.budget.bytes
	if remaining <= 0 {
		var one [1]byte
		n, e := r.File.Read(one[:])
		if n == 0 && e == io.EOF {
			return 0, e
		}
		return 0, errors.New("archive source byte budget exceeded")
	}
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}
	n, e := r.File.Read(p)
	r.budget.bytes += int64(n)
	if e == io.EOF {
		after, err := r.Stat()
		if err != nil || after.Size() != r.before.Size() || !after.ModTime().Equal(r.before.ModTime()) {
			return n, files.ErrWriteConflict
		}
	}
	return n, e
}
func collectArchiveFiles(ctx context.Context, d *data, paths []string, common string) ([]archives.FileInfo, error) {
	limits := d.settings.Work.Effective()
	budget := &archiveReadBudget{ctx: ctx, max: limits.ArchiveBytes}
	out := []archives.FileInfo{}
	visited := map[string]bool{}
	type node struct {
		path  string
		depth int
	}
	pending := []node{}
	for _, p := range paths {
		pending = append(pending, node{p, 0})
	}
	metadataBytes := 0
	for len(pending) > 0 {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		n := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if n.depth > limits.ArchiveDepth {
			return nil, errors.New("archive source depth limit exceeded")
		}
		if !d.Check(n.path) {
			continue
		}
		info, e := d.user.Fs.Stat(n.path)
		if e != nil {
			return nil, e
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			continue
		}
		physical := n.path
		if base := files.BasePath(d.user.Fs); base != nil {
			physical = afero.FullBaseFsPath(base, n.path)
			if resolved, e := filepath.EvalSymlinks(physical); e == nil {
				physical = resolved
			}
		}
		if info.IsDir() {
			if visited[physical] {
				continue
			}
			visited[physical] = true
		}
		if n.path != common {
			name := strings.TrimPrefix(strings.TrimPrefix(n.path, common), string(filepath.Separator))
			name = filepath.ToSlash(strings.ReplaceAll(name, "\\", "_"))
			if filepath.IsAbs(name) || filepath.Clean(name) != name || strings.HasPrefix(name, "../") {
				return nil, os.ErrPermission
			}
			metadataBytes += len(name) + 512
			if int64(len(out)) >= limits.ArchiveEntries || metadataBytes > 128<<20 {
				return nil, errors.New("archive source count or metadata budget exceeded")
			}
			path := n.path
			before := info
			out = append(out, archives.FileInfo{FileInfo: info, NameInArchive: name, Open: func() (fs.File, error) {
				if e := ctx.Err(); e != nil {
					return nil, e
				}
				fd, e := d.user.Fs.Open(path)
				if e != nil {
					return nil, e
				}
				current, e := fd.Stat()
				if e != nil || current.Size() != before.Size() || !current.ModTime().Equal(before.ModTime()) {
					_ = fd.Close()
					return nil, files.ErrWriteConflict
				}
				reportJobProgress(ctx, 1, 0)
				return &checkedArchiveReader{fd, budget, before}, nil
			}})
		}
		if info.IsDir() {
			dir, e := d.user.Fs.Open(n.path)
			if e != nil {
				return nil, e
			}
			for {
				entries, e := dir.Readdir(256)
				for _, entry := range entries {
					pending = append(pending, node{filepath.Join(n.path, entry.Name()), n.depth + 1})
					if int64(len(pending)+len(out)) > limits.ArchiveEntries {
						_ = dir.Close()
						return nil, errors.New("archive source entry budget exceeded")
					}
				}
				if e == io.EOF {
					break
				}
				if e != nil {
					_ = dir.Close()
					return nil, e
				}
			}
			_ = dir.Close()
		}
	}
	return out, nil
}
func archiveContext(ctx context.Context, d *data) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, time.Duration(d.settings.Work.Effective().ArchiveSeconds)*time.Second)
}
