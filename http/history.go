package fbhttp

import (
	"github.com/filebrowser/filebrowser/v2/files"
	"net/http"
)

var historyHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	path := cleanFileBrowserPath(r.URL.Path)
	if !d.user.Perm.Download || !d.Check(path) {
		return http.StatusForbidden, nil
	}
	versions, e := files.ListVersions(d.user.Fs, path)
	if e != nil {
		return errToStatus(e), e
	}
	if r.Method == http.MethodGet {
		return renderJSON(w, r, versions)
	}
	if !d.user.Perm.Modify {
		return http.StatusForbidden, nil
	}
	for _, v := range versions {
		if v.ID != r.URL.Query().Get("id") {
			continue
		}
		src, e := files.InternalFs(d.user.Fs).Open(v.Path)
		if e != nil {
			return errToStatus(e), e
		}
		defer src.Close()
		e = runPublicationHook(d, "save", path, func() error {
			_, e := writeFile(d.user.Fs, path, src, d.settings.FileMode, d.settings.DirMode)
			return e
		})
		return errToStatus(e), e
	}
	return http.StatusNotFound, nil
})
