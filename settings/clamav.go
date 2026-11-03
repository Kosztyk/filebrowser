package settings

// ClamAV contains upload antivirus scanning settings.
type ClamAV struct {
	AutomaticMaxBytes int64  `json:"automaticMaxBytes"`
	OversizePolicy    string `json:"oversizePolicy"`
	ScanWorkers       int    `json:"scanWorkers"`
	ScanTimeout       string `json:"scanTimeout"`
	Enabled           bool   `json:"enabled"`
	URL               string `json:"url"`
	ScanDepth         int    `json:"scanDepth"`
}
