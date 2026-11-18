package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/diagnose"
)

// saveDiagnoseTo makes the saves of the package a fixed path and records the packages.
func saveDiagnoseTo(t *testing.T, err error) *[]*diagnose.Package {
	t.Helper()
	var got []*diagnose.Package
	old := saveDiagnose
	saveDiagnose = func(p *diagnose.Package) (string, error) {
		got = append(got, p)
		if err != nil {
			return "", err
		}
		return "/home/demo/qhost/" + p.FileName(), nil
	}
	t.Cleanup(func() { saveDiagnose = old })
	return &got
}

func TestDiagnoseInTheServiceLogNamesThePath(t *testing.T) {
	got := saveDiagnoseTo(t, nil)
	b := svcBackend()
	m := openServiceLog(t, b)
	send(m, "D")
	if len(*got) != 1 || b.diagCalls != 1 || m.diagnosing {
		t.Fatalf("saved %d, built %d, diagnosing %v", len(*got), b.diagCalls, m.diagnosing)
	}
	contains(t, m, "Diagnostic package saved to /home/demo/qhost/qhost-diagnose-demo-")
	contains(t, m, "check it before you pass it on")
}

func TestDiagnoseNamesTheIncompleteParts(t *testing.T) {
	saveDiagnoseTo(t, nil)
	b := svcBackend()
	b.diagCall = func(_ context.Context, _ string, _, _ any) error { return errors.New("service is down") }
	m := openServiceLog(t, b)
	send(m, "D")
	if !strings.Contains(m.render(), "parts have a problem") {
		t.Fatalf("screen %s", m.render())
	}
}

func TestDiagnoseShowsTheWriteError(t *testing.T) {
	saveDiagnoseTo(t, errors.New("disk is full"))
	m := openServiceLog(t, svcBackend())
	send(m, "D")
	contains(t, m, "The diagnostic package was not saved: disk is full")
}

func TestDiagnoseWorksOnlyInTheServiceLog(t *testing.T) {
	got := saveDiagnoseTo(t, nil)
	b := clogBackend()
	m := newTestModel(t, i18n.English, b, wideWidth, goldenRows)
	send(m, "D")                                                   // runs list
	send(m, "5", "down", "down", "down", "down", "down", "o", "D") // container log
	send(m, "esc", "3", "down", "enter", "D")                      // run log
	if len(*got) != 0 || b.diagCalls != 0 {
		t.Fatalf("saved %d, built %d", len(*got), b.diagCalls)
	}
}

func TestDiagnoseIgnoresASecondPressWhileItRuns(t *testing.T) {
	saveDiagnoseTo(t, nil)
	b := svcBackend()
	b.diagGate = make(chan struct{})
	m := openServiceLog(t, b)
	_, cmd := m.Update(keyMsg("D"))
	if cmd == nil || !m.diagnosing {
		t.Fatal("the first press did not start the collection")
	}
	if _, again := m.Update(keyMsg("D")); again != nil {
		t.Fatal("the second press started another collection")
	}
	contains(t, m, "still running")
	close(b.diagGate)
	drain(m, cmd)
	if m.diagnosing || b.diagCalls != 1 {
		t.Fatalf("diagnosing %v, built %d", m.diagnosing, b.diagCalls)
	}
}

func TestGoldenDiagnoseSaved(t *testing.T) {
	saveDiagnoseTo(t, nil)
	for name, lang := range goldenLangs {
		for size, width := range map[string]int{"narrow": narrowWidth, "wide": wideWidth} {
			t.Run(name+"."+size, func(t *testing.T) {
				m := newTestModel(t, lang, svcBackend(), width, goldenRows)
				m.now = func() time.Time { return clogStart.Add(time.Hour) }
				send(m, "3", "v", "D")
				screen := m.render()
				checkScreen(t, screen, width, goldenRows)
				checkGolden(t, "diagnosesaved."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}
