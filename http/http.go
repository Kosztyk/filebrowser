package fbhttp

import (
	"io/fs"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/spf13/afero"

	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/storage"
)

type modifyRequest struct {
	What            string   `json:"what"`             // Answer to: what data type?
	Which           []string `json:"which"`            // Answer to: which fields?
	CurrentPassword string   `json:"current_password"` // Answer to: user logged password
}

func NewHandler(
	imgSvc ImgService,
	fileCache FileCache,
	uploadCache *UploadManager,
	store *storage.Storage,
	server *settings.Server,
	assetsFs fs.FS,
) (http.Handler, error) {
	server.Clean()
	server.CaseInsensitiveFs = files.CaseInsensitive(afero.NewOsFs(), server.Root)

	var features *featureRuntime
	if uploadCache != nil {
		var err error
		features, err = newFeatureRuntime(uploadCache)
		if err != nil {
			return nil, err
		}
	}
	r := mux.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			set, err := store.Settings.Get()
			if err == nil {
				w.Header().Set("Content-Security-Policy", contentSecurityPolicy(effectiveCollabora(server, set)))
			} else {
				w.Header().Set("Content-Security-Policy", contentSecurityPolicy(effectiveCollabora(server, nil)))
			}
			next.ServeHTTP(w, r)
		})
	})
	index, static := getStaticHandlers(store, server, assetsFs)

	monkey := func(fn handleFunc, prefix string) http.Handler {
		return handle(fn, prefix, store, server, features)
	}

	r.HandleFunc("/health", healthHandler)
	r.PathPrefix("/static").Handler(static)
	r.NotFoundHandler = index

	api := r.PathPrefix("/api").Subrouter()

	tokenExpirationTime := server.GetTokenExpirationTime(DefaultTokenExpirationTime)
	api.Handle("/login", monkey(loginHandler(tokenExpirationTime), ""))
	api.Handle("/signup", monkey(signupHandler, ""))
	api.Handle("/renew", monkey(renewHandler(tokenExpirationTime), ""))

	api.Handle("/jobs", monkey(jobsListHandler, "")).Methods("GET")
	api.Handle("/jobs/{id}", monkey(jobControlHandler, "")).Methods("GET", "POST")
	api.Handle("/sessions", monkey(sessionsHandler, "")).Methods("GET", "DELETE")
	api.Handle("/system", monkey(systemHandler, "")).Methods("GET")
	api.Handle("/diagnostics/{integration}", monkey(diagnosticsHandler, "")).Methods("GET", "POST")
	api.PathPrefix("/history").Handler(monkey(historyHandler, "/api/history")).Methods("GET", "POST")
	api.PathPrefix("/text-preview").Handler(monkey(textPreviewHandler, "/api/text-preview")).Methods("GET")
	api.Handle("/downloads", monkey(downloadPrepareHandler, "")).Methods("POST")
	api.Handle("/downloads/{id}", monkey(downloadArtifactHandler, "")).Methods("GET", "HEAD")
	users := api.PathPrefix("/users").Subrouter()
	users.Handle("", monkey(usersGetHandler, "")).Methods("GET")
	users.Handle("", monkey(userPostHandler, "")).Methods("POST")
	users.Handle("/{id:[0-9]+}", monkey(userPutHandler, "")).Methods("PUT")
	users.Handle("/{id:[0-9]+}", monkey(userGetHandler, "")).Methods("GET")
	users.Handle("/{id:[0-9]+}", monkey(userDeleteHandler, "")).Methods("DELETE")

	api.PathPrefix("/archive/resources").Handler(monkey(archiveResourceHandler, "/api/archive/resources")).Methods("GET")
	api.PathPrefix("/archive/raw").Handler(monkey(archiveRawHandler, "/api/archive/raw")).Methods("GET")
	api.PathPrefix("/archive/extract").Handler(monkey(archiveExtractHandler, "/api/archive/extract")).Methods("POST")
	api.Handle("/archive/create", monkey(archiveCreateHandler, "")).Methods("POST")
	api.PathPrefix("/archive/jobs").Handler(monkey(archiveCreateJobStatusHandler, "/api/archive/jobs")).Methods("GET")
	api.PathPrefix("/clamav/jobs").Handler(monkey(clamAVScanJobStatusHandler, "/api/clamav/jobs")).Methods("GET")
	api.PathPrefix("/clamav/scan").Handler(monkey(clamAVPathScanHandler, "/api/clamav/scan")).Methods("POST")
	api.Handle("/convertx/targets", monkey(convertXTargetsHandler, "")).Methods("GET")
	api.Handle("/convertx/convert", monkey(convertXConvertHandler, "")).Methods("POST")
	api.PathPrefix("/convertx/jobs").Handler(monkey(convertXConversionJobStatusHandler, "/api/convertx/jobs")).Methods("GET")

	api.PathPrefix("/resources/recursive").Handler(monkey(resourceGetRecursiveHandler, "/api/resources/recursive")).Methods("GET")
	api.PathPrefix("/resources").Handler(monkey(resourceGetHandler, "/api/resources")).Methods("GET")
	api.PathPrefix("/resources").Handler(monkey(resourceDeleteHandler(fileCache), "/api/resources")).Methods("DELETE")
	api.PathPrefix("/resources").Handler(monkey(resourcePostHandler(fileCache), "/api/resources")).Methods("POST")
	api.PathPrefix("/resources").Handler(monkey(resourcePutHandler, "/api/resources")).Methods("PUT")
	api.PathPrefix("/resources").Handler(monkey(resourcePatchHandler(fileCache), "/api/resources")).Methods("PATCH")

	api.PathPrefix("/tus").Handler(monkey(tusPostHandler(uploadCache), "/api/tus")).Methods("POST")
	api.PathPrefix("/tus").HandlerFunc(tusOptionsHandler).Methods("OPTIONS")
	api.Handle("/uploads", monkey(uploadListHandler(uploadCache), "")).Methods("GET")
	api.Handle("/uploads/", monkey(uploadListHandler(uploadCache), "")).Methods("GET")
	api.Handle("/uploads/{id}", monkey(tusHeadHandler(uploadCache), "/api/uploads")).Methods("HEAD")
	api.Handle("/uploads/{id}", monkey(tusPatchHandler(uploadCache), "/api/uploads")).Methods("PATCH")
	api.Handle("/uploads/{id}", monkey(tusDeleteHandler(uploadCache), "/api/uploads")).Methods("DELETE")
	api.Handle("/uploads/{id}", monkey(uploadStatusHandler(uploadCache), "/api/uploads")).Methods("GET")
	api.PathPrefix("/uploads").HandlerFunc(tusOptionsHandler).Methods("OPTIONS")
	for _, action := range []string{"pause", "resume", "retry"} {
		api.Handle("/uploads/{id}/"+action, monkey(uploadControlHandler(uploadCache, action), "/api/uploads")).Methods("POST")
	}
	api.PathPrefix("/tus").Handler(monkey(tusDeleteHandler(uploadCache), "/api/tus")).Methods("DELETE")

	api.PathPrefix("/usage").Handler(monkey(diskUsage, "/api/usage")).Methods("GET")

	api.Handle("/shares", monkey(shareListHandler, "")).Methods("GET")
	api.Handle("/share-config", monkey(erugoShareConfigHandler, "")).Methods("GET")
	api.Handle("/erugo/reverse-share", monkey(erugoReverseShareHandler, "")).Methods("POST")
	api.PathPrefix("/share").Handler(monkey(shareGetsHandler, "/api/share")).Methods("GET")
	api.PathPrefix("/share").Handler(monkey(sharePostHandler, "/api/share")).Methods("POST")
	api.PathPrefix("/share").Handler(monkey(shareDeleteHandler, "/api/share")).Methods("DELETE")

	api.Handle("/settings", monkey(settingsGetHandler, "")).Methods("GET")
	api.Handle("/settings", monkey(settingsPutHandler, "")).Methods("PUT")
	api.Handle("/settings/clamav/test", monkey(settingsClamAVTestHandler, "")).Methods("POST")
	api.Handle("/settings/convertx/test", monkey(settingsConvertXTestHandler, "")).Methods("POST")
	api.Handle("/settings/erugo/test", monkey(settingsErugoTestHandler, "")).Methods("POST")

	api.PathPrefix("/raw").Handler(monkey(rawHandler, "/api/raw")).Methods("GET")
	api.PathPrefix("/preview/{size}/{path:.*}").
		Handler(monkey(previewHandler(imgSvc, fileCache, server.EnableThumbnails, server.ResizePreview), "/api/preview")).Methods("GET")
	api.PathPrefix("/command").Handler(monkey(commandsHandler, "/api/command")).Methods("GET")
	api.PathPrefix("/search").Handler(monkey(searchHandler, "/api/search")).Methods("GET")
	api.PathPrefix("/subtitle").Handler(monkey(subtitleHandler, "/api/subtitle")).Methods("GET")

	api.Handle("/collabora/open", monkey(collaboraOpenHandler, "")).Methods("GET")
	api.Handle("/collabora/test", monkey(collaboraTestHandler, "")).Methods("POST")
	r.Handle("/wopi/files/{id}", monkey(wopiFileHandler, "")).Methods("GET", "POST")
	r.Handle("/wopi/files/{id}/contents", monkey(wopiFileContentsHandler, "")).Methods("GET", "POST")

	public := api.PathPrefix("/public").Subrouter()
	public.PathPrefix("/dl").Handler(monkey(publicDlHandler, "/api/public/dl/")).Methods("GET")
	public.PathPrefix("/share").Handler(monkey(publicShareHandler, "/api/public/share/")).Methods("GET")

	return stripPrefix(server.BaseURL, r), nil
}
