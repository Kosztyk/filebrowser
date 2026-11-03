package fbhttp

import (
	"errors"
	"fmt"
	"github.com/filebrowser/filebrowser/v2/files"
	"io"
	"net/http"
	"net/url"
	gopath "path"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
)

type wopiFileContext struct {
	Claims wopiTokenClaims
	Info   fileInfoStat
}

type fileInfoStat interface {
	Name() string
	Size() int64
	ModTime() time.Time
	IsDir() bool
}

type wopiCheckFileInfo struct {
	BaseFileName         string `json:"BaseFileName"`
	OwnerID              string `json:"OwnerId"`
	Size                 int64  `json:"Size"`
	UserID               string `json:"UserId"`
	UserFriendlyName     string `json:"UserFriendlyName"`
	UserCanWrite         bool   `json:"UserCanWrite"`
	IsAdminUser          bool   `json:"IsAdminUser"`
	ReadOnly             bool   `json:"ReadOnly"`
	SupportsLocks        bool   `json:"SupportsLocks"`
	SupportsGetLock      bool   `json:"SupportsGetLock"`
	SupportsUpdate       bool   `json:"SupportsUpdate"`
	LastModifiedTime     string `json:"LastModifiedTime"`
	Version              string `json:"Version"`
	BreadcrumbBrandName  string `json:"BreadcrumbBrandName"`
	BreadcrumbBrandURL   string `json:"BreadcrumbBrandUrl,omitempty"`
	BreadcrumbFolderName string `json:"BreadcrumbFolderName,omitempty"`
}

var wopiFileHandler = func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	ctx, status, err := loadWOPIFile(r, d)
	if status != 0 || err != nil {
		return status, err
	}

	if r.Method == http.MethodGet {
		return wopiCheckFileInfoHandler(w, r, d, ctx)
	}

	if !ctx.Claims.CanWrite || !d.user.Perm.Modify {
		return http.StatusForbidden, nil
	}
	if d.features == nil {
		return http.StatusServiceUnavailable, nil
	}
	return d.features.officeLock(w, r, d, ctx)

}

var wopiFileContentsHandler = func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	ctx, status, err := loadWOPIFile(r, d)
	if status != 0 || err != nil {
		return status, err
	}

	if r.Method == http.MethodGet {
		return wopiGetFile(w, r, d, ctx)
	}

	if strings.ToUpper(r.Header.Get("X-WOPI-Override")) != "PUT" {
		return http.StatusNotImplemented, nil
	}
	return wopiPutFile(w, r, d, ctx)
}

func loadWOPIFile(r *http.Request, d *data) (*wopiFileContext, int, error) {
	cfg := d.collaboraConfig()
	if !cfg.Enabled {
		return nil, http.StatusNotFound, errors.New("collabora integration is disabled")
	}

	tokenString := r.URL.Query().Get("access_token")
	if tokenString == "" {
		return nil, http.StatusUnauthorized, errors.New("missing WOPI access_token")
	}

	claims := wopiTokenClaims{}
	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	token, err := parser.ParseWithClaims(tokenString, &claims, func(_ *jwt.Token) (interface{}, error) {
		return wopiSigningKey(d), nil
	})
	if err != nil || !token.Valid {
		return nil, http.StatusUnauthorized, err
	}

	if claims.UserID == 0 || claims.Path == "" || claims.FileID == "" {
		return nil, http.StatusUnauthorized, errors.New("invalid WOPI token claims")
	}

	fileID := mux.Vars(r)["id"]
	if fileID == "" || fileID != claims.FileID || fileID != wopiFileID(claims.UserID, claims.Path) {
		return nil, http.StatusUnauthorized, errors.New("WOPI token does not match requested file")
	}

	user, err := d.store.Users.Get(d.server.Root, d.server.FollowExternalSymlinks, claims.UserID)
	if err != nil {
		return nil, errToStatus(err), err
	}
	d.user = user

	claims.Path = gopath.Clean("/" + strings.TrimPrefix(claims.Path, "/"))
	if claims.Path == "/" || !d.Check(claims.Path) || !d.user.Perm.Download {
		return nil, http.StatusNotFound, nil
	}

	info, err := d.user.Fs.Stat(claims.Path)
	if err != nil {
		return nil, errToStatus(err), err
	}
	if info.IsDir() {
		return nil, http.StatusNotFound, nil
	}

	return &wopiFileContext{Claims: claims, Info: info}, 0, nil
}

