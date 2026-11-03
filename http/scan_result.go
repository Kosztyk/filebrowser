package fbhttp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/spf13/afero"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type ScanResult struct {
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

func explicitCleanText(value string) bool {
	for _, line := range strings.Split(strings.ToUpper(strings.TrimSpace(value)), "\n") {
		line = strings.TrimSpace(line)
		if line != "OK" && line != "CLEAN" && !strings.HasSuffix(line, ": OK") {
			return false
		}
	}
	return true
}
func scannerHasError(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return strings.TrimSpace(v) != ""
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	default:
		return true
	}
}
func explicitCleanJSON(value any) bool {
	switch v := value.(type) {
	case string:
		return explicitCleanText(v)
	case []any:
		if len(v) == 0 {
			return false
		}
		for _, item := range v {
			if !explicitCleanJSON(item) {
				return false
			}
		}
		return true
	case map[string]any:
		direct, nested := false, false
		for key, raw := range v {
			name := normalizeClamAVKey(key)
			if name == "success" {
				if b, ok := raw.(bool); ok && !b {
					return false
				}
			}
			if (name == "error" || name == "errors") && scannerHasError(raw) {
				return false
			}
			if isInfectionBoolKey(name) {
				if b, ok := raw.(bool); ok {
					if b {
						return false
					}
					direct = true
				}
			}
			if name == "status" || name == "scan_status" || name == "scan_result" || name == "result" {
				if text, ok := raw.(string); ok {
					if explicitCleanText(text) {
						direct = true
					} else if text != "" && !strings.EqualFold(text, "success") {
						return false
					}
				}
			}
			switch name {
			case "data", "results", "files", "scans", "result", "scan_result":
				switch raw.(type) {
				case []any, map[string]any:
					nested = true
					if !explicitCleanJSON(raw) {
						return false
					}
				}
			}
		}
		return direct || nested
	}
	return false
}
func scanUpload(ctx context.Context, fs afero.Fs, path string, cfg settings.ClamAV, names ...string) (ScanResult, error) {
	if !cfg.Enabled {
		return ScanResult{Status: "disabled", Message: "Automatic scanning is disabled"}, nil
	}
	if cfg.AutomaticMaxBytes == 0 {
		cfg.AutomaticMaxBytes = clamAVAutomaticScanMaxBytes
	}
	if cfg.OversizePolicy == "" {
		cfg.OversizePolicy = "skip"
	}
	info, e := fs.Stat(path)
	if e != nil {
		return ScanResult{Status: "error", Message: e.Error()}, e
	}
	if cfg.AutomaticMaxBytes >= 0 && info.Size() > cfg.AutomaticMaxBytes && cfg.OversizePolicy != "scan" {
		r := ScanResult{Status: "skipped", Reason: "size_limit", Message: "File exceeds the configured automatic scan limit"}
		if cfg.OversizePolicy == "block" {
			return r, &clamAVServiceError{Message: "oversized file kept unpublished by scan policy"}
		}
		return r, nil
	}
	f, e := fs.Open(path)
	if e != nil {
		return ScanResult{Status: "error", Message: e.Error()}, e
	}
	defer f.Close()
	name := filepath.Base(path)
	if len(names) > 0 {
		name = filepath.Base(names[0])
	}
	e = scanWithClamAV(ctx, cfg, name, f)
	if e == nil {
		return ScanResult{Status: "clean", Message: "Scanner explicitly reported a clean result"}, nil
	}
	r := ScanResult{Status: "error", Message: e.Error()}
	var threat *clamAVThreatError
	var service *clamAVServiceError
	if errors.As(e, &threat) {
		r.Status = "infected"
	}
	if errors.As(e, &service) && service.Unknown {
		r.Status = "unknown"
	}
	return r, e
}
func writeScannedFile(ctx context.Context, fs afero.Fs, dest string, in io.Reader, cfg settings.ClamAV, mode, dirMode os.FileMode) (os.FileInfo, ScanResult, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return nil, ScanResult{}, e
	}
	s, f, e := files.NewStagedFile(fs, dest, hex.EncodeToString(b), mode, dirMode)
	if e != nil {
		return nil, ScanResult{}, e
	}
	defer func() { _ = s.Abort() }()
	_, e = io.Copy(f, in)
	if e == nil {
		e = f.Sync()
	}
	c := f.Close()
	if e != nil {
		return nil, ScanResult{}, e
	}
	if c != nil {
		return nil, ScanResult{}, c
	}
	result, e := scanUpload(ctx, s.Fs, s.Path, cfg, dest)
	if e != nil {
		return nil, result, e
	}
	info, e := s.Commit()
	return info, result, e
}
func runPublicationHook(d *data, event, path string, action func() error) error {
	published := false
	e := d.RunHook(func() error { e := action(); published = e == nil; return e }, event, path, "", d.user)
	if e != nil && published {
		log.Printf("after-publication hook failed for %q: %v", path, e)
		return nil
	}
	return e
}
