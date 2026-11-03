package fbhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
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
const defaultErugoUploadTimeout = 2 * time.Hour
const defaultErugoShareEndpoint = "/api/integrations/convertx/share"
const defaultErugoReverseShareEndpoint = "/api/integrations/filebrowser/reverse-share"
const maxErugoBandwidthLimitKbps = 1_000_000

type erugoTestResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type erugoShareConfigResponse struct {
	Backend      string `json:"backend"`
	Enabled      bool   `json:"enabled"`
	ReverseShare bool   `json:"reverseShare"`
}

// erugoCreatedShare is an API-only response model. It must remain separate
// from share.Link, which is persisted by Storm/Bolt for File Browser's native
// sharing feature. Keeping external fields out of share.Link preserves complete
// compatibility with the upstream native share implementation.
type erugoCreatedShare struct {
	Hash                   string `json:"hash"`
	Path                   string `json:"path"`
	UserID                 uint   `json:"userID"`
	Expire                 int64  `json:"expire"`
	URL                    string `json:"url"`
	Name                   string `json:"name,omitempty"`
	EmailSent              bool   `json:"emailSent,omitempty"`
	ExternalService        string `json:"externalService"`
	DownloadSpeedLimitKbps int    `json:"downloadSpeedLimitKbps,omitempty"`
}

type erugoShareResponse struct {
	Status    string          `json:"status"`
	Message   string          `json:"message"`
	ShareURL  string          `json:"share_url"`
	ShareLink string          `json:"share_link"`
	EmailSent bool            `json:"email_sent"`
	Data      json.RawMessage `json:"data"`
}

type erugoReverseShareRequest struct {
	RecipientEmail       string `json:"recipientEmail"`
	RecipientName        string `json:"recipientName"`
	Message              string `json:"message,omitempty"`
	UploadSpeedLimitKbps int    `json:"uploadSpeedLimitKbps,omitempty"`
}

type erugoReverseShareResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Invite struct {
			ID                   int    `json:"id"`
			RecipientEmail       string `json:"recipient_email"`
			RecipientName        string `json:"recipient_name"`
			UploadSpeedLimitKbps int    `json:"upload_speed_limit_kbps"`
			ExpiresAt            string `json:"expires_at"`
		} `json:"invite"`
	} `json:"data"`
}

type erugoCreatedReverseShare struct {
	Status               string `json:"status"`
	Message              string `json:"message"`
	InviteID             int    `json:"inviteId"`
	RecipientEmail       string `json:"recipientEmail"`
	RecipientName        string `json:"recipientName"`
	UploadSpeedLimitKbps int    `json:"uploadSpeedLimitKbps"`
	ExpiresAt            string `json:"expiresAt,omitempty"`
}

type erugoUpstreamError struct {
	status  int
	message string
}

func (e *erugoUpstreamError) Error() string {
	return e.message
}

func erugoUpstreamStatus(err error) int {
	var upstreamErr *erugoUpstreamError
	if errors.As(err, &upstreamErr) {
		return upstreamErr.status
	}
	return 0
}

func normalizeErugoURL(raw string) (string, error) {
	cleaned := strings.TrimRight(strings.TrimSpace(raw), "/")
	if cleaned == "" {
		return "", errors.New("no Erugo URL is configured")
	}

	parsed, err := url.Parse(cleaned)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("erugo URL must be a valid absolute URL, for example http://erugo.lan:9998")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("erugo URL must use http or https")
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

func erugoUploadTimeout(raw string) time.Duration {
	if strings.TrimSpace(raw) == "" {
		return defaultErugoUploadTimeout
	}
	duration, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || duration <= 0 {
		return defaultErugoUploadTimeout
	}
	return duration
}

func normalizeErugoEndpoint(endpoint, fallback string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = fallback
	}
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}
	return endpoint
}

func erugoShareEndpoint(cfg settings.Erugo) string {
	return normalizeErugoEndpoint(cfg.ShareEndpoint, defaultErugoShareEndpoint)
}

