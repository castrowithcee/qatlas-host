package engine

import (
	"testing"
)

func TestFailedUpdateLocksExactlyItsVariableAndVersion(t *testing.T) {
	r := updateRig(t)
	r.dep.fail("check", errBoom)
	res := r.update(step("1.0.0", "1.1.0"))
	if res[0].Result != ResultRolledBack {
		t.Fatalf("results = %+v", res)
	}
	if got := r.stack("web").FailedVersions; len(got) != 1 || got["APP_VERSION"] != "1.1.0" {
		t.Fatalf("failed versions = %v", got)
	}
}

func TestStepwiseFailureLocksOnlyTheFailedStep(t *testing.T) {
	r := updateRig(t)
	r.dep.fail("check", nil, errBoom)
	r.update(step("1.0.0", "1.2.0"), step("1.2.0", "2.3.0"), step("2.3.0", "3.1.0"))
	if got := r.stack("web").FailedVersions; len(got) != 1 || got["APP_VERSION"] != "2.3.0" {
		t.Fatalf("failed versions = %v", got)
	}
}

func TestRollbackInTheObservationTimeLocksTheObservedVersion(t *testing.T) {
	r := updateRig(t)
	r.update(step("1.0.0", "1.1.0"))
	if o := r.stack("web").Observe; o == nil || o.Variable != "APP_VERSION" || o.Version != "1.1.0" {
		t.Fatalf("observation = %+v", o)
	}
	if len(r.stack("web").FailedVersions) != 0 {
		t.Fatal("a successful update locked a version")
	}
	if _, err := r.rollbackUpdate(commit(2001)); err != nil {
		t.Fatal(err)
	}
	if got := r.stack("web").FailedVersions; len(got) != 1 || got["APP_VERSION"] != "1.1.0" {
		t.Fatalf("failed versions = %v", got)
	}
}

func TestSuccessfulUpdateLiftsTheLock(t *testing.T) {
	for _, to := range []string{"1.1.0", "1.2.0"} {
		r := updateRig(t)
		r.dep.fail("check", errBoom)
		r.update(step("1.0.0", "1.1.0"))
		r.update(step("1.0.0", to))
		if st := r.stack("web"); st.LastRun.Result != ResultSucceeded || len(st.FailedVersions) != 0 {
			t.Fatalf("to %s: %+v / %v", to, st.LastRun, st.FailedVersions)
		}
	}
}

func TestValidateFailedVersions(t *testing.T) {
	ok := State{Schema: stateSchema, Stacks: map[string]StackState{"web": {FailedVersions: map[string]string{"APP_VERSION": "1.1.0"}}}}
	if err := validateState(ok); err != nil {
		t.Fatal(err)
	}
	for _, m := range []map[string]string{{"": "1.1.0"}, {"APP_VERSION": ""}} {
		if validateState(State{Schema: stateSchema, Stacks: map[string]StackState{"web": {FailedVersions: m}}}) == nil {
			t.Fatalf("%v was accepted", m)
		}
	}
}
