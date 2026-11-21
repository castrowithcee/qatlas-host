package registrylogin

import (
	"strings"
	"testing"
)

func TestHostAndName(t *testing.T) {
	for server, want := range map[string]string{
		"ghcr.io":                      "registry.ghcr.io",
		"https://ghcr.io":              "registry.ghcr.io",
		"GHCR.IO":                      "registry.ghcr.io",
		"https://index.docker.io/v1/":  "registry.docker.io",
		"registry-1.docker.io":         "registry.docker.io",
		"http://host.example:5000/v2/": "registry.host.example-5000",
	} {
		got, ok := Name(Host(server))
		if !ok || got != want {
			t.Errorf("Name(Host(%q)) = %q, %v; want %q", server, got, ok, want)
		}
	}
}

func TestParse(t *testing.T) {
	c, err := Parse("demo-user:demo-token:0000")
	if err != nil || c.User != "demo-user" || c.Token != "demo-token:0000" {
		t.Fatalf("Parse = %+v, %v", c, err)
	}
	for _, bad := range []string{"", "demo-user", ":demo-token", "demo-user:"} {
		if _, err := Parse(bad); err == nil || strings.Contains(err.Error(), "demo") {
			t.Errorf("Parse(%q) error = %v", bad, err)
		}
	}
}