func erugoReverseShareEndpoint(cfg settings.Erugo) string {
	return normalizeErugoEndpoint(cfg.ReverseShareEndpoint, defaultErugoReverseShareEndpoint)
}

func normalizeErugoBandwidthLimitKbps(value int) (int, error) {
	if value < 0 || value > maxErugoBandwidthLimitKbps {
		return 0, fmt.Errorf("bandwidth limit must be between 0 and %d Kbps", maxErugoBandwidthLimitKbps)
	}
	return value, nil
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
		return fmt.Errorf("erugo health check returned HTTP %d", healthRes.StatusCode)
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
		// Expected for an authenticated probe without a file.
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("erugo rejected the API token with HTTP %d", probeRes.StatusCode)
	case http.StatusNotFound:
		return fmt.Errorf("erugo integration endpoint was not found at %s", erugoShareEndpoint(cfg))
	default:
		if probeRes.StatusCode < 200 || probeRes.StatusCode >= 300 {
			return fmt.Errorf("erugo integration endpoint returned HTTP %d: %s", probeRes.StatusCode, strings.TrimSpace(string(responseBody)))
		}
	}

	// Verify the reverse-share integration route without creating an invitation.
	reverseProbeBody := bytes.NewBufferString(`{}`)
	reverseReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+erugoReverseShareEndpoint(cfg), reverseProbeBody)
	if err != nil {
		return err
	}
	reverseReq.Header.Set("Content-Type", "application/json")
	addErugoAuth(reverseReq, cfg.APIKey)
	reverseRes, err := client.Do(reverseReq)
	if err != nil {
		return fmt.Errorf("failed to reach Erugo reverse-share integration endpoint: %w", err)
	}
	defer reverseRes.Body.Close()
	reverseBody, _ := io.ReadAll(io.LimitReader(reverseRes.Body, 4096))
	switch reverseRes.StatusCode {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("erugo rejected the API token for reverse shares with HTTP %d", reverseRes.StatusCode)
	case http.StatusNotFound:
		return fmt.Errorf("erugo reverse-share integration endpoint was not found at %s", erugoReverseShareEndpoint(cfg))
	}
	if reverseRes.StatusCode >= 200 && reverseRes.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("erugo reverse-share endpoint returned HTTP %d: %s", reverseRes.StatusCode, strings.TrimSpace(string(reverseBody)))
}

var erugoShareConfigHandler = withPermShare(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	backend := "filebrowser"
	if d.settings.Erugo.Enabled {
		backend = "erugo"
	}
	return renderJSON(w, r, erugoShareConfigResponse{
		Backend:      backend,
		Enabled:      d.settings.Erugo.Enabled,
		ReverseShare: d.settings.Erugo.Enabled,
	})
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
	return renderJSON(w, r, erugoTestResponse{Status: "OK", Message: "erugo connection successful"})
})

var erugoReverseShareHandler = withPermShare(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	if !d.settings.Erugo.Enabled {
		return http.StatusBadRequest, errors.New("erugo integration is disabled")
	}

	var body erugoReverseShareRequest
	if r.Body == nil {
		return http.StatusBadRequest, errors.New("request body is required")
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		return http.StatusBadRequest, fmt.Errorf("failed to decode reverse-share request: %w", err)
	}

	body.RecipientEmail = strings.TrimSpace(body.RecipientEmail)
	body.RecipientName = strings.TrimSpace(body.RecipientName)
	body.Message = strings.TrimSpace(body.Message)
	if body.RecipientEmail == "" || body.RecipientName == "" {
		return http.StatusBadRequest, errors.New("recipient name and email are required")
	}
	if !strings.Contains(body.RecipientEmail, "@") {
		return http.StatusBadRequest, errors.New("recipient email is invalid")
	}
	limit, err := normalizeErugoBandwidthLimitKbps(body.UploadSpeedLimitKbps)
	if err != nil {
		return http.StatusBadRequest, err
	}
	body.UploadSpeedLimitKbps = limit

	created, err := createErugoReverseShare(r.Context(), d.settings.Erugo, body)
	if err != nil {
		if status := erugoUpstreamStatus(err); status != 0 {
			return status, err
		}
		return http.StatusBadGateway, err
	}
	return renderJSON(w, r, created)
})

