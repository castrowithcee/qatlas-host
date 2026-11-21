package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// The terminal interface without root runs these commands the way the tests here do: no terminal, the value
// only on standard input, the answer as JSON, and the name after "--". The fields are the ones it reads.
func TestSetCommandsTakeTheValueFromStdinAndPrintJSON(t *testing.T) {
	var log syncBuffer
	e := startSecretService(t, &log)
	keyDir := t.TempDir()
	pem := testKeyPEM(t, "ed25519", "")
	hook := newReceiver(t, 204)
	url := hook.URL + "/hook/demo-path-secret"
	for _, tt := range []struct {
		name, stdin string
		args        []string
		want        map[string]any
	}{
		{"secret", fakeSecretValue, []string{"stack", "secret", "set", "--output", "json", "--", "plain-stack", "DB_PASSWORD"},
			map[string]any{"stack": "plain-stack", "key": "DB_PASSWORD", "stored": true}},
		{"credential", "alice:" + demoToken, []string{"credential", "set", "--output", "json", "--", "registry.example"},
			map[string]any{"name": "registry.example", "stored": true}},
		{"notify", url + "\n" + demoHookKey, []string{"notify", "add", "--type", "webhook", "--events", "succeeded,failed", "--output", "json", "--", "ops-hook"},
			map[string]any{"name": "ops-hook", "type": "webhook", "events": "succeeded,failed"}},
		{"key", pem, []string{"key", "set", "--dir", keyDir, "--output", "json"}, map[string]any{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runWithStdin(t, tt.stdin, tt.args...)
			if code != 0 || stderr != "" {
				t.Fatalf("code %d, stderr %q", code, stderr)
			}
			var got map[string]any
			if err := json.Unmarshal([]byte(stdout), &got); err != nil {
				t.Fatalf("stdout %q is no JSON object: %v", stdout, err)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("%s = %v, want %v in %s", k, got[k], v, stdout)
				}
			}
			if tt.name == "key" && (got["public_key"] == "" || got["fingerprint"] == "") {
				t.Errorf("no public key or fingerprint in %s", stdout)
			}
			for _, secret := range []string{fakeSecretValue, demoToken, demoHookKey, "demo-path-secret"} {
				if strings.Contains(stdout+stderr+log.String(), secret) {
					t.Errorf("%q appears in output or log", secret)
				}
			}
		})
	}
	_ = e
}
