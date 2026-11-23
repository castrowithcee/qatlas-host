package tui

import (
	"fmt"
	"time"
)

// humanSize writes a size in bytes with a binary unit.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	f, suffix := float64(n), ""
	for _, s := range []string{"KiB", "MiB", "GiB", "TiB"} {
		f /= unit
		suffix = s
		if f < unit {
			break
		}
	}
	return fmt.Sprintf("%.1f %s", f, suffix)
}

// formatDuration writes a run's duration: milliseconds below a second, whole seconds above.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return d.Round(time.Second).String()
}
