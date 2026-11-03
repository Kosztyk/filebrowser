package fbhttp

import (
	"context"
	"encoding/json"
	"github.com/filebrowser/filebrowser/v2/share"
	"os"
	"time"
)

type erugoWork struct {
	Path string
	Body share.CreateBody
}
type erugoContextKey struct{}
type erugoJobContext struct {
	f        *featureRuntime
	j        *Job
	prepared string
	bytes    int64
	last     time.Time
}

func (f *featureRuntime) erugo(ctx context.Context, j *Job, d *data) (any, error) {
	if !d.user.Perm.Share || !d.user.Perm.Download || !d.settings.Erugo.Enabled {
		return nil, os.ErrPermission
	}
	var p erugoWork
	if e := json.Unmarshal(j.Payload, &p); e != nil {
		return nil, e
	}
	if !d.Check(p.Path) {
		return nil, os.ErrPermission
	}
	info, e := d.user.Fs.Stat(p.Path)
	if e != nil {
		return nil, e
	}
	state := &erugoJobContext{f: f, j: j}
	if info.IsDir() {
		if _, e = f.buildArtifact(ctx, j, d, downloadWork{Items: []string{p.Path}, Name: info.Name() + ".zip"}); e != nil {
			return nil, e
		}
		state.prepared = f.artifactPath(j.ID)
	}
	ctx = context.WithValue(ctx, erugoContextKey{}, state)
	return createErugoShare(ctx, d, p.Path, p.Body)
}

type erugoProgressReader struct {
	ctx   context.Context
	r     interface{ Read([]byte) (int, error) }
	state *erugoJobContext
	total int64
}

func (r erugoProgressReader) Read(p []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	n, e := r.r.Read(p)
	r.state.bytes += int64(n)
	if time.Since(r.state.last) > time.Second {
		r.state.f.progress(r.state.j.ID, r.state.bytes, r.total)
		r.state.last = time.Now()
	}
	return n, e
}