func createErugoReverseShare(ctx context.Context, cfg settings.Erugo, body erugoReverseShareRequest) (*erugoCreatedReverseShare, error) {
	base, err := normalizeErugoURL(cfg.URL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("no Erugo API token is configured")
	}

	payload := map[string]any{
		"recipient_email":         body.RecipientEmail,
		"recipient_name":          body.RecipientName,
		"message":                 body.Message,
		"upload_speed_limit_kbps": body.UploadSpeedLimitKbps,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	requestCtx, cancel := context.WithTimeout(ctx, erugoTimeout(cfg.Timeout))
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, base+erugoReverseShareEndpoint(cfg), bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	addErugoAuth(req, cfg.APIKey)

	res, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("erugo reverse-share request failed: %w", err)
	}
	defer res.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(res.Body, 64*1024))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		status := http.StatusBadGateway
		if res.StatusCode >= 400 && res.StatusCode < 500 {
			status = res.StatusCode
		}
		return nil, &erugoUpstreamError{
			status:  status,
			message: fmt.Sprintf("erugo reverse-share creation failed with HTTP %d: %s", res.StatusCode, erugoErrorMessage(responseBody)),
		}
	}

	var erugoResponse erugoReverseShareResponse
	if err := json.Unmarshal(responseBody, &erugoResponse); err != nil {
		return nil, fmt.Errorf("erugo returned invalid reverse-share JSON: %w", err)
	}
	if erugoResponse.Data.Invite.ID <= 0 {
		return nil, errors.New("erugo did not return a reverse-share invitation ID")
	}

	return &erugoCreatedReverseShare{
		Status:               erugoResponse.Status,
		Message:              erugoResponse.Message,
		InviteID:             erugoResponse.Data.Invite.ID,
		RecipientEmail:       erugoResponse.Data.Invite.RecipientEmail,
		RecipientName:        erugoResponse.Data.Invite.RecipientName,
		UploadSpeedLimitKbps: erugoResponse.Data.Invite.UploadSpeedLimitKbps,
		ExpiresAt:            erugoResponse.Data.Invite.ExpiresAt,
	}, nil
}