func wopiCheckFileInfoHandler(w http.ResponseWriter, r *http.Request, d *data, ctx *wopiFileContext) (int, error) {
	canWrite := ctx.Claims.CanWrite && d.user.Perm.Modify
	folder := gopath.Base(gopath.Dir(ctx.Claims.Path))
	if folder == "." || folder == "/" {
		folder = "Files"
	}

	info := wopiCheckFileInfo{
		BaseFileName:         ctx.Info.Name(),
		OwnerID:              strconv.FormatUint(uint64(ctx.Claims.UserID), 10),
		Size:                 ctx.Info.Size(),
		UserID:               strconv.FormatUint(uint64(ctx.Claims.UserID), 10),
		UserFriendlyName:     d.user.Username,
		UserCanWrite:         canWrite,
		IsAdminUser:          d.user.Perm.Admin,
		ReadOnly:             !canWrite,
		SupportsLocks:        canWrite,
		SupportsGetLock:      true,
		SupportsUpdate:       canWrite,
		LastModifiedTime:     ctx.Info.ModTime().UTC().Format(time.RFC3339),
		Version:              wopiVersion(ctx.Info),
		BreadcrumbBrandName:  "File Browser",
		BreadcrumbBrandURL:   strings.TrimRight(selectCollaboraWOPIBaseURL(d.collaboraConfig(), r, d), "/"),
		BreadcrumbFolderName: folder,
	}
	return renderJSON(w, r, info)
}

func wopiGetFile(w http.ResponseWriter, _ *http.Request, d *data, ctx *wopiFileContext) (int, error) {
	fd, err := d.user.Fs.Open(ctx.Claims.Path)
	if err != nil {
		return errToStatus(err), err
	}
	defer fd.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename*=utf-8''"+url.PathEscape(ctx.Info.Name()))
	w.Header().Set("X-WOPI-ItemVersion", wopiVersion(ctx.Info))
	_, err = io.Copy(w, fd)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	return 0, nil
}

func wopiPutFile(w http.ResponseWriter, r *http.Request, d *data, ctx *wopiFileContext) (int, error) {
	if !ctx.Claims.CanWrite || !d.user.Perm.Modify {
		return http.StatusForbidden, nil
	}

	if d.features == nil {
		return http.StatusServiceUnavailable, nil
	}
	d.features.office.Lock()
	defer d.features.office.Unlock()
	requestedLock := r.Header.Get("X-WOPI-Lock")
	current, locked := files.GetLease(d.user.Fs, ctx.Claims.Path)
	if locked {
		if requestedLock == "" || requestedLock != current.Value {
			return wopiLockMismatch(w, current.Value, "lock mismatch")
		}
		version, e := files.Version(d.user.Fs, ctx.Claims.Path)
		if e != nil || version != current.Before {
			return wopiLockMismatch(w, current.Value, "file changed outside the office session")
		}
	} else if ctx.Info.Size() != 0 {
		return wopiLockMismatch(w, "", "file is not locked")
	}
	d.user.Fs = files.WithLease(d.user.Fs, requestedLock)
	if expected := r.Header.Get("X-Filebrowser-Version"); expected != "" && expected != wopiVersion(ctx.Info) {
		return wopiLockMismatch(w, requestedLock, "version mismatch")
	}

	err := runPublicationHook(d, "save", ctx.Claims.Path, func() error {
		return d.features.saveOffice(r, d, ctx.Claims.Path)
	})
	if err != nil {
		return errToStatus(err), err
	}

	info, err := d.user.Fs.Stat(ctx.Claims.Path)
	if err == nil {
		w.Header().Set("X-WOPI-ItemVersion", wopiVersion(info))
		if locked && d.features != nil {
			current.Before, _ = files.Version(d.user.Fs, ctx.Claims.Path)
			if err = d.features.persistOfficeLock(d, ctx.Claims.Path, current); err != nil {
				return http.StatusInternalServerError, err
			}
		}
	}
	return 0, nil
}

func wopiLockTTL(r *http.Request) (time.Duration, int, error) {
	raw := strings.TrimSpace(r.Header.Get("X-WOPI-LockExpirationTimeout"))
	if raw == "" {
		return 30 * time.Minute, 0, nil
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 60 || seconds > 3600 {
		return 0, http.StatusBadRequest, errors.New("invalid X-WOPI-LockExpirationTimeout")
	}
	return time.Duration(seconds) * time.Second, 0, nil
}

func wopiLockMismatch(w http.ResponseWriter, currentLock, reason string) (int, error) {
	w.Header().Set("X-WOPI-Lock", currentLock)
	if reason != "" {
		w.Header().Set("X-WOPI-LockFailureReason", reason)
	}
	return http.StatusConflict, nil
}

func wopiVersion(info fileInfoStat) string {
	return fmt.Sprintf("%x%x", info.ModTime().UnixNano(), info.Size())
}
