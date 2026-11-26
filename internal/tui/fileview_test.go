package tui

import "testing"

func TestFileViewPathsLieBelowTheRoot(t *testing.T) {
	for _, tc := range []struct {
		root  string
		parts []string
		want  string
	}{
		{"/", nil, "/"},
		{"/", []string{"etc", "hosts"}, "/etc/hosts"},
		{"/srv/demo-stack", nil, "/srv/demo-stack"},
		{"/srv/demo-stack", []string{"config", ".env"}, "/srv/demo-stack/config/.env"},
	} {
		v := &fileView{root: tc.root}
		if got := v.path(tc.parts...); got != tc.want {
			t.Errorf("path %v below %s is %s, want %s", tc.parts, tc.root, got, tc.want)
		}
	}
}
