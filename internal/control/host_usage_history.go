package control

import "time"

// HostUsageHistoryCommand reads the history of the host load the host service keeps.
const HostUsageHistoryCommand = "usage.history"

// CodeNoHistory answers HostUsageHistoryCommand when the history is off or its database cannot be used. The
// message names the reason and the next step.
const CodeNoHistory = "no-history"

// HostUsageHistoryArgs are the arguments of HostUsageHistoryCommand. From and To are RFC 3339 times; without
// them the period is the last hour. Resolution is auto (the default), raw, minute or hour. Metrics selects
// series by name; empty means all.
type HostUsageHistoryArgs struct {
	From       string   `json:"from,omitempty"`
	To         string   `json:"to,omitempty"`
	Resolution string   `json:"resolution,omitempty"`
	Metrics    []string `json:"metrics,omitempty"`
}

// HostUsageHistoryResult answers HostUsageHistoryCommand with the resolution used. A gap in the history is a
// missing point.
type HostUsageHistoryResult struct {
	Resolution string            `json:"resolution"`
	From       time.Time         `json:"from"`
	To         time.Time         `json:"to"`
	Series     []HostUsageSeries `json:"series"`
}

// HostUsageSeries is one metric over the period; Mount and TotalBytes are set for a file system, TotalBytes
// being its largest size in the period.
type HostUsageSeries struct {
	Metric     string           `json:"metric"`
	Mount      string           `json:"mount,omitempty"`
	TotalBytes int64            `json:"total_bytes,omitempty"`
	Points     []HostUsagePoint `json:"points"`
}

// HostUsagePoint is one bucket at Time (unix seconds). The raw resolution has Value; minutes and hours have the
// Mean and the Max of the bucket.
type HostUsagePoint struct {
	Time  int64    `json:"t"`
	Value *float64 `json:"v,omitempty"`
	Mean  *float64 `json:"mean,omitempty"`
	Max   *float64 `json:"max,omitempty"`
}
