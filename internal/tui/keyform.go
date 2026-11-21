package tui

import (
	"errors"
	"io"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const (
	// formKeyChoice asks for the stack folder and whether to store or generate; formKeyStore asks for the key.
	formKeyChoice formKind = formDataDirs + 1
	formKeyStore  formKind = formKeyChoice + 1
	formGeneral   formKind = formKeyStore + 1
)

// maxKeyBytes is the largest private key the service takes. The terminal interface may not import the
// credential store, so the limit stands here as with the credential values.
const maxKeyBytes = 16 << 10

// The values of the mode choice.
const (
	keyModeStore    = "store"
	keyModeGenerate = "generate"
)

// keyExists is the answer of a write that found a key in the stack folder and sent nothing: replace sends it
// again with the replacement the person confirmed.
type keyExists struct {
	details []string
	replace func() tea.Cmd
}

// keyGenerateAsk is the answer of reading the key of a folder before generating one without root: exists says
// whether the command has to replace a key.
type keyGenerateAsk struct {
	dir     string
	details []string
	exists  bool
}

// askKeyGenerate asks for the generation of a key through sudo; an existing key is replaced only with --replace,
// which the question names.
func (m *Model) askKeyGenerate(k keyGenerateAsk) {
	t := m.text
	args := []string{"key", "generate", "--dir", k.dir}
	ctx := m.engineCtx(m.folderEngineOf(k.dir))
	c := settingChange{title: textTitleKey, question: t.Lookup(textAskKeyGenerate, k.dir), consequence: t.Lookup(textWhatKeyGenerate),
		details: k.details, op: opKeyGenerate, name: k.dir,
		direct: func(b Backend) (any, error) { return b.KeyGenerate(ctx, k.dir, k.exists) },
		decode: decodeSudo[control.KeyResult](func(r control.KeyResult) bool { return r.Fingerprint != "" })}
	if k.exists {
		args = append(args, "--replace")
		c.question, c.consequence, c.danger = t.Lookup(textAskKeyReplace), t.Lookup(textWhatKeyReplace), true
	}
	c.args = m.engineArgs(m.folderEngineOf(k.dir), append(args, "--output", "json"))
	m.askSetting(c)
}

// keySetAsk is the answer of reading the key of a folder before storing one without root. The private key stays
// in the closure of the question that follows; it is never a field that is shown.
type keySetAsk struct {
	keyGenerateAsk
	key string
}

// askKeySet asks for the storing of a key through sudo, the key on standard input; an existing key is replaced
// only with --replace, which the question names.
func (m *Model) askKeySet(k keySetAsk) {
	t := m.text
	args := []string{"key", "set", "--dir", k.dir}
	ctx := m.engineCtx(m.folderEngineOf(k.dir))
	c := settingChange{title: textTitleKey, question: t.Lookup(textAskKeySet, k.dir), consequence: t.Lookup(textWhatKeySet),
		details: k.details, op: opKeySet, name: k.dir,
		direct: func(b Backend) (any, error) { return b.KeySet(ctx, k.dir, k.key, k.exists) },
		input:  secretInput(k.key),
		decode: decodeSudo[control.KeyResult](func(r control.KeyResult) bool { return r.Fingerprint != "" })}
	if k.exists {
		args = append(args, "--replace")
		c.question, c.consequence, c.danger = t.Lookup(textAskKeyReplace), t.Lookup(textWhatKeyReplace), true
	}
	c.args = m.engineArgs(m.folderEngineOf(k.dir), append(args, "--output", "json"))
	m.askSetting(c)
}

// openKeyForm asks how to set the key of the folder the key view shows.
func (m *Model) openKeyForm() tea.Cmd {
	fields := []kittui.FormField{{ID: "mode", Kind: kittui.FieldChoice, Label: textFieldKeyMode, Hint: textHintKeyMode,
		Choices: []kittui.Choice{choice(keyModeStore, textKeyModeStore), choice(keyModeGenerate, textKeyModeGenerate)}}}
	return m.openForm(m.newSettingsForm(formKeyChoice, textTitleKey, fields))
}

func (m *Model) submitKeyChoice(f *settingsForm, v map[kittui.FieldID]kittui.FormValue) tea.Cmd {
	dir := m.set.keyFor
	label := dir
	if v["mode"].Choice == keyModeGenerate {
		return m.keyWrite(f, dir, label, "", true)
	}
	next := m.newSettingsForm(formKeyStore, textTitleKeyStore, []kittui.FormField{
		{ID: "key", Kind: kittui.FieldMaskedBlock, Label: textFieldKeyBlock, Hint: textHintKeyBlock},
		textField("path", textFieldKeyPath, textHintKeyPath, ""),
	})
	if next != nil {
		next.stackDir, next.keyLabel = dir, label
	}
	return m.openForm(next)
}

func (m *Model) submitKeyStore(f *settingsForm, v map[kittui.FieldID]kittui.FormValue) tea.Cmd {
	pasted, path := v["key"].Text, strings.TrimSpace(v["path"].Text)
	key := pasted
	switch {
	case strings.TrimSpace(pasted) == "" && path == "":
		f.problems(m.bad("key", textProblemKeyNone))
		return nil
	case strings.TrimSpace(pasted) != "" && path != "":
		f.problems(m.bad("path", textProblemKeyBoth))
		return nil
	case path != "":
		text, cause := readKeyFile(path)
		if cause != "" {
			f.problems(m.bad("path", cause))
			return nil
		}
		key = text
	}
	if len(key) > maxKeyBytes {
		f.problems(m.bad(map[bool]string{true: "path", false: "key"}[path != ""], textProblemKeyTooBig))
		return nil
	}
	return m.keyWrite(f, f.stackDir, f.keyLabel, key, false)
}

// readKeyFile reads a key file for the form; cause is the text of the problem, never anything of the file.
func readKeyFile(path string) (text string, cause kittui.TextID) {
	file, err := os.Open(path)
	if err != nil {
		return "", textProblemKeyFile
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxKeyBytes+1))
	if err != nil {
		return "", textProblemKeyFile
	}
	if len(raw) > maxKeyBytes {
		return "", textProblemKeyTooBig
	}
	return string(raw), ""
}