func createErugoShare(ctx context.Context, d *data, requestedPath string, body share.CreateBody) (*erugoCreatedShare, error) {
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

	downloadLimit, err := normalizeErugoBandwidthLimitKbps(body.DownloadSpeedLimitKbps)
	if err != nil {
		return nil, &erugoUpstreamError{status: http.StatusBadRequest, message: err.Error()}
	}
	body.DownloadSpeedLimitKbps = downloadLimit

	shareName := strings.TrimSpace(body.Name)
	if shareName == "" {
		shareName = info.Name()
	}
	filename := info.Name()
	if info.IsDir() {
		filename = strings.TrimSuffix(filename, string(os.PathSeparator)) + ".zip"
	}

	uploadTimeout := erugoUploadTimeout(cfg.UploadTimeout)
	requestCtx, cancel := context.WithTimeout(ctx, uploadTimeout)
	defer cancel()

	var (
		requestBody   io.Reader
		contentType   string
		contentLength int64
	)

	jobState, _ := ctx.Value(erugoContextKey{}).(*erugoJobContext)
	switch {
	case info.IsDir() && jobState != nil && jobState.prepared != "":
		prefix, suffix, kind, e := buildErugoMultipartEnvelope(filename, body, cfg, shareName)
		if e != nil {
			return nil, e
		}
		fd, e := os.Open(jobState.prepared)
		if e != nil {
			return nil, e
		}
		defer fd.Close()
		stat, e := fd.Stat()
		if e != nil {
			return nil, e
		}
		requestBody = io.MultiReader(bytes.NewReader(prefix), fd, bytes.NewReader(suffix))
		contentType = kind
		contentLength = int64(len(prefix)+len(suffix)) + stat.Size()
	case info.IsDir():
		// Folder size is not known until the ZIP is produced, so folders still use
		// a temporary multipart file. Regular files use the zero-copy streaming
		// path below and do not consume an additional file-sized amount of /tmp.
		tmp, err := os.CreateTemp("", "filebrowser-erugo-share-*.multipart")
		if err != nil {
			return nil, err
		}
		cleanup := func() {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
		defer cleanup()

		writer := multipart.NewWriter(tmp)
		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			_ = writer.Close()
			return nil, err
		}

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
		if err := writeErugoShareFields(writer, body, cfg, shareName); err != nil {
			_ = writer.Close()
			return nil, err
		}
		contentType = writer.FormDataContentType()
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
		requestBody = tmp
		contentLength = stat.Size()
	default:
		// Build only the small multipart envelope in memory, then stream the
		// original file directly between the prefix and suffix. The request still
		// has an exact Content-Length, so PHP/Apache does not receive chunked
		// multipart data, but File Browser no longer creates a second 2+ GB copy.
		prefix, suffix, multipartType, err := buildErugoMultipartEnvelope(filename, body, cfg, shareName)
		if err != nil {
			return nil, err
		}
		file, err := d.user.Fs.Open(requestedPath)
		if err != nil {
			return nil, err
		}
		cleanup := func() { _ = file.Close() }
		defer cleanup()

		requestBody = io.MultiReader(bytes.NewReader(prefix), file, bytes.NewReader(suffix))
		contentType = multipartType
		contentLength = int64(len(prefix)) + info.Size() + int64(len(suffix))
		if contentLength < int64(len(prefix)+len(suffix)) {
			return nil, errors.New("erugo share failed: invalid source file size")
		}
	}

	if jobState != nil {
		if e := jobState.f.phase(jobState.j, "submitting"); e != nil {
			return nil, e
		}
		requestBody = erugoProgressReader{requestCtx, requestBody, jobState, contentLength}
	}
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, base+erugoShareEndpoint(cfg), requestBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	if jobState != nil {
		req.Header.Set("Idempotency-Key", jobState.j.ID)
	}
	req.ContentLength = contentLength
	addErugoAuth(req, cfg.APIKey)

	clamAVSize := info.Size()
	if info.IsDir() {
		// A folder is archived before upload, so only the completed multipart
		// length is available here. Erugo still makes the authoritative bypass
		// decision from the stored file size.
		clamAVSize = contentLength
	}
	largeFile := shouldBypassAutomaticClamAV(clamAVSize)
	if largeFile {
		// Ask the upstream server to approve the request headers before Go starts
		// sending a multi-gigabyte body. Erugo still decides the ClamAV bypass
		// from the actual stored file size; this header is informational only.
		req.Header.Set("Expect", "100-continue")
		req.Header.Set("X-Erugo-Source-Size", strconv.FormatInt(clamAVSize, 10))
	}

	started := time.Now()
	log.Printf("erugo share upload started: path=%q filename=%q bytes=%d timeout=%s clamav_bypass_expected=%t", requestedPath, filename, contentLength, uploadTimeout, largeFile)

	// The request context is the upload deadline. Leaving Client.Timeout unset
	// avoids a second, shorter timeout cutting off multi-gigabyte transfers.
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erugo share failed after %s: %w", time.Since(started).Round(time.Second), err)
	}
	defer res.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(res.Body, 64*1024))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		duration := time.Since(started).Round(time.Second)
		message := erugoErrorMessage(responseBody)

		switch res.StatusCode {
		case http.StatusRequestEntityTooLarge:
			return nil, &erugoUpstreamError{
				status: http.StatusRequestEntityTooLarge,
				message: fmt.Sprintf(
					"erugo rejected the upload as too large after %s. Increase Caddy request_body max_size and PHP upload_max_filesize/post_max_size on Erugo (upstream response: %s)",
					duration, message,
				),
			}
		case http.StatusUnprocessableEntity, http.StatusBadRequest:
			return nil, &erugoUpstreamError{
				status:  res.StatusCode,
				message: fmt.Sprintf("erugo rejected the share with HTTP %d after %s: %s", res.StatusCode, duration, message),
			}
		default:
			status := http.StatusBadGateway
			if res.StatusCode >= 400 && res.StatusCode < 500 {
				status = res.StatusCode
			}
			return nil, &erugoUpstreamError{
				status:  status,
				message: fmt.Sprintf("erugo share failed with HTTP %d after %s: %s", res.StatusCode, duration, message),
			}
		}
	}

	var erugoResponse erugoShareResponse
	if err := json.Unmarshal(responseBody, &erugoResponse); err != nil {
		return nil, fmt.Errorf("erugo returned invalid JSON: %w", err)
	}
	shareURL := extractErugoShareURL(erugoResponse)
	if shareURL == "" {
		return nil, errors.New("erugo did not return a share URL")
	}

	log.Printf("erugo share upload completed: path=%q bytes=%d duration=%s", requestedPath, contentLength, time.Since(started).Round(time.Second))

	return &erugoCreatedShare{
		Hash:                   "erugo",
		Path:                   requestedPath,
		UserID:                 d.user.ID,
		Expire:                 time.Now().Add(time.Duration(erugoExpiryHours(body, cfg)) * time.Hour).Unix(),
		URL:                    shareURL,
		Name:                   shareName,
		EmailSent:              erugoResponse.EmailSent,
		ExternalService:        "erugo",
		DownloadSpeedLimitKbps: body.DownloadSpeedLimitKbps,
	}, nil
}

