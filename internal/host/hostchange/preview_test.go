package hostchange

import (
	"strings"
	"testing"
)

func TestPreviewPlansSecretActionsWithPlaceholdersOnly(t *testing.T) {
	f := newUserFixture(t, 1000)
	reg := f.runner.Registry
	for _, c := range []struct {
		id   string
		args []string
	}{
		{ActionUserCreate, []string{"demo-new"}},
		{ActionUserSetPassword, []string{"demo-new"}},
		{ActionUserAddKey, []string{"demo-new"}},
	} {
		plan, err := reg.Preview(c.id, c.args)
		if err != nil {
			t.Errorf("%s: %v", c.id, err)
			continue
		}
		shown := plan.Sentence
		for _, cmd := range plan.Commands {
			shown += " " + cmd.Display()
		}
		if strings.Contains(shown, DefaultPreviewSecret) || strings.Contains(shown, "SHA256:") {
			t.Errorf("%s: the placeholder or its fingerprint is shown: %q", c.id, shown)
		}
	}
	if _, err := reg.Preview("demo.unknown", nil); err == nil {
		t.Error("an unknown action must fail")
	}
}

func TestPreviewUndoChangesNothing(t *testing.T) {
	f := newUserFixture(t, 1000)
	res := f.apply(t, ActionUserCreate, createInput(t, "demo-pass-1"), "demo-new")
	before, _ := f.journal.List()
	plan, err := f.runner.PreviewUndo(res.Entry.ID)
	if err != nil || plan.Sentence == "" || len(plan.Commands) == 0 {
		t.Fatalf("plan %+v, err %v", plan, err)
	}
	after, _ := f.journal.List()
	if len(before) != len(after) || after[0].UndoneBy != "" {
		t.Errorf("the journal changed: %d -> %d entries", len(before), len(after))
	}
	if _, err := f.runner.PreviewUndo("20300102T030405.000000Z-aaaaaa"); err == nil {
		t.Error("an unknown entry must fail")
	}
}