// keyWrite stores the key or generates one. The key of the folder is asked for first, so that the question about
// replacing it comes before anything is sent; a key that appeared in between is refused by the service and
// shown at the form.
func (m *Model) keyWrite(f *settingsForm, dir, label, key string, generate bool) tea.Cmd {
	op := opKeySet
	if generate {
		op = opKeyGenerate
	}
	ctx := m.engineCtx(m.folderEngineOf(dir))
	send := func(b Backend, replace bool) (control.KeyResult, error) {
		if generate {
			return b.KeyGenerate(ctx, dir, replace)
		}
		return b.KeySet(ctx, dir, key, replace)
	}
	var details []string
	if label != "" {
		details = []string{m.text.Lookup(textDetailKeyFolder, label)}
	}
	return m.writeCall(op, dir, func(b Backend) (any, error) {
		_, err := b.KeyShow(ctx, dir)
		var remote *control.Error
		if m.euid() != 0 {
			// Without root the command asks, and says whether it replaces a key; reading the key needs no root.
			if err != nil && (!errors.As(err, &remote) || remote.Code != control.CodeNoSuchKey) {
				return nil, err
			}
			ask := keyGenerateAsk{dir: dir, details: details, exists: err == nil}
			if generate {
				return ask, nil
			}
			return keySetAsk{keyGenerateAsk: ask, key: key}, nil
		}
		switch {
		case err == nil:
			return keyExists{details: details, replace: func() tea.Cmd {
				return m.writeCall(op, dir, func(b Backend) (any, error) { return send(b, true) })
			}}, nil
		case !errors.As(err, &remote) || remote.Code != control.CodeNoSuchKey:
			return nil, err
		}
		return send(b, false)
	})
}

// keyProblem places a refused key at the field it came from: the pasted block, or the path when that was empty.
func (m *Model) keyProblem(f *settingsForm, msg settingMsg) (kittui.FormProblem, bool) {
	var remote *control.Error
	if f.kind != formKeyStore || msg.op != opKeySet || !errors.As(msg.err, &remote) || remote.Code != control.CodeInvalid {
		return kittui.FormProblem{}, false
	}
	field := "path"
	if strings.TrimSpace(f.form.Values()["key"].Text) != "" {
		field = "key"
	}
	return kittui.FormProblem{Field: kittui.FieldID(field), Cause: m.serviceText(msg.err), NextStep: m.text.Lookup(textFormFix)}, true
}
