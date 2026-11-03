package settings

const DefaultTusChunkSize = 10 * 1024 * 1024 // 10MB
const DefaultTusRetryCount = 5

// Tus contains the tus.io settings of the app.
type Tus struct {
	RetentionHours    int    `json:"retentionHours"`
	ConcurrentUploads int    `json:"concurrentUploads"`
	MaxActiveUploads  int    `json:"maxActiveUploads"`
	MaxPendingBytes   int64  `json:"maxPendingBytes"`
	ReserveBytes      uint64 `json:"reserveBytes"`
	ChunkSize         uint64 `json:"chunkSize"`
	RetryCount        uint16 `json:"retryCount"`
}
