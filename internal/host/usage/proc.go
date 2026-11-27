package usage

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// cpuSample is the cumulative jiffies of /proc/stat.
type cpuSample struct{ total, idle uint64 }

func (s Source) readFile(name string) ([]byte, bool) {
	b, err := os.ReadFile(filepath.Join(s.ProcRoot, name))
	return b, err == nil
}

func (s Source) cpuSample() (cpuSample, bool) {
	b, ok := s.readFile("stat")
	if !ok {
		return cpuSample{}, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 5 || f[0] != "cpu" {
			continue
		}
		var c cpuSample
		// user..steal; guest time is already part of user.
		for i, v := range f[1:min(len(f), 9)] {
			n, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return cpuSample{}, false
			}
			c.total += n
			if i == 3 || i == 4 {
				c.idle += n
			}
		}
		return c, true
	}
	return cpuSample{}, false
}

// busyPercent is nil when no time passed between the samples.
func busyPercent(a, b cpuSample) *float64 {
	if b.total <= a.total || b.idle < a.idle {
		return nil
	}
	dt, di := b.total-a.total, b.idle-a.idle
	if di > dt {
		return nil
	}
	p := float64(dt-di) * 100 / float64(dt)
	return &p
}

func (s Source) cores() int {
	b, ok := s.readFile("stat")
	if !ok {
		return 0
	}
	n := 0
	for _, line := range strings.Split(string(b), "\n") {
		name, _, _ := strings.Cut(line, " ")
		if id, found := strings.CutPrefix(name, "cpu"); found && id != "" {
			if _, err := strconv.Atoi(id); err == nil {
				n++
			}
		}
	}
	return n
}

func (s Source) load() *Load {
	b, ok := s.readFile("loadavg")
	f := strings.Fields(string(b))
	if !ok || len(f) < 3 {
		return nil
	}
	var v [3]float64
	for i := range v {
		x, err := strconv.ParseFloat(f[i], 64)
		if err != nil {
			return nil
		}
		v[i] = x
	}
	return &Load{One: v[0], Five: v[1], Fifteen: v[2]}
}

func (s Source) uptime() *int64 {
	b, ok := s.readFile("uptime")
	f := strings.Fields(string(b))
	if !ok || len(f) < 1 {
		return nil
	}
	x, err := strconv.ParseFloat(f[0], 64)
	if err != nil || x < 0 {
		return nil
	}
	n := int64(x)
	return &n
}

// memory returns RAM and swap; swap is nil when the host has none.
func (s Source) memory() (ram, swap *Memory) {
	f, err := os.Open(filepath.Join(s.ProcRoot, "meminfo"))
	if err != nil {
		return nil, nil
	}
	defer f.Close()
	kb := map[string]uint64{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, rest, ok := strings.Cut(sc.Text(), ":")
		fields := strings.Fields(rest)
		if !ok || len(fields) == 0 {
			continue
		}
		if n, err := strconv.ParseUint(fields[0], 10, 64); err == nil {
			kb[key] = n * 1024
		}
	}
	if total, ok := kb["MemTotal"]; ok {
		avail, have := kb["MemAvailable"]
		if !have {
			avail = kb["MemFree"] + kb["Buffers"] + kb["Cached"]
		}
		ram = newMemory(total, avail)
	}
	if total := kb["SwapTotal"]; total > 0 {
		swap = newMemory(total, kb["SwapFree"])
	}
	return ram, swap
}

func newMemory(total, avail uint64) *Memory {
	avail = min(avail, total)
	return &Memory{TotalBytes: total, UsedBytes: total - avail, AvailableBytes: avail}
}
