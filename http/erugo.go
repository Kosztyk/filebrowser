package fbhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	gopath "path"
	"strconv"
	"strings"
	"time"

	"github.com/mholt/archives"

	fberrors "github.com/filebrowser/filebrowser/v2/errors"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/share"
)

const defaultErugoTimeout = 2 * time.Minute
const defaultErugoShareEndpoint = "/api/integrations/convertx/share"

type erugoTestResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type erugoShareConfigResponse struct {
	Backend string `json:"backend"`
	Enabled bool   `json:"enabled"`
}

type erugoShareResponse struct {
	Status    string          `json:"status"`
	Message   string          `json:"message"`
	ShareURL  string          `json:"share_url"`
	ShareLink string          `json:"share_link"`
	EmailSent bool            `json:"email_sent"`
	Data      json.RawMessage `json:"data"`
}

func normalizeErugoURL(raw string) (string, error) {
	cleaned := strings.TrimRight(strings.TrimSpace(raw), "/")
	if cleaned == "" {
		return "", errors.New("no Erugo URL is configured")
	}

	parsed, err := url.Parse(cleaned)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("Erugo URL must be a valid absolute URL, for example http://erugo.lan:9998")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("Erugo URL must use http or https")
	}

	return cleaned, nil
}

func erugoTimeout(raw string) time.Duration {
	if strings.TrimSpace(raw) == "" {
		return defaultErugoTimeout
	}
	duration, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || duration <= 0 {
		return defaultErugoTimeout
	}
	return duration
}

func erugoShareEndpoint(cfg settings.Erugo) string {
	endpoint := strings.TrimSpace(cfg.ShareEndpoint)
	if endpoint == "" {
		endpoint = defaultErugoShareEndpoint
	}
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}
	return endpoint
}

func addErugoAuth(req *http.Request, apiKey string) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("X-Erugo-Api-Token", apiKey)
}

func testErugoConnection(ctx context.Context, cfg settings.Erugo) error {
	base, err := normalizeErugoURL(cfg.URL)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: erugoTimeout(cfg.Timeout)}

	healthReq, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/health", nil)
	if err != nil {
		return err
	}
	healthRes, err := client.Do(healthReq)
	if err != nil {
		return fmt.Errorf("failed to reach Erugo: %w", err)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(healthRes.Body, 2048))
	healthRes.Body.Close()
	if healthRes.StatusCode < 200 || healthRes.StatusCode >= 400 {
		return fmt.Errorf("Erugo health check returned HTTP %d", healthRes.StatusCode)
	}

	// Validate that the integration endpoint exists and that the token is accepted.
	// A token-authenticated request without a file should return validation error
	// 400/422. Missing/wrong tokens return 401/403 and missing endpoint returns 404.
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.Close()

	probeReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+erugoShareEndpoint(cfg), &body)
	if err != nil {
		return err
	}
	probeReq.Header.Set("Content-Type", writer.FormDataContentType())
	addErugoAuth(probeReq, cfg.APIKey)

	probeRes, err := client.Do(probeReq)
	if err != nil {
		return fmt.Errorf("failed to reach Erugo integration endpoint: %w", err)
	}
	defer probeRes.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(probeRes.Body, 4096))

	switch probeRes.StatusCode {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("Erugo rejected the API token with HTTP %d", probeRes.StatusCode)
	case http.StatusNotFound:
		return fmt.Errorf("Erugo integration endpoint was not found at %s", erugoShareEndpoint(cfg))
	}
	if probeRes.StatusCode >= 200 && probeRes.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("Erugo integration endpoint returned HTTP %d: %s", probeRes.StatusCode, strings.TrimSpace(string(responseBody)))
}

var erugoShareConfigHandler = withPermShare(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	backend := "filebrowser"
	if d.settings.Erugo.Enabled {
		backend = "erugo"
	}
	return renderJSON(w, r, erugoShareConfigResponse{Backend: backend, Enabled: d.settings.Erugo.Enabled})
})

var settingsErugoTestHandler = withAdmin(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	cfg := d.settings.Erugo
	if r.Body != nil {
		defer r.Body.Close()
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
			return http.StatusBadRequest, err
		}
	}
	if err := testErugoConnection(r.Context(), cfg); err != nil {
		return http.StatusBadGateway, err
	}
	return renderJSON(w, r, erugoTestResponse{Status: "OK", Message: "Erugo connection successful"})
})

