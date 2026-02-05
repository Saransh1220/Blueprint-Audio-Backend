package dto

type LicenseDownloadsResponse struct {
	LicenseID   string  `json:"license_id"`
	LicenseType string  `json:"license_type"`
	SpecTitle   string  `json:"spec_title"`
	ExpiresIn   int     `json:"expires_in"`
	MP3URL      *string `json:"mp3_url,omitempty"`
	WAVURL      *string `json:"wav_url,omitempty"`
	StemsURL    *string `json:"stems_url,omitempty"`
}
