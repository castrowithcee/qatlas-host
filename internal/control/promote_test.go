package control

import "testing"

func TestPromoteSourceIsProvenOnlyWhenAPromotionWouldTakeIt(t *testing.T) {
	proven := PromoteSource{Stack: "stage", Commit: "0123456789abcdef0123456789abcdef01234567", LastResult: "succeeded"}
	if why := proven.Unproven(); why != "" {
		t.Fatalf("a proven source is unproven: %s", why)
	}
	for want, mutate := range map[string]func(*PromoteSource){
		UnprovenNotDeployed:  func(p *PromoteSource) { p.Commit = "" },
		UnprovenRun:          func(p *PromoteSource) { p.LastResult = "failed" },
		UnprovenFailedCommit: func(p *PromoteSource) { p.FailedCommit = "89abcdef0123456789abcdef0123456789abcdef" },
		UnprovenNotHealthy:   func(p *PromoteSource) { p.Unhealthy = true },
	} {
		p := proven
		mutate(&p)
		if got := p.Unproven(); got != want {
			t.Errorf("Unproven() = %q, want %q", got, want)
		}
	}
}

func TestPromoteSourceVersionsAreMatchedByImage(t *testing.T) {
	src := PromoteSource{Versions: []PromoteVersion{{Variable: "A_TAG", Image: "example/a", Version: "2.0"}, {Variable: "B_TAG", Image: "example/b", Version: "3.0"}}}
	got := src.VersionsFor([]ImageInfo{{Var: "WEB_TAG", Image: "example/a", Version: "1.0"}, {Var: "DB_TAG", Image: "example/db", Version: "9"}})
	if len(got) != 1 || got[0] != (PromoteVersion{Variable: "WEB_TAG", Image: "example/a", Version: "2.0"}) {
		t.Fatalf("versions = %+v", got)
	}
}
