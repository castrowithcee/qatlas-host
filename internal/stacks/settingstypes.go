package stacks

// RetentionSettings is the retention of BackupSettings.
type RetentionSettings struct {
	Keep       *int `yaml:"keep,omitempty" json:"keep,omitempty"`
	MaxAgeDays *int `yaml:"max_age_days,omitempty" json:"max_age_days,omitempty"`
}

// CheckSettings is the check section of Settings.
type CheckSettings struct {
	Containers *bool         `yaml:"containers,omitempty" json:"containers,omitempty"`
	HTTP       *HTTPSettings `yaml:"http,omitempty" json:"http,omitempty"`
}

// HTTPSettings is the HTTP probe of CheckSettings.
type HTTPSettings struct {
	URL     string  `yaml:"url" json:"url"`
	Status  *int    `yaml:"status,omitempty" json:"status,omitempty"`
	Timeout *string `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// PromoteSettings is the promote section of Settings.
type PromoteSettings struct {
	CompareImages *bool `yaml:"compare_images,omitempty" json:"compare_images,omitempty"`
}

func (p *PromoteSettings) compare() bool {
	return p != nil && p.CompareImages != nil && *p.CompareImages
}

// promoteSettings is the promote section of the rules, nil when off: the settings of a stack that never
// turned it on stay as they are.
func (r *Rules) promoteSettings() *PromoteSettings {
	if !r.PromoteCompare {
		return nil
	}
	return &PromoteSettings{CompareImages: ptr(true)}
}

// ComparesPromotion says whether a promotion into the stack compares the fingerprints of the images with the
// source's, see Rules.PromoteCompare.
func (s Stack) ComparesPromotion() bool { return s.Rules != nil && s.Rules.PromoteCompare }
