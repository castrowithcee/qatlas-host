package tui

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/client"
)

// DeploySetup is the host side of setting up the deploy service. It acts on the host, not through a service
// channel, because the deploy service does not exist before it ran. Nothing runs without the person's answer.
// A user name stands for the instance of the rootless engine of that user; the empty name is the service of the
// system engine.
type DeploySetup interface {
	// Installed reports whether a deploy service is set up: the one of the system engine or the instance of a
	// rootless engine. A host with only instances is set up, and the setup of the service of the system engine
	// is still offered.
	Installed() bool
	// Users lists the users that have a rootless engine; it is a list to choose from, not a check.
	Users() []string
	// Plan lists the exact commands the setup still needs and what the person must read first; it changes
	// nothing. An error is a refusal with its reason.
	Plan(ctx context.Context, user string) (SetupPlan, error)
	// Run carries the setup out; it needs root and returns when the service answers.
	Run(ctx context.Context, user string) error
}

// SetupPlan is what a setup still has to do. Without commands there is nothing to do.
type SetupPlan struct {
	Commands []string
	// Warnings are what the person must read before answering.
	Warnings []string
}

// deploySetUp is the result of a finished setup; user is empty for the service of the system engine.
type deploySetUp struct{ user string }

// markNotSetUp decides whether a failed refresh means that the deploy service does not exist yet.
func (m *Model) markNotSetUp(err error) bool {
	m.notSetUp = errors.Is(err, client.ErrNotRunning) && m.deploy != nil && !m.deploy.Installed()
	return m.notSetUp
}

// setupShown reports whether the area shows the setup instead of the deploy service's content. Settings and System
// need no deploy service.
func (m *Model) setupShown() bool {
	return m.notSetUp && m.area != areaSettings && m.area != areaSystem
}

// setupView says that the deploy service is not set up and how to set it up; a click on it is the key.
func (m *Model) setupView(width, height int) string {
	var lines []string
	lines = append(lines, m.wrapped(2, kittui.StyleHint, m.text.Lookup(textSetupBody), width)...)
	lines = lines[:min(len(lines), max(height, 1))]
	m.parts = append(m.parts, mousePart{rect: kittui.Rect{Width: width, Height: len(lines)},
		targets: []kittui.Target{{Rect: kittui.Rect{Width: width, Height: len(lines)}, Zone: kittui.ZoneRows, Action: actSetupDeploy}}})
	return strings.Join(lines, "\n")
}

// setupOffered reports whether G offers the setup: where the deploy service does not exist, and in the area Stacks,
// where the instance of a rootless engine can be added to a host that has the service already.
func (m *Model) setupOffered() bool {
	return m.setupShown() || (m.deploy != nil && m.area == areaStacks && !m.notSetUp && m.set.form == nil && !m.set.keyOpen)
}

// beginSetup asks which deploy service to set up.
func (m *Model) beginSetup() tea.Cmd {
	if m.deploy == nil {
		return nil
	}
	spec := kittui.DialogSpec{Title: textSetupTitle, Question: m.text.Lookup(textAskSetupWhich), Consequence: m.text.Lookup(textWhatSetupWhich),
		Answers: []kittui.Answer{{Action: actYes, Label: textAnswerSetupSystem}, {Action: actRootless, Label: textAnswerSetupRootless},
			{Action: actNo, Label: textAnswerCancel}},
		Default: actNo, Keys: kittui.DialogKeys{Enter: actEnter}}
	dialog, err := newDialog(spec, m.keys, m.text)
	if err != nil {
		m.failure = m.text.Lookup(kittui.TextError, err.Error())
		return nil
	}
	m.modal = &modal{kind: modalAsk, screen: titleScreen(textSetupTitle), dialog: dialog,
		yes: func() tea.Cmd { return m.askSetup("") },
		alt: m.askRootlessUser}
	return nil
}

