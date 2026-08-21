package settings

// Erugo stores the optional Erugo share integration settings.
//
// When enabled, File Browser's Share action creates an Erugo share by sending
// the selected file/folder to Erugo's backend-to-backend integration endpoint.
type Erugo struct {
	Configured           bool   `json:"configured"`
	Enabled              bool   `json:"enabled"`
	URL                  string `json:"url"`
	APIKey               string `json:"apiKey"`
	ShareEndpoint        string `json:"shareEndpoint"`
	ReverseShareEndpoint string `json:"reverseShareEndpoint"`
	DefaultExpiryHours   int    `json:"defaultExpiryHours"`
	Timeout              string `json:"timeout"`
	UploadTimeout        string `json:"uploadTimeout"`
}
