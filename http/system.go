package fbhttp

import (
	"context"
	"encoding/json"
	"github.com/filebrowser/filebrowser/v2/version"
	"github.com/gorilla/mux"
	bolt "go.etcd.io/bbolt"
	"io"
	"net/http"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

var systemHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	info := map[string]any{"version": version.Version, "build": version.CommitSHA, "sourceChecksum": version.SourceSHA, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "features": []string{"persistent jobs", "resumable uploads", "prepared downloads", "office locks", "previous versions", "paged listings", "revocable sessions"}}
	if d.user.Perm.Admin {
		info["limits"] = d.settings.Work.Effective()
		info["integrations"] = map[string]bool{"convertx": d.settings.ConvertX.Enabled, "erugo": d.settings.Erugo.Enabled, "clamav": d.settings.ClamAV.Enabled, "collabora": d.collaboraConfig().Enabled}
		if b, ok := debug.ReadBuildInfo(); ok {
			deps := map[string]string{}
			for _, dep := range b.Deps {
				deps[dep.Path] = dep.Version
			}
			info["dependencies"] = deps
		}
	}
	return renderJSON(w, r, info)
})

type integrationDiagnostic struct {
	Integration  string         `json:"integration"`
	Checked      time.Time      `json:"checked"`
	Status       string         `json:"status"`
	Message      string         `json:"message"`
	Version      string         `json:"version"`
	Capabilities map[string]any `json:"capabilities"`
	Config       string         `json:"-"`
}
type storedDiagnostic struct {
	integrationDiagnostic
	Config string
}

func redactDiagnostic(s string, d *data) string {
	for _, secret := range []string{d.settings.ConvertX.APIKey, d.settings.Erugo.APIKey, d.collaboraConfig().WOPISecret} {
		if secret != "" {
			s = strings.ReplaceAll(s, secret, "[redacted]")
		}
	}
	return safeJobMessage(s)
}

var diagnosticsHandler = withAdmin(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	if d.features == nil {
		return http.StatusServiceUnavailable, nil
	}
	name := mux.Vars(r)["integration"]
	if name != "convertx" && name != "erugo" && name != "collabora" && name != "clamav" {
		return http.StatusNotFound, nil
	}
	cfg, _ := json.Marshal([]any{d.settings.ConvertX, d.settings.Erugo, d.settings.ClamAV, d.collaboraConfig()})
	fingerprint := credentialHash(string(cfg))
	diag := integrationDiagnostic{Integration: name, Checked: time.Now(), Status: "ok", Version: "not advertised", Config: fingerprint, Capabilities: map[string]any{}}
	if r.Method == http.MethodGet {
		found := false
		e := d.features.m.journal.View(func(tx *bolt.Tx) error {
			v := tx.Bucket([]byte("diagnostics-v1")).Get([]byte(name))
			if v == nil {
				return nil
			}
			var saved storedDiagnostic
			if e := json.Unmarshal(v, &saved); e != nil {
				return e
			}
			diag = saved.integrationDiagnostic
			if saved.Config != fingerprint {
				diag.Status = "stale"
				diag.Message = "Configuration changed; run the connection test again"
			}
			found = true
			return nil
		})
		if e != nil {
			return http.StatusInternalServerError, e
		}
		if !found {
			diag.Status = "unchecked"
			diag.Message = "No diagnostic has been run"
			diag.Checked = time.Time{}
		}
		return renderJSON(w, r, diag)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var e error
	switch name {
	case "convertx":
		e = testConvertXConnection(ctx, d.settings.ConvertX)
		diag.Capabilities = map[string]any{"contract": "File Browser conversion API", "targets": "/api/conversions", "jobs": "/api/jobs/{id}", "upstreamResume": "poll persisted job ID", "duplicateSubmission": "ambiguous outcomes require review", "schemaChecked": "health JSON and conversion targets", "byteResume": "not advertised by the integration API"}
		if e == nil {
			_, e = fetchConvertXTargets(ctx, d.settings.ConvertX, "txt")
		}
		if e == nil {
			base, _ := normalizeConvertXURL(d.settings.ConvertX.URL)
			req, _ := http.NewRequestWithContext(ctx, "GET", base+"/api/health", nil)
			addConvertXAuth(req, d.settings.ConvertX.APIKey)
			res, err := convertClient(d.settings.ConvertX, 30*time.Second).Do(req)
			if err == nil {
				defer res.Body.Close()
				var payload map[string]any
				if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&payload) == nil {
					if v, ok := payload["version"].(string); ok {
						diag.Version = v
					}
				}
			}
		}
	case "erugo":
		e = testErugoConnection(ctx, d.settings.Erugo)
		diag.Capabilities = map[string]any{"contract": "configured Erugo File Browser endpoints", "streaming": true, "folderPackaging": "persistent local artifact", "byteResume": false, "automaticRetryAfterSubmission": false}
	case "clamav":
		e = testClamAVConnection(ctx, d.settings.ClamAV)
		diag.Capabilities = map[string]any{"contract": "HTTP multipart scanner", "automaticScanMaxBytes": d.settings.ClamAV.AutomaticMaxBytes, "strictCleanResponse": true}
	case "collabora":
		var discovery *collaboraDiscovery
		discovery, e = fetchCollaboraDiscovery(ctx, d.collaboraConfig().URL)
		extensions := []string{}
		if e == nil {
			for _, app := range discovery.Apps {
				for _, action := range app.Actions {
					extensions = append(extensions, action.Ext)
				}
			}
		}
		diag.Capabilities = map[string]any{"contract": "WOPI discovery XML", "extensions": extensions, "persistentLocks": true, "previousContents": true}
	}
	if e != nil {
		diag.Status = "error"
		diag.Message = redactDiagnostic(e.Error(), d)
	} else {
		diag.Message = "Connection and advertised API contract checked. End-to-end operation should be tested with a disposable file."
	}
	e = d.features.m.journal.Update(func(tx *bolt.Tx) error {
		v, e := json.Marshal(storedDiagnostic{diag, fingerprint})
		if e != nil {
			return e
		}
		return tx.Bucket([]byte("diagnostics-v1")).Put([]byte(name), v)
	})
	if e != nil {
		return http.StatusInternalServerError, e
	}
	return renderJSON(w, r, diag)
})
