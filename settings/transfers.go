package settings

import (
	"fmt"
	"time"
)

func NormalizeTransfers(u *Tus, c *ClamAV) {
	if u.ChunkSize == 0 {
		u.ChunkSize = DefaultTusChunkSize
	}
	if u.RetentionHours == 0 {
		u.RetentionHours = 72
		if u.ReserveBytes == 0 {
			u.ReserveBytes = 64 << 20
		}
	}
	if u.ConcurrentUploads == 0 {
		u.ConcurrentUploads = 5
	}
	if u.MaxActiveUploads == 0 {
		u.MaxActiveUploads = 1000
	}
	if c.AutomaticMaxBytes == 0 {
		c.AutomaticMaxBytes = 1900000000
	}
	if c.OversizePolicy == "" {
		c.OversizePolicy = "skip"
	}
	if c.ScanWorkers == 0 {
		c.ScanWorkers = 2
	}
	if c.ScanTimeout == "" {
		c.ScanTimeout = "30m"
	}
}
func ValidateTransfers(u Tus, c ClamAV) error {
	NormalizeTransfers(&u, &c)
	if u.RetentionHours < 1 || u.RetentionHours > 720 {
		return fmt.Errorf("upload retention must be 1–720 hours")
	}
	if u.ConcurrentUploads < 1 || u.ConcurrentUploads > 32 {
		return fmt.Errorf("upload concurrency must be 1–32")
	}
	if u.MaxActiveUploads < 1 || u.MaxActiveUploads > 10000 || u.MaxPendingBytes < 0 {
		return fmt.Errorf("invalid unfinished upload quota")
	}
	if u.ChunkSize > 1<<30 {
		return fmt.Errorf("chunk size exceeds 1 GiB")
	}
	if c.AutomaticMaxBytes < -1 {
		return fmt.Errorf("invalid automatic scan limit")
	}
	if c.OversizePolicy != "skip" && c.OversizePolicy != "block" && c.OversizePolicy != "scan" {
		return fmt.Errorf("invalid oversized-file scan policy")
	}
	if c.ScanWorkers < 1 || c.ScanWorkers > 8 {
		return fmt.Errorf("scan workers must be 1–8")
	}
	duration, e := time.ParseDuration(c.ScanTimeout)
	if e != nil || duration < time.Second || duration > 24*time.Hour {
		return fmt.Errorf("scan timeout must be 1s–24h")
	}
	return nil
}