func createErugoShare(ctx context.Context, d *data, requestedPath string, body share.CreateBody) (*share.Link, error) {
	cfg := d.settings.Erugo
	base, err := normalizeErugoURL(cfg.URL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("no Erugo API token is configured")
	}

	requestedPath = cleanFileBrowserPath(requestedPath)
	if requestedPath == "/" || !d.Check(requestedPath) {
		return nil, fberrors.ErrPermissionDenied
	}
	info, err := d.user.Fs.Stat(requestedPath)
	if err != nil {
		return nil, err
	}

	shareName := strings.TrimSpace(body.Name)
	if shareName == "" {
		shareName = info.Name()
	}
	filename := info.Name()
	if info.IsDir() {
		filename = strings.TrimSuffix(filename, string(os.PathSeparator)) + ".zip"
	}

	requestCtx, cancel := context.WithTimeout(ctx, erugoTimeout(cfg.Timeout))
	defer cancel()

	// Build the multipart request body into a temporary file instead of using
	// io.Pipe/chunked transfer encoding. Some PHP/Laravel deployments behind
	// reverse proxies do not reliably process chunked multipart uploads and can
	// leave the File Browser request waiting until the timeout. A temp file gives
	// the outbound request a real Content-Length, matching curl/browser uploads.
	tmp, err := os.CreateTemp("", "filebrowser-erugo-share-*.multipart")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	writer := multipart.NewWriter(tmp)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		_ = writer.Close()
		return nil, err
	}

	if info.IsDir() {
		commonDir := gopath.Dir(requestedPath)
		allFiles, err := getFiles(d, requestedPath, commonDir)
		if err != nil {
			_ = writer.Close()
			return nil, err
		}
		if len(allFiles) == 0 {
			_ = writer.Close()
			return nil, os.ErrNotExist
		}
		if err := (archives.Zip{}).Archive(requestCtx, part, allFiles); err != nil {
			_ = writer.Close()
			return nil, err
		}
	} else {
		file, err := d.user.Fs.Open(requestedPath)
		if err != nil {
			_ = writer.Close()
			return nil, err
		}
		if _, err := io.Copy(part, file); err != nil {
			_ = file.Close()
			_ = writer.Close()
			return nil, err
		}
		if err := file.Close(); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}

	if err := writer.WriteField("name", shareName); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if description := strings.TrimSpace(body.Description); description != "" {
		if err := writer.WriteField("description", description); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	if recipientEmail := strings.TrimSpace(body.RecipientEmail); recipientEmail != "" {
		if err := writer.WriteField("recipient_email", recipientEmail); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	if recipientName := strings.TrimSpace(body.RecipientName); recipientName != "" {
		if err := writer.WriteField("recipient_name", recipientName); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	expiryHours := erugoExpiryHours(body, cfg)
	if err := writer.WriteField("expires_in_hours", strconv.Itoa(expiryHours)); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.WriteField("expiry_hours", strconv.Itoa(expiryHours)); err != nil {
		_ = writer.Close()
		return nil, err
	}

	contentType := writer.FormDataContentType()
	if err := writer.Close(); err != nil {
		return nil, err
	}

	stat, err := tmp.Stat()
	if err != nil {
		return nil, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, base+erugoShareEndpoint(cfg), tmp)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = stat.Size()
	addErugoAuth(req, cfg.APIKey)

	client := &http.Client{Timeout: erugoTimeout(cfg.Timeout)}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Erugo share failed: %w", err)
	}
	defer res.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(res.Body, 64*1024))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("Erugo share failed with HTTP %d: %s", res.StatusCode, erugoErrorMessage(responseBody))
	}

	var erugoResponse erugoShareResponse
	if err := json.Unmarshal(responseBody, &erugoResponse); err != nil {
		return nil, fmt.Errorf("Erugo returned invalid JSON: %w", err)
	}
	shareURL := extractErugoShareURL(erugoResponse)
	if shareURL == "" {
		return nil, errors.New("Erugo did not return a share URL")
	}

	return &share.Link{
		Hash:            "erugo",
		Path:            requestedPath,
		UserID:          d.user.ID,
		Expire:          time.Now().Add(time.Duration(erugoExpiryHours(body, cfg)) * time.Hour).Unix(),
		URL:             shareURL,
		Name:            shareName,
		EmailSent:       erugoResponse.EmailSent,
		ExternalService: "erugo",
	}, nil
}

func erugoExpiryHours(body share.CreateBody, cfg settings.Erugo) int {
	if body.Expires != "" {
		num, err := strconv.Atoi(body.Expires)
		if err == nil && num > 0 {
			switch strings.ToLower(strings.TrimSpace(body.Unit)) {
			case "seconds":
				return maxInt(1, int((time.Duration(num)*time.Second+time.Hour-1)/time.Hour))
			case "minutes":
				return maxInt(1, int((time.Duration(num)*time.Minute+time.Hour-1)/time.Hour))
			case "days":
				return num * 24
			default:
				return num
			}
		}
	}
	if cfg.DefaultExpiryHours > 0 {
		return cfg.DefaultExpiryHours
	}
	return 168
}

func extractErugoShareURL(response erugoShareResponse) string {
	for _, value := range []string{response.ShareURL, response.ShareLink} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if len(response.Data) == 0 {
		return ""
	}
	var data map[string]any
	if err := json.Unmarshal(response.Data, &data); err != nil {
		return ""
	}
	for _, key := range []string{"url", "share_url", "share_link"} {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if shareData, ok := data["share"].(map[string]any); ok {
		for _, key := range []string{"url", "share_url", "share_link"} {
			if value, ok := shareData[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func erugoErrorMessage(responseBody []byte) string {
	trimmed := strings.TrimSpace(string(responseBody))
	if trimmed == "" {
		return "empty response from Erugo"
	}

	var payload struct {
		Message string `json:"message"`
		Error   string `json:"error"`
		Data    struct {
			Errors map[string][]string `json:"errors"`
		} `json:"data"`
	}
	if err := json.Unmarshal(responseBody, &payload); err == nil {
		if payload.Message != "" {
			if len(payload.Data.Errors) > 0 {
				parts := make([]string, 0, len(payload.Data.Errors))
				for field, messages := range payload.Data.Errors {
					if len(messages) > 0 {
						parts = append(parts, field+": "+strings.Join(messages, ", "))
					}
				}
				if len(parts) > 0 {
					return payload.Message + " (" + strings.Join(parts, "; ") + ")"
				}
			}
			return payload.Message
		}
		if payload.Error != "" {
			return payload.Error
		}
	}

	return trimmed
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
