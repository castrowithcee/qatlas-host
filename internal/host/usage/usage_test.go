package usage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const (
	statA = "cpu  100 0 100 700 100 0 0 0 0 0\ncpu0 50 0 50 350 50 0 0 0 0 0\ncpu1 50 0 50 350 50 0 0 0 0 0\nintr 1\n"
	// 100 more jiffies of work and 100 more idle: half busy.
	statB = "cpu  150 0 150 750 150 0 0 0 0 0\ncpu0 75 0 75 375 75 0 0 0 0 0\ncpu1 75 0 75 375 75 0 0 0 0 0\n"

	meminfo = "MemTotal:        8000 kB\nMemFree:          100 kB\nMemAvailable:    2000 kB\nSwapTotal:       1000 kB\nSwapFree:         400 kB\n"
	mounts  = "sysfs /sys sysfs rw 0 0\nproc /proc proc rw 0 0\n/dev/vda1 / ext4 rw 0 0\n/dev/vda1 /bind ext4 rw 0 0\n" +
		"overlay /merged overlay rw 0 0\ntmpfs /run tmpfs rw 0 0\n/dev/vdb1 /mnt/my\\040data xfs rw 0 0\n/dev/gone /gone ext4 rw 0 0\n"
)

func fixture(t *testing.T, files map[string]string) Source {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return Source{ProcRoot: root, Statfs: func(path string) (FSStat, error) {
		switch path {
		case "/":
			return FSStat{BlockSize: 1024, Blocks: 1000, Free: 400, Available: 300}, nil
		case "/mnt/my data":
			return FSStat{BlockSize: 4096, Blocks: 10, Free: 10, Available: 10}, nil
		}
		return FSStat{}, errors.New("no such mount")
	}}
}

func TestReadFromExampleProc(t *testing.T) {
	s := fixture(t, map[string]string{"stat": statA, "loadavg": "0.50 1.25 2.00 1/100 42\n", "uptime": "3600.75 7000.00\n",
		"meminfo": meminfo, "self/mounts": mounts})
	s.Pause = func(context.Context) error {
		return os.WriteFile(filepath.Join(s.ProcRoot, "stat"), []byte(statB), 0o644)
	}
	u, err := s.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.CPUPercent == nil || *u.CPUPercent != 50 || u.Cores != 2 {
		t.Fatalf("cpu = %v, cores %d", u.CPUPercent, u.Cores)
	}
	if *u.Load != (Load{0.5, 1.25, 2}) || *u.UptimeSeconds != 3600 {
		t.Fatalf("load %+v, uptime %d", u.Load, *u.UptimeSeconds)
	}
	if *u.Memory != (Memory{TotalBytes: 8000 * 1024, UsedBytes: 6000 * 1024, AvailableBytes: 2000 * 1024}) ||
		*u.Swap != (Memory{TotalBytes: 1000 * 1024, UsedBytes: 600 * 1024, AvailableBytes: 400 * 1024}) {
		t.Fatalf("memory %+v, swap %+v", u.Memory, u.Swap)
	}
	want := []Filesystem{
		{Mount: "/", Device: "/dev/vda1", Type: "ext4", TotalBytes: 1000 * 1024, UsedBytes: 600 * 1024, FreeBytes: 300 * 1024},
		{Mount: "/mnt/my data", Device: "/dev/vdb1", Type: "xfs", TotalBytes: 10 * 4096, UsedBytes: 0, FreeBytes: 10 * 4096},
	}
	if len(u.Filesystems) != len(want) || u.Filesystems[0] != want[0] || u.Filesystems[1] != want[1] {
		t.Fatalf("filesystems = %+v", u.Filesystems)
	}
}

func TestMissingSourcesGiveEmptyValues(t *testing.T) {
	s := fixture(t, map[string]string{"meminfo": "MemTotal: 100 kB\nMemFree: 40 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n"})
	s.Pause = func(context.Context) error { t.Fatal("paused without a CPU sample"); return nil }
	u, err := s.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.CPUPercent != nil || u.Cores != 0 || u.Load != nil || u.UptimeSeconds != nil || u.Swap != nil || len(u.Filesystems) != 0 {
		t.Fatalf("not empty: %+v", u)
	}
	if u.Memory == nil || u.Memory.AvailableBytes != 40*1024 {
		t.Fatalf("memory without MemAvailable = %+v", u.Memory)
	}
}

func TestReadEndsWithTheContext(t *testing.T) {
	s := fixture(t, map[string]string{"stat": statA})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.Pause = func(ctx context.Context) error { return ctx.Err() }
	if _, err := s.Read(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestIdleCPUWithoutElapsedTimeIsEmpty(t *testing.T) {
	s := fixture(t, map[string]string{"stat": statA})
	s.Pause = func(context.Context) error { return nil }
	if u, _ := s.Read(context.Background()); u.CPUPercent != nil {
		t.Fatalf("cpu = %v", *u.CPUPercent)
	}
}

func TestStatfsReadsARealFolder(t *testing.T) {
	st, err := NewSource().Statfs(t.TempDir())
	if err != nil || st.BlockSize == 0 || st.Blocks == 0 {
		t.Fatalf("%+v, %v", st, err)
	}
}
