// Package idmap reads the ID mapping of a user namespace, the content of /proc/<pid>/uid_map and gid_map.
package idmap

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Range maps Count IDs starting at Inside in the namespace to the IDs starting at Outside in its parent.
type Range struct {
	Inside  uint64 `json:"inside"`
	Outside uint64 `json:"outside"`
	Count   uint64 `json:"count"`
}

// Map is a normalized mapping: ranges sorted by Inside, and consecutive ranges that continue each other on
// both sides merged into one. Two Maps describe the same mapping exactly when they are equal.
type Map []Range

// Default is the mapping of the initial namespace, which is what an engine without a user namespace uses.
func Default() Map { return Map{{Inside: 0, Outside: 0, Count: 4294967295}} }

// Parse reads the text of a map file: one range per line as three decimal numbers separated by white space.
// An empty text, a line that is not three numbers, a range of size zero and overlapping ranges are errors, so
// a mapping that cannot be read is never taken for a valid one.
func Parse(text string) (Map, error) {
	var in Map
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if len(f) != 3 {
			return nil, fmt.Errorf("id map line %q does not have three fields", line)
		}
		var n [3]uint64
		for i, s := range f {
			v, err := strconv.ParseUint(s, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("id map line %q: %w", line, err)
			}
			n[i] = v
		}
		if n[2] == 0 {
			return nil, fmt.Errorf("id map line %q has a range of size zero", line)
		}
		if n[0]+n[2] < n[0] || n[1]+n[2] < n[1] {
			return nil, fmt.Errorf("id map line %q overflows", line)
		}
		in = append(in, Range{Inside: n[0], Outside: n[1], Count: n[2]})
	}
	if len(in) == 0 {
		return nil, errors.New("id map is empty")
	}
	return Normalize(in)
}

// Normalize sorts the ranges and merges consecutive ones. Overlapping ranges are an error.
func Normalize(in Map) (Map, error) {
	r := append(Map(nil), in...)
	sort.Slice(r, func(i, j int) bool { return r[i].Inside < r[j].Inside })
	var out Map
	for _, x := range r {
		if n := len(out); n > 0 {
			last := &out[n-1]
			switch end := last.Inside + last.Count; {
			case x.Inside < end:
				return nil, errors.New("id map ranges overlap")
			case x.Inside == end && x.Outside == last.Outside+last.Count:
				last.Count += x.Count
				continue
			}
		}
		out = append(out, x)
	}
	return out, nil
}

// Equal reports whether a and b are the same normalized mapping. An empty map is never equal to anything.
func (a Map) Equal(b Map) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// String is the map as the kernel prints it, one range per line.
func (a Map) String() string {
	var b strings.Builder
	for _, r := range a {
		fmt.Fprintf(&b, "%d %d %d\n", r.Inside, r.Outside, r.Count)
	}
	return b.String()
}