func writeErugoShareFields(writer *multipart.Writer, body share.CreateBody, cfg settings.Erugo, shareName string) error {
	if err := writer.WriteField("name", shareName); err != nil {
		return err
	}
	if description := strings.TrimSpace(body.Description); description != "" {
		if err := writer.WriteField("description", description); err != nil {
			return err
		}
	}
	if recipientEmail := strings.TrimSpace(body.RecipientEmail); recipientEmail != "" {
		if err := writer.WriteField("recipient_email", recipientEmail); err != nil {
			return err
		}
	}
	if recipientName := strings.TrimSpace(body.RecipientName); recipientName != "" {
		if err := writer.WriteField("recipient_name", recipientName); err != nil {
			return err
		}
	}
	if body.DownloadSpeedLimitKbps > 0 {
		if err := writer.WriteField("download_speed_limit_kbps", strconv.Itoa(body.DownloadSpeedLimitKbps)); err != nil {
			return err
		}
	}
	expiryHours := strconv.Itoa(erugoExpiryHours(body, cfg))
	if err := writer.WriteField("expires_in_hours", expiryHours); err != nil {
		return err
	}
	return writer.WriteField("expiry_hours", expiryHours)
}

func buildErugoMultipartEnvelope(filename string, body share.CreateBody, cfg settings.Erugo, shareName string) ([]byte, []byte, string, error) {
	var envelope bytes.Buffer
	writer := multipart.NewWriter(&envelope)
	if _, err := writer.CreateFormFile("file", filename); err != nil {
		return nil, nil, "", err
	}
	prefixLength := envelope.Len()
	if err := writeErugoShareFields(writer, body, cfg, shareName); err != nil {
		_ = writer.Close()
		return nil, nil, "", err
	}
	contentType := writer.FormDataContentType()
	if err := writer.Close(); err != nil {
		return nil, nil, "", err
	}

	all := envelope.Bytes()
	prefix := append([]byte(nil), all[:prefixLength]...)
	suffix := append([]byte(nil), all[prefixLength:]...)
	return prefix, suffix, contentType, nil
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