// askRootlessUser asks for the user whose rootless engine gets the service. The users that have one are the
// hint and the suggestions, and a name that is not among them can be typed too: the list is read from the host
// files and a person may know better, and the plan refuses a user that cannot own an instance.
func (m *Model) askRootlessUser() tea.Cmd {
	users := m.deploy.Users()
	hint := m.text.Lookup(textSetupNoUsers)
	if len(users) > 0 {
		hint = m.text.Lookup(textAskSetupUser, strings.Join(users, ", "))
	}
	suggest := func(typed string) []kittui.Suggestion {
		var out []kittui.Suggestion
		for _, u := range users {
			if strings.HasPrefix(u, typed) {
				out = append(out, kittui.Suggestion{Text: u})
			}
		}
		return out
	}
	dialog, err := newDialog(kittui.DialogSpec{Title: textSetupTitle, Question: hint,
		Input:   &kittui.DialogInput{Suggest: suggest},
		Answers: []kittui.Answer{{Action: actEnter, Label: textAnswerSetupSteps}, {Action: actBack, Label: textAnswerCancel}},
		Default: actEnter,
		Keys:    kittui.DialogKeys{Enter: actEnter, Up: actUp, Down: actDown, Complete: actNext}}, m.keys, m.text)
	if err != nil {
		m.failure = m.text.Lookup(kittui.TextError, err.Error())
		return nil
	}
	m.modal = &modal{kind: modalAsk, screen: titleScreen(textSetupTitle), dialog: dialog, input: func(typed string) tea.Cmd {
		if typed = strings.TrimSpace(typed); typed != "" {
			return m.askSetup(typed)
		}
		return nil
	}}
	return nil
}

// askSetup shows the plan of the setup for user, the empty name being the system engine, and asks for the
// confirmation. A refusal is shown with its reason and a setup that is done already says so.
func (m *Model) askSetup(user string) tea.Cmd {
	plan, err := m.deploy.Plan(m.ctx, user)
	if err != nil {
		m.failure = m.text.Lookup(kittui.TextError, m.text.Lookup(textErrHost, err.Error()))
		return nil
	}
	if len(plan.Commands) == 0 && (user != "" || m.deploy.Installed()) {
		m.warn(m.text.Lookup(textSetupNothing))
		return nil
	}
	var details []kittui.DialogLine
	for _, w := range plan.Warnings {
		details = append(details, kittui.DialogLine{Prefix: "! ", Text: w, Style: kittui.StyleWarn})
	}
	if user != "" {
		details = append(details, kittui.DialogLine{Text: m.text.Lookup(textSetupAsRoot)})
	}
	for _, c := range plan.Commands {
		details = append(details, kittui.DialogLine{Prefix: "$ ", Text: c, Style: kittui.StyleHint})
	}
	question, consequence := m.text.Lookup(textAskSetup), m.text.Lookup(textWhatSetup)
	if user != "" {
		question, consequence = m.text.Lookup(textAskSetupFor, user), m.text.Lookup(textWhatSetupFor, user)
	}
	if m.euid() != 0 {
		m.askSetupSudo(user, question, consequence, details)
		return nil
	}
	spec := kittui.DialogSpec{Title: textSetupTitle, Question: question, Consequence: consequence, Details: details,
		Answers: []kittui.Answer{{Action: actYes, Label: textAnswerYes}, {Action: actNo, Label: textAnswerCancel}},
		Default: actNo, Keys: kittui.DialogKeys{Enter: actEnter}}
	dialog, err := newDialog(spec, m.keys, m.text)
	if err != nil {
		m.failure = m.text.Lookup(kittui.TextError, err.Error())
		return nil
	}
	m.modal = &modal{kind: modalAsk, screen: titleScreen(textSetupTitle), dialog: dialog, yes: func() tea.Cmd {
		return m.writeCall(opDeploySetup, user, func(Backend) (any, error) {
			if err := m.deploy.Run(m.ctx, user); err != nil {
				return nil, hostError{err}
			}
			return deploySetUp{user: user}, nil
		})
	}}
	return nil
}

// askSetupSudo asks to set up with `sudo qhost deploy-service setup`; the steps of the plan are the lead, and the
// command runs them as root itself.
func (m *Model) askSetupSudo(user, question, consequence string, lead []kittui.DialogLine) {
	if m.settingsBusy() {
		m.warn(m.text.Lookup(textActionBusy))
		return
	}
	args := []string{"deploy-service", "setup"}
	if user != "" {
		args = append(args, "--rootless", user)
	}
	args = append(args, "--confirm", "--yes", "--output", "json")
	var gen int
	m.askSudo(sudoAsk{title: textSetupTitle, question: question, consequence: consequence, lead: lead, args: args,
		begin: func() { gen = m.beginSetting() },
		done: func(r sudoRun) tea.Cmd {
			msg := settingMsg{gen: gen, op: opDeploySetup, name: user}
			var v struct {
				SetUp bool `json:"set_up"`
			}
			if !r.decode(&v) || !v.SetUp {
				msg.err = sudoProblem(r.problem)
				return m.applySetting(msg)
			}
			msg.res = deploySetUp{user: user}
			return m.applySetting(msg)
		}})
}
