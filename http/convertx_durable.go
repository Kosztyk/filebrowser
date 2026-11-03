package fbhttp

import (
	"context"
	"errors"
	"github.com/filebrowser/filebrowser/v2/settings"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

func phaseTimeout(raw string, fallback time.Duration) time.Duration {
	d, e := time.ParseDuration(raw)
	if e != nil || d <= 0 || d > 24*time.Hour {
		return fallback
	}
	return d
}
func convertClient(cfg settings.ConvertX, timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: phaseTimeout(cfg.ConnectTimeout, 15*time.Second), KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = phaseTimeout(cfg.ConnectTimeout, 15*time.Second)
	transport.ResponseHeaderTimeout = phaseTimeout(cfg.Timeout, 2*time.Minute)
	return &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if len(via) > 0 && (req.URL.Scheme != via[0].URL.Scheme || req.URL.Host != via[0].URL.Host) {
			return errors.New("cross-origin ConvertX redirect rejected")
		}
		return nil
	}}
}
func (f *featureRuntime) convert(ctx context.Context, j *Job, d *data, p *convertXConvertPlan) error {
	cfg := d.settings.ConvertX
	submit := j.Upstream
	if submit == nil {
		if e := f.phase(j, "submitting"); e != nil {
			return e
		}
		file, e := d.user.Fs.Open(p.Source)
		if e != nil {
			return e
		}
		upload, cancel := context.WithTimeout(ctx, phaseTimeout(cfg.UploadTimeout, 2*time.Hour))
		submit, e = submitConvertXConversion(context.WithValue(upload, convertKey{}, j.ID), cfg, filepath.Base(p.Source), p.From, p.ConvertTo, p.Converter, contextReader{upload, file})
		_ = file.Close()
		cancel()
		if e != nil {
			return e
		}
		j.Upstream = submit
		if e = f.change(j.ID, func(c *Job) error { c.Upstream = submit; c.Phase = "processing"; return nil }); e != nil {
			return e
		}
	}
	if e := f.phase(j, "processing"); e != nil {
		return e
	}
	processing, cancel := context.WithTimeout(ctx, phaseTimeout(cfg.ProcessTimeout, 2*time.Hour))
	output, e := pollConvertXOutput(processing, cfg, submit, j.ID, f)
	cancel()
	if e != nil {
		return e
	}
	if e = f.phase(j, "downloading"); e != nil {
		return e
	}
	downloadCtx, cancel := context.WithTimeout(ctx, phaseTimeout(cfg.DownloadTimeout, 2*time.Hour))
	defer cancel()
	base, e := normalizeConvertXURL(cfg.URL)
	if e != nil {
		return e
	}
	download, e := downloadConvertXOutput(downloadCtx, convertClient(cfg, 0), cfg.APIKey, base, output)
	if e != nil {
		return e
	}
	defer download.Close()
	return runPublicationHook(d, "upload", p.Destination, func() error { return f.publishJob(downloadCtx, j, d, p.Destination, download) })
}

type convertKey struct{}

func pollConvertXOutput(ctx context.Context, cfg settings.ConvertX, submit *convertXSubmitResponse, id string, f *featureRuntime) (string, error) {
	base, e := normalizeConvertXURL(cfg.URL)
	if e != nil {
		return "", e
	}
	client := convertClient(cfg, phaseTimeout(cfg.Timeout, 2*time.Minute))
	defer client.CloseIdleConnections()
	t := time.NewTicker(convertXJobPollInterval)
	defer t.Stop()
	for {
		job, e := fetchConvertXJob(ctx, client, cfg.APIKey, base, submit.JobID)
		if e != nil {
			return "", e
		}
		if f != nil {
			f.progress(id, int64(job.Done), int64(job.Total))
		}
		if strings.EqualFold(job.Status, "failed") || strings.EqualFold(job.Status, "error") {
			return "", convertXMessageError(job.Message)
		}
		if strings.EqualFold(job.Status, "completed") || strings.EqualFold(job.Status, "done") {
			u := firstConvertXDownloadURL(job, submit.ExpectedOutput)
			if u == "" {
				return "", errors.New("ConvertX did not return output")
			}
			return u, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-t.C:
		}
	}
}
func sameConvertOrigin(base, target string) (string, error) {
	b, e := url.Parse(base)
	if e != nil {
		return "", e
	}
	u, e := url.Parse(target)
	if e != nil {
		return "", e
	}
	u = b.ResolveReference(u)
	if u.Host != b.Host || u.Scheme != b.Scheme || u.User != nil {
		return "", errors.New("ConvertX output URL must belong to its configured origin")
	}
	return u.String(), nil
}
