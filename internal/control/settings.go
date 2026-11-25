package control

// StackRetentionSettings keep a backup while it is among the newest Keep and younger than MaxAgeDays, for
// each limit that is set; Keep is 7 when neither is set.
type StackRetentionSettings struct {
	Keep       int `json:"keep,omitempty"`
	MaxAgeDays int `json:"max_age_days,omitempty"`
}

// StackCheckSettings say when a deployed stack counts as healthy.
type StackCheckSettings struct {
	Containers *bool              `json:"containers,omitempty"`
	HTTP       *StackHTTPSettings `json:"http,omitempty"`
}

// StackHTTPSettings are an HTTP probe: the URL must answer Status (200 by default) within Timeout, a Go
// duration from 1s to 10m (10s by default).
type StackHTTPSettings struct {
	URL     string `json:"url"`
	Status  int    `json:"status,omitempty"`
	Timeout string `json:"timeout,omitempty"`
}

// StackPromoteSettings say how a promotion into the stack is judged. CompareImages (false when left out) makes
// it refuse unless the images the target loads are the ones the source runs, by their fingerprints.
type StackPromoteSettings struct {
	CompareImages *bool `json:"compare_images,omitempty"`
}
