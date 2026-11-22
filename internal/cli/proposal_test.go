package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func proposalInfo(id, state string) control.ProposalInfo {
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	return control.ProposalInfo{ID: id, Kind: control.ProposalKindSourceAdd, State: state, Fingerprint: "sha256:abc",
		SubmitterUID: 1000, SubmitterName: "demo-admin", Submitted: at, Expires: at.Add(control.ProposalLifetime)}
}

func withStdin(t *testing.T, r io.Reader) {
	t.Helper()
	old := stdinStream
	stdinStream = func() io.Reader { return r }
	t.Cleanup(func() { stdinStream = old })
}

func TestProposalSubmitFromStdin(t *testing.T) {
	useFolderHost(t)
	var got control.ProposalSubmitArgs
	svc := startFakeService(t, fakeHandlers{control.ProposalSubmitCommand: func(raw json.RawMessage) any {
		_ = json.Unmarshal(raw, &got)
		return proposalInfo("p-0123456789abcdef", control.ProposalOpen)
	}})
	withStdin(t, strings.NewReader(`{"path":"/srv/stacks","url":"ssh://git.example.test/stacks.git"}`))
	code, stdout, stderr := run(t, "proposal", "submit", "--kind", "source-add", "--output", "compact")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "p-0123456789abcdef") || svc.count(control.ProposalSubmitCommand) != 1 {
		t.Fatalf("stdout %q", stdout)
	}
	if got.Kind != "source-add" || !strings.Contains(string(got.Content), `"/srv/stacks"`) {
		t.Fatalf("sent %+v", got)
	}
}

