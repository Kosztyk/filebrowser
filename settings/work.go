package settings

import "fmt"

// Work defines bounded background and archive resource use. Zero values migrate
// older databases to the conservative defaults below.
type Work struct {
	Workers        int   `json:"workers"`
	MaxJobs        int   `json:"maxJobs"`
	ArchiveBytes   int64 `json:"archiveBytes"`
	ArchiveEntries int64 `json:"archiveEntries"`
	ArchiveDepth   int   `json:"archiveDepth"`
	ArchiveSeconds int   `json:"archiveSeconds"`
	ReserveBytes   int64 `json:"reserveBytes"`
	ArtifactBytes  int64 `json:"artifactBytes"`
}

func (w Work) Effective() Work {
	if w.Workers < 1 || w.Workers > 8 {
		w.Workers = 2
	}
	if w.MaxJobs < 1 || w.MaxJobs > 10000 {
		w.MaxJobs = 1000
	}
	if w.ArchiveBytes <= 0 {
		w.ArchiveBytes = 100 << 30
	}
	if w.ArchiveEntries <= 0 {
		w.ArchiveEntries = 100000
	}
	if w.ArchiveDepth <= 0 {
		w.ArchiveDepth = 64
	}
	if w.ArchiveSeconds <= 0 {
		w.ArchiveSeconds = 3600
	}
	if w.ReserveBytes <= 0 {
		w.ReserveBytes = 1 << 30
	}
	if w.ArtifactBytes <= 0 {
		w.ArtifactBytes = 100 << 30
	}
	return w
}

func (w Work) Validate() error {
	if w.Workers < 0 || w.Workers > 8 || w.MaxJobs < 0 || w.MaxJobs > 10000 || w.ArchiveBytes < 0 || w.ArchiveEntries < 0 || w.ArchiveEntries > 1000000 || w.ArchiveDepth < 0 || w.ArchiveDepth > 256 || w.ArchiveSeconds < 0 || w.ArchiveSeconds > 86400 || w.ReserveBytes < 0 || w.ArtifactBytes < 0 {
		return fmt.Errorf("invalid background/archive limits: workers 1–8, jobs up to 10000, entries up to 1000000, depth up to 256, duration up to 86400 seconds; byte limits must be positive")
	}
	return nil
}
