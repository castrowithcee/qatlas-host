package tui

import (
	"context"
	"slices"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func TestWithEngineFlag(t *testing.T) {
	for _, tt := range []struct {
		name   string
		args   []string
		engine string
		want   []string
	}{
		{"no engine", []string{"stack", "deploy", "--output", "json", "--", "demo"}, "", []string{"stack", "deploy", "--output", "json", "--", "demo"}},
		{"before the separator", []string{"stack", "deploy", "--output", "json", "--", "demo"}, "rootless:alice",
			[]string{"stack", "deploy", "--output", "json", "--engine", "rootless:alice", "--", "demo"}},
		{"without separator", []string{"stack", "updates", "--refresh"}, "system", []string{"stack", "updates", "--refresh", "--engine", "system"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			orig := slices.Clone(tt.args)
			if got := withEngineFlag(tt.args, tt.engine); !slices.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			if !slices.Equal(tt.args, orig) {
				t.Fatal("the arguments were changed in place")
			}
		})
	}
}

func TestEngineContext(t *testing.T) {
	if EngineOf(context.Background()) != "" {
		t.Fatal("an engine without WithEngine")
	}
	if got := EngineOf(WithEngine(context.Background(), "rootless:alice")); got != "rootless:alice" {
		t.Fatalf("got %q", got)
	}
}

// twoEngineBackend marks the stacks of demoBackend as of two engines, as the routing backend delivers them.
func twoEngineBackend() *fakeBackend {
	b := demoBackend()
	for i := range b.stacks.Stacks {
		b.stacks.Stacks[i].Engine = control.EngineSystem
		if b.stacks.Stacks[i].Name == "demo-stack" {
			b.stacks.Stacks[i].Engine = "rootless:alice"
		}
	}
	return b
}

// With stacks of several engines, the command handed to sudo names the engine of the stack; a host with one engine
// runs the commands unchanged (see TestStackActionWithoutRootRunsTheCommandThroughSudo).
func TestStackActionThroughSudoNamesTheEngineWhenSeveralAreShown(t *testing.T) {
	b := twoEngineBackend()
	m, f := sudoStackModel(t, i18n.English, b, wideWidth, 60)
	selectStack(t, m, "demo-stack")
	send(m, "a")
	want := []string{"stack", "deploy", "--output", "json", "--engine", "rootless:alice", "--", "demo-stack"}
	if !shows(m.render(), "sudo qhost "+"stack deploy --output json --engine rootless:alice -- demo-stack") {
		t.Fatalf("the question lacks the engine:\n%s", m.render())
	}
	send(m, "y")
	if got := commandsOf(f); len(got) != 1 || !slices.Equal(got[0], want) {
		t.Fatalf("commands %v, want %v", got, want)
	}

	m, f = sudoStackModel(t, i18n.English, twoEngineBackend(), wideWidth, 60)
	selectStack(t, m, "mail-relay")
	send(m, "p", "y")
	got := commandsOf(f)
	if len(got) != 1 || !slices.Equal(got[0], []string{"stack", "pause", "--output", "json", "--engine", control.EngineSystem, "--", "mail-relay"}) {
		t.Fatalf("commands %v", got)
	}
}
