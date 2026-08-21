package fbhttp

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"strconv"
	"testing"

	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/share"
)

func TestNormalizeErugoBandwidthLimitKbps(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		want    int
		wantErr bool
	}{
		{name: "unlimited", value: 0, want: 0},
		{name: "one megabit", value: 1000, want: 1000},
		{name: "maximum", value: maxErugoBandwidthLimitKbps, want: maxErugoBandwidthLimitKbps},
		{name: "negative", value: -1, wantErr: true},
		{name: "above maximum", value: maxErugoBandwidthLimitKbps + 1, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeErugoBandwidthLimitKbps(test.value)
			if test.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %d", test.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.want {
				t.Fatalf("got %d, want %d", got, test.want)
			}
		})
	}
}

func TestWriteErugoShareFieldsBandwidthLimit(t *testing.T) {
	for _, limit := range []int{0, 50000} {
		t.Run(strconv.Itoa(limit), func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			contentType := writer.FormDataContentType()
			if err := writeErugoShareFields(writer, share.CreateBody{
				DownloadSpeedLimitKbps: limit,
			}, settings.Erugo{DefaultExpiryHours: 168}, "example"); err != nil {
				t.Fatalf("write fields: %v", err)
			}
			if err := writer.Close(); err != nil {
				t.Fatalf("close writer: %v", err)
			}

			_, params, err := mime.ParseMediaType(contentType)
			if err != nil {
				t.Fatalf("parse content type: %v", err)
			}
			reader := multipart.NewReader(bytes.NewReader(body.Bytes()), params["boundary"])
			fields := map[string]string{}
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("read multipart: %v", err)
				}
				value, err := io.ReadAll(part)
				if err != nil {
					t.Fatalf("read field: %v", err)
				}
				fields[part.FormName()] = string(value)
			}

			if limit == 0 {
				if _, exists := fields["download_speed_limit_kbps"]; exists {
					t.Fatal("unlimited shares must omit download_speed_limit_kbps")
				}
			} else if fields["download_speed_limit_kbps"] != strconv.Itoa(limit) {
				t.Fatalf("got bandwidth field %q", fields["download_speed_limit_kbps"])
			}
		})
	}
}

func TestErugoReverseShareEndpointDefault(t *testing.T) {
	if got := erugoReverseShareEndpoint(settings.Erugo{}); got != defaultErugoReverseShareEndpoint {
		t.Fatalf("got %q, want %q", got, defaultErugoReverseShareEndpoint)
	}
	if got := erugoReverseShareEndpoint(settings.Erugo{ReverseShareEndpoint: "custom/reverse"}); got != "/custom/reverse" {
		t.Fatalf("got %q, want /custom/reverse", got)
	}
}
