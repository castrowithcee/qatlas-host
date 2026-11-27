// Package usage reads the current load of the machine from /proc and statfs, without root. A value the
// environment does not provide is left empty instead of failing the read.
package usage

import (
	"context"
	"syscall"
	"time"
)

// Usage is the current load of the host. Pointers and empty slices stand for values the environment does not
// provide.
type Usage struct {
	// CPUPercent is the busy share of all cores over the measuring interval, 0 to 100.
	CPUPercent    *float64     `json:"cpu_percent,omitempty"`
	Cores         int          `json:"cores,omitempty"`
	Load          *Load        `json:"load,omitempty"`
	Memory        *Memory      `json:"memory,omitempty"`
	Swap          *Memory      `json:"swap,omitempty"`
	Filesystems   []Filesystem `json:"filesystems,omitempty"`
	UptimeSeconds *int64       `json:"uptime_seconds,omitempty"`
}

// Load is the load average of the last 1, 5 and 15 minutes.
type Load struct {
	One     float64 `json:"one"`
	Five    float64 `json:"five"`
	Fifteen float64 `json:"fifteen"`
}

// Memory is the usage of RAM or swap in bytes. Used is Total minus Available.
type Memory struct {
	TotalBytes     uint64 `json:"total_bytes"`
	UsedBytes      uint64 `json:"used_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
}

// Filesystem is one mounted real file system. FreeBytes is what an unprivileged user can still write.
type Filesystem struct {
	Mount      string `json:"mount"`
	Device     string `json:"device"`
	Type       string `json:"type"`
	TotalBytes uint64 `json:"total_bytes"`
	UsedBytes  uint64 `json:"used_bytes"`
	FreeBytes  uint64 `json:"free_bytes"`
}

// FSStat is the part of statfs the reader needs.
type FSStat struct {
	BlockSize, Blocks, Free, Available uint64
}

// Source is where a Reader reads from; tests and later users replace its fields.
type Source struct {
	// ProcRoot is the folder of the proc file system.
	ProcRoot string
	// Mounts is the mount table; empty means self/mounts below ProcRoot.
	Mounts string
	Statfs func(path string) (FSStat, error)
	// Pause waits between the two CPU measurements and ends early with the error of ctx.
	Pause func(ctx context.Context) error
}

// cpuInterval is long enough for a stable share and short enough not to hold a request.
const cpuInterval = 200 * time.Millisecond

// NewSource returns the source of the running machine.
func NewSource() Source {
	return Source{ProcRoot: "/proc", Statfs: realStatfs, Pause: func(ctx context.Context) error {
		t := time.NewTimer(cpuInterval)
		defer t.Stop()
		select {
		case <-t.C:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
}

// realStatfs is shared by every source of the process, so hanging mounts are tracked once.
var realStatfs = bounded(statfs, statfsTimeout)

func statfs(path string) (FSStat, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return FSStat{}, err
	}
	return FSStat{BlockSize: uint64(st.Bsize), Blocks: st.Blocks, Free: st.Bfree, Available: st.Bavail}, nil
}

// Read measures the host now. It fails only when ctx ends during the CPU measurement.
func (s Source) Read(ctx context.Context) (Usage, error) {
	var u Usage
	first, ok := s.cpuSample()
	if ok && s.Pause != nil {
		if err := s.Pause(ctx); err != nil {
			return Usage{}, err
		}
		if second, ok2 := s.cpuSample(); ok2 {
			u.CPUPercent = busyPercent(first, second)
		}
	}
	u.Cores = s.cores()
	u.Load = s.load()
	u.Memory, u.Swap = s.memory()
	u.Filesystems = s.filesystems()
	u.UptimeSeconds = s.uptime()
	return u, nil
}