func TestProposalSubmitFromFileAndDash(t *testing.T) {
	useFolderHost(t)
	startFakeService(t, fakeHandlers{control.ProposalSubmitCommand: func(json.RawMessage) any {
		return proposalInfo("p-0123456789abcdef", control.ProposalOpen)
	}})
	file := filepath.Join(t.TempDir(), "p.json")
	if err := os.WriteFile(file, []byte(`{"path":"/srv/stacks"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := run(t, "proposal", "submit", "--kind", "source-add", "--file", file); code != 0 {
		t.Fatalf("file: code %d, stderr %q", code, stderr)
	}
	withStdin(t, strings.NewReader(`{"path":"/srv/stacks"}`))
	if code, _, stderr := run(t, "proposal", "submit", "--kind", "source-add", "--file", "-"); code != 0 {
		t.Fatalf("dash: code %d, stderr %q", code, stderr)
	}
}

func TestProposalSubmitRefusesBadInputLocally(t *testing.T) {
	useFolderHost(t)
	svc := startFakeService(t, fakeHandlers{control.ProposalSubmitCommand: func(json.RawMessage) any {
		return proposalInfo("p-0123456789abcdef", control.ProposalOpen)
	}})
	for name, args := range map[string][]string{
		"no kind":      {"proposal", "submit", "--file", "-"},
		"missing file": {"proposal", "submit", "--kind", "source-add", "--file", filepath.Join(t.TempDir(), "absent.json")},
	} {
		if code, _, _ := run(t, args...); code == 0 {
			t.Errorf("%s: succeeded", name)
		}
	}
	for name, in := range map[string]string{"empty": "", "not json": "{path"} {
		withStdin(t, strings.NewReader(in))
		if code, _, _ := run(t, "proposal", "submit", "--kind", "source-add"); code == 0 {
			t.Errorf("%s: succeeded", name)
		}
	}
	if n := svc.count(control.ProposalSubmitCommand); n != 0 {
		t.Fatalf("the service was asked %d times", n)
	}
}

func TestProposalSubmitReadsBoundedAndReportsOversize(t *testing.T) {
	useFolderHost(t)
	startFakeService(t, fakeHandlers{control.ProposalSubmitCommand: func(json.RawMessage) any { return &control.Error{Code: control.CodeTooLarge, Message: "x"} }})
	r := &countingReader{}
	withStdin(t, r)
	code, _, stderr := run(t, "proposal", "submit", "--kind", "source-add")
	if code == 0 || !strings.Contains(stderr, "at most 64 KiB") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if r.n != control.ProposalMaxBytes+1 {
		t.Fatalf("read %d bytes, want %d", r.n, control.ProposalMaxBytes+1)
	}
	// A valid oversize content reaches the transport, which refuses the frame.
	big := `{"path":"/` + strings.Repeat("a", control.ProposalMaxBytes) + `"}`
	withStdin(t, strings.NewReader(big))
	code, _, stderr = run(t, "proposal", "submit", "--kind", "source-add")
	if code == 0 || !strings.Contains(stderr, "at most 64 KiB") {
		t.Fatalf("valid oversize: code %d, stderr %q", code, stderr)
	}
}

type countingReader struct{ n int }

func (r *countingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	r.n += len(p)
	return len(p), nil
}

func TestProposalRefusalsOfTheService(t *testing.T) {
	useFolderHost(t)
	for name, tc := range map[string]struct {
		code string
		want string
	}{
		"limit":      {control.CodeProposalLimit, "qhost proposal withdraw"},
		"foreign":    {control.CodeNotSubmitter, "submitter"},
		"invalid":    {control.CodeInvalid, "Fix the argument"},
		"no access":  {control.CodeNoAccess, "reserved for administrators"},
		"unknown id": {control.CodeNotFound, "qhost proposal list"},
	} {
		t.Run(name, func(t *testing.T) {
			startFakeService(t, fakeHandlers{
				control.ProposalSubmitCommand:   func(json.RawMessage) any { return &control.Error{Code: tc.code, Message: "refused"} },
				control.ProposalWithdrawCommand: func(json.RawMessage) any { return &control.Error{Code: tc.code, Message: "refused"} },
			})
			withStdin(t, strings.NewReader(`{"path":"/srv/stacks"}`))
			code, _, stderr := run(t, "proposal", "submit", "--kind", "source-add")
			if code == 0 || !strings.Contains(stderr, tc.want) {
				t.Fatalf("submit: code %d, stderr %q", code, stderr)
			}
		})
	}
}

func TestProposalWithdrawAndList(t *testing.T) {
	useFolderHost(t)
	var sent control.ProposalIDArgs
	startFakeService(t, fakeHandlers{
		control.ProposalWithdrawCommand: func(raw json.RawMessage) any {
			_ = json.Unmarshal(raw, &sent)
			return proposalInfo(sent.ID, control.ProposalWithdrawn)
		},
		control.ProposalListCommand: func(json.RawMessage) any {
			return control.ProposalListResult{Proposals: []control.ProposalInfo{
				proposalInfo("p-aaaaaaaaaaaaaaaa", control.ProposalOpen), proposalInfo("p-bbbbbbbbbbbbbbbb", control.ProposalExpired)}}
		},
	})
	code, stdout, stderr := run(t, "proposal", "withdraw", "p-0123456789abcdef", "--output", "compact")
	if code != 0 || sent.ID != "p-0123456789abcdef" || !strings.Contains(stdout, "withdrawn") {
		t.Fatalf("code %d, sent %q, stdout %q, stderr %q", code, sent.ID, stdout, stderr)
	}
	code, stdout, stderr = run(t, "proposal", "list", "--output", "json")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(stdout), &rows); code != 0 || err != nil || len(rows) != 2 {
		t.Fatalf("code %d, stdout %q, stderr %q: %v", code, stdout, stderr, err)
	}
	if rows[0]["id"] != "p-aaaaaaaaaaaaaaaa" || rows[1]["state"] != "expired" {
		t.Fatalf("rows %v", rows)
	}
}

func TestProposalListEmptyIsJSON(t *testing.T) {
	useFolderHost(t)
	startFakeService(t, fakeHandlers{control.ProposalListCommand: func(json.RawMessage) any { return control.ProposalListResult{} }})
	code, stdout, _ := run(t, "proposal", "list", "--output", "json")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(stdout), &rows); code != 0 || err != nil {
		t.Fatalf("code %d, stdout %q: %v", code, stdout, err)
	}
}

const hostile = "evil\x1b]0;title\x07\x1b[2Jname\u202e\r\n"

func hostileShow(json.RawMessage) any {
	info := proposalInfo("p-0123456789abcdef", control.ProposalOpen)
	info.SubmitterName = hostile
	content, _ := json.Marshal(control.ProposalSourceAdd{Path: "/srv/" + hostile, URL: "ssh://git.example.test/" + hostile})
	return control.ProposalShowResult{ProposalInfo: info, Content: content}
}

func TestProposalTextOutputReplacesControlCharacters(t *testing.T) {
	useFolderHost(t)
	startFakeService(t, fakeHandlers{
		control.ProposalShowCommand: hostileShow,
		control.ProposalListCommand: func(json.RawMessage) any {
			info := proposalInfo("p-0123456789abcdef", control.ProposalOpen)
			info.SubmitterName = hostile
			return control.ProposalListResult{Proposals: []control.ProposalInfo{info}}
		},
	})
	for _, format := range []string{"table", "compact"} {
		for _, args := range [][]string{{"proposal", "show", "p-0123456789abcdef"}, {"proposal", "list"}} {
			code, stdout, stderr := run(t, append(args, "--output", format)...)
			if code != 0 {
				t.Fatalf("%v: code %d, stderr %q", args, code, stderr)
			}
			for _, r := range stdout {
				if r < 0x20 && r != '\n' && r != '\t' || r == 0x7f || r == 0x202e {
					t.Fatalf("%v %s: output holds %U:\n%q", args, format, r, stdout)
				}
			}
			if !strings.Contains(stdout, "evil") || !strings.Contains(stdout, "\uFFFD") {
				t.Fatalf("%v %s: replacement missing:\n%q", args, format, stdout)
			}
		}
	}
}

func TestProposalShowJSONKeepsContent(t *testing.T) {
	useFolderHost(t)
	startFakeService(t, fakeHandlers{control.ProposalShowCommand: hostileShow})
	code, stdout, stderr := run(t, "proposal", "show", "p-0123456789abcdef", "--output", "json")
	var got struct {
		ID      string `json:"id"`
		Content struct {
			Path string `json:"path"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); code != 0 || err != nil {
		t.Fatalf("code %d, stdout %q, stderr %q: %v", code, stdout, stderr, err)
	}
	if got.ID != "p-0123456789abcdef" || got.Content.Path != "/srv/"+hostile {
		t.Fatalf("got %+v", got)
	}
}

func TestPlainText(t *testing.T) {
	if got := plainText("a\x00b\u2028c\xffd\u00e4"); got != "a\uFFFDb\uFFFDc\uFFFDd\u00e4" {
		t.Fatalf("plainText = %q", got)
	}
}
