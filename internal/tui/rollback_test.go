package tui

import (
	"slices"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const updateRun = "20261030T140200Z-0000000c"

// returnBackend is the settings backend where the update run of demo-stack made a backup.
func returnBackend() *fakeBackend {
	b := settingsBackend()
	for i := range b.runs.Runs {
		if b.runs.Runs[i].ID == updateRun {
			b.runs.Runs[i].Backup = olderBackup
		}
	}
	return b
}

// noBackupBackend is the same without the backup.
func noBackupBackend() *fakeBackend {
	b := returnBackend()
	for i := range b.runs.Runs {
		b.runs.Runs[i].Backup = ""
	}
	return b
}

// The ways to the dialog: the Runs list, second row, and the detail of the first stack.
var (
	rollbackFromRuns   = keys("3", "down", "B")
	rollbackFromDetail = keys("enter", "B")
)

func TestRollbackIsOfferedOnlyOnSuccessfulUpdateRuns(t *testing.T) {
	m := newSettingsModel(t, returnBackend())
	// The first run is a manual one, the third a rolled-back poll.
	send(m, "3", "B")
	if m.modal != nil {
		t.Fatal("offered on a run that is no update")
	}
	contains(t, m, "Only a successful update run")
	send(m, "down", "down", "B")
	if m.modal != nil {
		t.Fatal("offered on a run that did not succeed")
	}
	send(m, "up", "B")
	contains(t, m, "Back to the state before", updateRun)
	// The footer names the key on the update run only.
	send(m, "n")
	contains(t, m, "back to before update")
}

func TestRollbackFromDetailTakesTheNewestUpdateRun(t *testing.T) {
	m := newSettingsModel(t, returnBackend())
	send(m, rollbackFromDetail...)
	contains(t, m, "Back to the state before", "demo-stack", updateRun)
	send(m, "n", "esc", "down", "enter", "B")
	if m.modal != nil {
		t.Fatal("offered on a stack without a successful update run")
	}
	contains(t, m, "no successful update run")
}

func TestRollbackDialogNamesBothConsequences(t *testing.T) {
	m := newSettingsModel(t, returnBackend())
	send(m, rollbackFromRuns...)
	contains(t, m, "With data: Everything written since this update is lost.",
		"Version only: The data stays", "with data", "version only")
	de := newTestModel(t, i18n.German, returnBackend(), wideWidth, 40)
	send(de, rollbackFromRuns...)
	contains(t, de, "Mit Daten: Alles seit diesem Update",
		"Nur Version: Die Daten bleiben", "mit Daten", "nur Version")
}

func TestRollbackWithoutBackupOffersOnlyTheVersion(t *testing.T) {
	b := noBackupBackend()
	m := newSettingsModel(t, b)
	send(m, rollbackFromRuns...)
	contains(t, m, "made no backup", "version only")
	if shows(m.render(), "with data") {
		t.Fatalf("data is offered without a backup:\n%s", m.render())
	}
	send(m, "d")
	if len(b.returns) != 0 {
		t.Fatalf("the data answer worked without a backup: %v", b.returns)
	}
	send(m, "v")
	contains(t, m, "The stack starts on the old versions")
}

func TestRollbackCancelChangesNothing(t *testing.T) {
	b := returnBackend()
	m := newSettingsModel(t, b)
	send(m, rollbackFromRuns...)
	send(m, "enter")
	if m.modal != nil || len(b.returns) != 0 {
		t.Fatalf("modal %v, returns %v after the default answer", m.modal, b.returns)
	}
	send(m, "B", "n")
	if m.modal != nil || len(b.returns) != 0 {
		t.Fatal("cancel changed something")
	}
	// Cancelling the confirmation after a choice changes nothing either.
	send(m, "B", "d", "n")
	if m.modal != nil || len(b.returns) != 0 {
		t.Fatal("cancel of the confirmation changed something")
	}
}

func TestRollbackRunsTheChoiceAndShowsThePinnedStack(t *testing.T) {
	for _, tc := range []struct{ answer, data, what string }{
		{"d", control.ReturnRestoreData, "The stack is stopped"},
		{"v", control.ReturnKeepData, "and keeps its data"},
	} {
		b := returnBackend()
		m := newSettingsModel(t, b)
		send(m, rollbackFromRuns...)
		send(m, tc.answer)
		contains(t, m, "Return demo-stack to the versions before", tc.what)
		if len(b.returns) != 0 {
			t.Fatal("run before the confirmation")
		}
		send(m, "y")
		if want := []control.ReturnArgs{{Stack: "demo-stack", Run: updateRun, Data: tc.data}}; !slices.Equal(b.returns, want) {
			t.Fatalf("returns %v, want %v", b.returns, want)
		}
		contains(t, m, "is back on the versions", "is pinned on these versions", "APP_VERSION 2.0.0 -> 1.9.0", "Release it with unpin")
		send(m, "enter", "1")
		contains(t, m, "pinned")
	}
}

func TestRollbackFromDetailRuns(t *testing.T) {
	b := returnBackend()
	m := newSettingsModel(t, b)
	send(m, rollbackFromDetail...)
	send(m, "v", "y")
	if want := []control.ReturnArgs{{Stack: "demo-stack", Run: updateRun, Data: control.ReturnKeepData}}; !slices.Equal(b.returns, want) {
		t.Fatalf("returns %v", b.returns)
	}
}

func TestRollbackFailureNamesTheRun(t *testing.T) {
	b := returnBackend()
	b.returnRes = &control.ReturnRunResult{ReturnCheckResult: control.ReturnCheckResult{Stack: "demo-stack"}, ReturnRun: "20261101T040000Z-00000002", Result: "rolled-back"}
	m := newSettingsModel(t, b)
	send(m, rollbackFromRuns...)
	send(m, "v", "y")
	contains(t, m, "did not succeed")
}

func TestRollbackWithoutRootRunsTheCommand(t *testing.T) {
	for _, tc := range []struct{ answer, flag string }{{"d", "--restore-data"}, {"v", "--keep-data"}} {
		m, f := sudoDetailModel(t, i18n.English, returnBackend(), wideWidth, 40)
		f.report = `{"stack":"demo-stack","update_run":"` + updateRun + `","data":"keep","versions":"APP_VERSION 2.0.0 -> 1.9.0","run":"20261101T040000Z-00000001","commit":"0123456","result":"succeeded","pinned":true}`
		send(m, rollbackFromRuns...)
		send(m, tc.answer)
		args := []string{"stack", "rollback", tc.flag, "--yes", "--output", "json", "--", "demo-stack", updateRun}
		contains(t, m, "sudo qhost stack rollback "+tc.flag)
		if len(f.reqs) != 0 {
			t.Fatalf("sudo ran before the answer: %+v", f.reqs)
		}
		send(m, "y")
		if len(f.reqs) == 0 || !slices.Equal(f.reqs[len(f.reqs)-1].Args, args) {
			t.Fatalf("requests %+v, want the arguments %v", f.reqs, args)
		}
		contains(t, m, "is back on the versions", "is pinned on these versions", "APP_VERSION 2.0.0 -> 1.9.0")
	}
}

func TestGoldenRollbackDialog(t *testing.T) {
	goldenScreens(t, "rollback.dialog", detailEditRows, returnBackend, rollbackFromRuns...)
}

func TestGoldenRollbackDialogNoBackup(t *testing.T) {
	goldenScreens(t, "rollback.nobackup", detailEditRows, noBackupBackend, rollbackFromDetail...)
}

func TestGoldenRollbackConfirm(t *testing.T) {
	goldenScreens(t, "rollback.confirm", detailEditRows, returnBackend, seq(rollbackFromRuns, keys("d"))...)
}

func TestGoldenRollbackDone(t *testing.T) {
	goldenScreens(t, "rollback.done", detailEditRows, returnBackend, seq(rollbackFromRuns, keys("v", "y"))...)
}

func TestGoldenRollbackPinnedStack(t *testing.T) {
	goldenScreens(t, "rollback.pinned", detailEditRows, returnBackend, seq(rollbackFromRuns, keys("v", "y", "enter", "1"))...)
}

func TestRollbackAsRootShowsTheVersionsOfTheCheck(t *testing.T) {
	b := returnBackend()
	m := newSettingsModel(t, b)
	send(m, rollbackFromRuns...)
	contains(t, m, "APP_VERSION: 2.0.0 -> 1.9.0", "DB_VERSION: 16.2 -> 16.1", "with data")
	// The check asks with keep, so a missing backup does not hide the versions.
	if want := []control.ReturnArgs{{Stack: "demo-stack", Run: updateRun, Data: control.ReturnKeepData}}; !slices.Equal(b.checks, want) {
		t.Fatalf("checks %v", b.checks)
	}
	if len(b.returns) != 0 {
		t.Fatal("the check changed something")
	}
	nb := noBackupBackend()
	m = newSettingsModel(t, nb)
	send(m, rollbackFromRuns...)
	contains(t, m, "APP_VERSION: 2.0.0 -> 1.9.0", "made no backup")
}

func TestRollbackRefusedByTheCheckRunsNothing(t *testing.T) {
	b := returnBackend()
	b.errs["returncheck:demo-stack/"+updateRun] = control.NewError(control.CodeNotUpdatable, "the stack already has the versions from before that update")
	m := newSettingsModel(t, b)
	send(m, rollbackFromRuns...)
	if m.modal != nil {
		t.Fatal("the dialog opened for a refused return")
	}
	contains(t, m, "already has the versions from before")
	if len(b.returns) != 0 {
		t.Fatalf("returns %v", b.returns)
	}
}

func TestRollbackWithoutRootShowsTheVersionsOfTheCheck(t *testing.T) {
	for lang, want := range map[i18n.Lang]string{i18n.English: "APP_VERSION: 2.0.0 -> 1.9.0", i18n.German: "APP_VERSION: 2.0.0 -> 1.9.0"} {
		b := returnBackend()
		m, _ := sudoDetailModel(t, lang, b, wideWidth, 40)
		send(m, rollbackFromRuns...)
		contains(t, m, want)
		if len(b.checks) != 1 {
			t.Fatalf("checks %v without root", b.checks)
		}
	}
}

func TestRollbackRefusedByTheCheckWithoutRootOpensNoDialog(t *testing.T) {
	b := returnBackend()
	b.errs["returncheck:demo-stack/"+updateRun] = control.NewError(control.CodeBusy, "an update is running")
	m, _ := sudoDetailModel(t, i18n.English, b, wideWidth, 40)
	send(m, rollbackFromRuns...)
	if m.modal != nil {
		t.Fatal("the dialog opened for a refused check")
	}
	contains(t, m, "an update is running")
}

func TestGoldenRollbackDialogWithoutRoot(t *testing.T) {
	for name, lang := range goldenLangs {
		m, _ := sudoDetailModel(t, lang, returnBackend(), wideWidth, detailEditRows)
		send(m, rollbackFromRuns...)
		screen := m.render()
		checkScreen(t, screen, wideWidth, detailEditRows)
		checkGolden(t, "rollback.noroot."+name+".wide.golden", screen+"\n")
	}
}
