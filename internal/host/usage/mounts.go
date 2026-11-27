package usage

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// pseudoTypes are file systems that hold no storage of their own or only a view of another one.
var pseudoTypes = map[string]bool{
	"proc": true, "sysfs": true, "tmpfs": true, "devtmpfs": true, "devpts": true, "cgroup": true, "cgroup2": true,
	"overlay": true, "squashfs": true, "securityfs": true, "pstore": true, "bpf": true, "debugfs": true,
	"tracefs": true, "configfs": true, "fusectl": true, "mqueue": true, "hugetlbfs": true, "autofs": true,
	"binfmt_misc": true, "nsfs": true, "ramfs": true, "efivarfs": true, "rpc_pipefs": true, "fuse.lxcfs": true,
	"selinuxfs": true, "devfs": true,
}

// filesystems lists the real file systems of the mount table, each device once at its first mount point.
func (s Source) filesystems() []Filesystem {
	table := s.Mounts
	if table == "" {
		table = filepath.Join(s.ProcRoot, "self", "mounts")
	}
	b, err := os.ReadFile(table)
	if err != nil || s.Statfs == nil {
		return nil
	}
	var out []Filesystem
	seen := map[string]bool{}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || pseudoTypes[f[2]] || seen[f[0]] {
			continue
		}
		mount := unescapeMount(f[1])
		st, err := s.Statfs(mount)
		if err != nil || st.Blocks == 0 {
			continue
		}
		seen[f[0]] = true
		out = append(out, Filesystem{
			Mount: mount, Device: unescapeMount(f[0]), Type: f[2],
			TotalBytes: st.Blocks * st.BlockSize,
			UsedBytes:  (st.Blocks - min(st.Free, st.Blocks)) * st.BlockSize,
			FreeBytes:  st.Available * st.BlockSize,
		})
	}
	return out
}

// unescapeMount decodes the octal escapes (\040 for a space) the kernel writes into mount tables.
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
