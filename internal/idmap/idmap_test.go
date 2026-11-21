package idmap

import "testing"

func TestParseSingleRange(t *testing.T) {
	m, err := Parse("         0     100000      65536\n")
	if err != nil || len(m) != 1 || m[0] != (Range{0, 100000, 65536}) {
		t.Fatalf("map = %v, %v", m, err)
	}
	d, err := Parse("0 0 4294967295")
	if err != nil || !d.Equal(Default()) {
		t.Fatalf("default = %v, %v", d, err)
	}
}

func TestParseMergesConsecutiveRangesOnly(t *testing.T) {
	// The rootless layout: own ID 0 maps to the owner, 1.. to the sub range.
	m, err := Parse("0 1000 1\n1 100000 65536\n")
	if err != nil || len(m) != 2 {
		t.Fatalf("map = %v, %v", m, err)
	}
	// Ranges that continue each other on both sides are one range, whatever order they are in.
	a, err := Parse("65536 165536 100\n0 100000 65536\n")
	if err != nil || len(a) != 1 || a[0] != (Range{0, 100000, 65636}) {
		t.Fatalf("merged = %v, %v", a, err)
	}
	b, _ := Parse("0 100000 65636")
	if !a.Equal(b) {
		t.Fatal("equal mappings differ")
	}
	// Consecutive inside but not outside stays two ranges.
	c, err := Parse("0 100000 10\n10 300000 10\n")
	if err != nil || len(c) != 2 || c.Equal(b) {
		t.Fatalf("split = %v, %v", c, err)
	}
}

func TestParseRefusesWhatItCannotRead(t *testing.T) {
	for _, in := range []string{"", "\n", "0 0", "0 0 x", "0 0 0", "-1 0 1", "0 0 1 1", "0 0 10\n5 20 10", "18446744073709551615 0 2"} {
		if m, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) = %v, want error", in, m)
		}
	}
}

func TestEmptyMapEqualsNothing(t *testing.T) {
	if (Map{}).Equal(Map{}) || Default().Equal(nil) {
		t.Fatal("an empty map must never be equal")
	}
}
