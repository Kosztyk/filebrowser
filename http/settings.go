package fbhttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/filebrowser/filebrowser/v2/rules"
	"github.com/filebrowser/filebrowser/v2/settings"
)

type settingsData struct {
	Signup                bool                  `json:"signup"`
	HideLoginButton       bool                  `json:"hideLoginButton"`
	CreateUserDir         bool                  `json:"createUserDir"`
	MinimumPasswordLength uint                  `json:"minimumPasswordLength"`
	UserHomeBasePath      string                `json:"userHomeBasePath"`
	Defaults              settings.UserDefaults `json:"defaults"`
	AuthMethod            settings.AuthMethod   `json:"authMethod"`
	Rules                 []rules.Rule          `json:"rules"`
	Branding              settings.Branding     `json:"branding"`
	Work                  settings.Work         `json:"work"`
	Tus                   settings.Tus          `json:"tus"`
	Collabora             settings.Collabora    `json:"collabora"`
	ConvertX              settings.ConvertX     `json:"convertx"`
	Erugo                 settings.Erugo        `json:"erugo"`
	ClamAV                settings.ClamAV       `json:"clamav"`
	Shell                 []string              `json:"shell"`
	Commands              map[string][]string   `json:"commands"`
}

var settingsGetHandler = withAdmin(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	data := &settingsData{
		Signup:                d.settings.Signup,
		HideLoginButton:       d.settings.HideLoginButton,
		CreateUserDir:         d.settings.CreateUserDir,
		MinimumPasswordLength: d.settings.MinimumPasswordLength,
		UserHomeBasePath:      d.settings.UserHomeBasePath,
		Defaults:              d.settings.Defaults,
		AuthMethod:            d.settings.AuthMethod,
		Rules:                 d.settings.Rules,
		Branding:              d.settings.Branding,
		Work:                  d.settings.Work.Effective(),
		Tus:                   d.settings.Tus,
		Collabora:             d.collaboraConfig(),
		ConvertX:              d.settings.ConvertX,
		Erugo:                 d.settings.Erugo,
		ClamAV:                d.settings.ClamAV,
		Shell:                 d.settings.Shell,
		Commands:              d.settings.Commands,
	}

	return renderJSON(w, r, data)
})

var settingsPutHandler = withAdmin(func(_ http.ResponseWriter, r *http.Request, d *data) (int, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, 2<<20)
	req := &settingsData{}
	err := json.NewDecoder(r.Body).Decode(req)
	if err != nil {
		return http.StatusBadRequest, err
	}

	if e := req.Work.Validate(); e != nil {
		return http.StatusBadRequest, e
	}
	for _, value := range []string{req.ConvertX.ConnectTimeout, req.ConvertX.UploadTimeout, req.ConvertX.ProcessTimeout, req.ConvertX.DownloadTimeout} {
		if value != "" {
			duration, e := time.ParseDuration(value)
			if e != nil || duration <= 0 || duration > 24*time.Hour {
				return http.StatusBadRequest, fmt.Errorf("integration timeouts must be positive durations no greater than 24h")
			}
		}
	}
	settings.NormalizeTransfers(&req.Tus, &req.ClamAV)
	if err := settings.ValidateTransfers(req.Tus, req.ClamAV); err != nil {
		return http.StatusBadRequest, err
	}
	d.settings.Signup = req.Signup
	d.settings.CreateUserDir = req.CreateUserDir
	d.settings.MinimumPasswordLength = req.MinimumPasswordLength
	d.settings.UserHomeBasePath = req.UserHomeBasePath
	d.settings.Defaults = req.Defaults
	d.settings.Rules = req.Rules
	d.settings.Branding = req.Branding
	d.settings.Tus = req.Tus
	d.settings.Work = req.Work.Effective()
	req.Collabora.Configured = true
	d.settings.Collabora = req.Collabora
	req.ConvertX.Configured = true
	d.settings.ConvertX = req.ConvertX
	req.Erugo.Configured = true
	d.settings.Erugo = req.Erugo
	d.settings.ClamAV = req.ClamAV
	d.settings.Shell = req.Shell
	d.settings.Commands = req.Commands
	d.settings.HideLoginButton = req.HideLoginButton

	err = d.store.Settings.Save(d.settings)
	return errToStatus(err), err
})

var settingsClamAVTestHandler = withAdmin(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	cfg := d.settings.ClamAV

	if r.Body != nil {
		defer r.Body.Close()
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
			return http.StatusBadRequest, err
		}
	}

	if err := testClamAVConnection(r.Context(), cfg); err != nil {
		return clamAVHTTPStatus(err), err
	}

	return renderJSON(w, r, map[string]string{
		"status":  "OK",
		"message": "ClamAV connection successful",
	})
})

var settingsConvertXTestHandler = withAdmin(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	cfg := d.settings.ConvertX

	if r.Body != nil {
		defer r.Body.Close()
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
			return http.StatusBadRequest, err
		}
	}

	if err := testConvertXConnection(r.Context(), cfg); err != nil {
		return http.StatusBadGateway, err
	}

	return renderJSON(w, r, map[string]string{
		"status":  "OK",
		"message": "ConvertX connection successful",
	})
})
