package fbhttp

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/filebrowser/filebrowser/v2/files"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxPatchDrainBytes = 32 << 20

func drainRequestBody(r *http.Request) {
	if r.Body != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, maxPatchDrainBytes))
	}
}
func tusHeaders(w http.ResponseWriter) {
	w.Header().Set("Tus-Resumable", "1.0.0")
	w.Header().Set("Cache-Control", "no-store")
}
func uploadInfoHeaders(w http.ResponseWriter, i UploadInfo) {
	tusHeaders(w)
	if i.ID == "" {
		return
	}
	w.Header().Set("Upload-Offset", strconv.FormatInt(i.Offset, 10))
	w.Header().Set("Upload-Length", strconv.FormatInt(i.Length, 10))
	w.Header().Set("Upload-State", i.Status)
	w.Header().Set("Upload-Scan-State", i.Scan.Status)
	w.Header().Set("Upload-Expires", i.ExpiresAt.UTC().Format(http.TimeFormat))
}
func tusOptionsHandler(w http.ResponseWriter, _ *http.Request) {
	tusHeaders(w)
	w.Header().Set("Tus-Version", "1.0.0")
	w.Header().Set("Tus-Extension", "creation,expiration,termination")
	w.WriteHeader(http.StatusNoContent)
}
func uploadErrorState(err error) string {
	if errors.Is(err, files.ErrWriteConflict) || os.IsExist(err) {
		return "conflict"
	}
	return "error"
}
func uploadStatus(err error) int {
	switch {
	case err == nil:
		return 200
	case errors.Is(err, ErrUploadExpired):
		return 410
	case errors.Is(err, ErrUploadLength):
		return 413
	case errors.Is(err, ErrUploadStorage):
		return 507
	case errors.Is(err, ErrUploadState), errors.Is(err, files.ErrWriteConflict):
		return 409
	default:
		return errToStatus(err)
	}
}
func uploadID(r *http.Request) string { return strings.Split(strings.Trim(r.URL.Path, "/"), "/")[0] }
func uploadMetadata(r *http.Request) (map[string]string, error) {
	out := map[string]string{}
	raw := r.Header.Get("Upload-Metadata")
	if len(raw) > 8192 {
		return nil, fmt.Errorf("upload metadata is too large")
	}
	if raw == "" {
		return out, nil
	}
	for _, item := range strings.Split(raw, ",") {
		parts := strings.SplitN(strings.TrimSpace(item), " ", 2)
		if parts[0] == "" {
			return nil, fmt.Errorf("invalid metadata")
		}
		if _, ok := out[parts[0]]; ok {
			return nil, fmt.Errorf("duplicate metadata")
		}
		value := ""
		if len(parts) == 2 {
			b, e := base64.StdEncoding.DecodeString(parts[1])
			if e != nil {
				return nil, e
			}
			value = string(b)
		}
		out[parts[0]] = value
	}
	return out, nil
}
func tusPostHandler(m *UploadManager) handleFunc {
	return withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		tusHeaders(w)
		if version := r.Header.Get("Tus-Resumable"); version != "" && version != "1.0.0" {
			w.Header().Set("Tus-Version", "1.0.0")
			return 412, fmt.Errorf("unsupported tus version")
		}
		length, e := getUploadLength(r)
		if e != nil || length < 0 {
			return 400, fmt.Errorf("invalid Upload-Length")
		}
		meta, e := uploadMetadata(r)
		if e != nil {
			return 400, e
		}
		if value := meta["sha256"]; value != "" {
			b, err := hex.DecodeString(value)
			if err != nil || len(b) != 32 {
				return 400, fmt.Errorf("invalid expected SHA-256")
			}
		}
		if !d.user.Perm.Create || !d.Check(r.URL.Path) {
			return 403, nil
		}
		if _, e = files.Version(d.user.Fs, r.URL.Path); e != nil {
			return uploadStatus(e), e
		}
		i, e := m.Create(d, r.URL.Path, length, r.URL.Query().Get("override") == "true", meta["modified"], meta["sha256"])
		if e != nil {
			return uploadStatus(e), e
		}
		prefix := "/" + strings.Trim(d.server.BaseURL, "/")
		if prefix == "/" {
			prefix = ""
		}
		w.Header().Set("Location", prefix+"/api/uploads/"+i.ID)
		uploadInfoHeaders(w, i)
		return 201, nil
	})
}
func tusHeadHandler(m *UploadManager) handleFunc {
	return withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		tusHeaders(w)
		i, e := m.Info(uploadID(r), d)
		if e != nil {
			return uploadStatus(e), e
		}
		uploadInfoHeaders(w, i)
		if i.Status == "cancelled" || i.Status == "expired" || time.Now().After(i.ExpiresAt) {
			return 410, nil
		}
		return 200, nil
	})
}
func tusPatchHandler(m *UploadManager) handleFunc {
	return withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		tusHeaders(w)
		if r.Header.Get("Content-Type") != "application/offset+octet-stream" {
			drainRequestBody(r)
			return 415, nil
		}
		offset, e := getUploadOffset(r)
		if e != nil || offset < 0 {
			drainRequestBody(r)
			return 400, fmt.Errorf("invalid Upload-Offset")
		}
		i, e := m.Patch(uploadID(r), d, offset, r.Body, r.ContentLength)
		uploadInfoHeaders(w, i)
		if e != nil {
			drainRequestBody(r)
			return uploadStatus(e), e
		}
		return 204, nil
	})
}
func tusDeleteHandler(m *UploadManager) handleFunc {
	return withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		tusHeaders(w)
		var e error
		if r.URL.Query().Get("dismiss") == "true" {
			e = m.Dismiss(uploadID(r), d)
		} else {
			e = m.Cancel(uploadID(r), d)
		}
		if e != nil {
			return uploadStatus(e), e
		}
		return 204, nil
	})
}
func uploadStatusHandler(m *UploadManager) handleFunc {
	return withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		i, e := m.Info(uploadID(r), d)
		if e != nil {
			return uploadStatus(e), e
		}
		w.Header().Set("Cache-Control", "no-store")
		return renderJSON(w, r, i)
	})
}
func uploadListHandler(m *UploadManager) handleFunc {
	return withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		list := m.List(d)
		sort.Slice(list, func(i, j int) bool { return list[i].UpdatedAt.After(list[j].UpdatedAt) })
		// A long-lived unfinished upload must not disappear behind newer
		// completed history after a browser restart.
		visible := make([]UploadInfo, 0, len(list))
		completed := 0
		for _, info := range list {
			if sessionHasStaging(info.Status) {
				visible = append(visible, info)
			} else if completed < 200 {
				visible = append(visible, info)
				completed++
			}
		}
		list = visible
		w.Header().Set("Cache-Control", "no-store")
		return renderJSON(w, r, list)
	})
}
func uploadControlHandler(m *UploadManager, action string) handleFunc {
	return withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		var i UploadInfo
		var e error
		if action == "retry" {
			i, e = m.Retry(uploadID(r), d)
		} else {
			i, e = m.Pause(uploadID(r), d, action == "resume")
		}
		if e != nil {
			return uploadStatus(e), e
		}
		return renderJSON(w, r, i)
	})
}

func getUploadLength(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.Header.Get("Upload-Length"), 10, 64)
}
func getUploadOffset(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
}
