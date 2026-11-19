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
type DeploySetup interface {
	// Installed reports whether the deploy service is set up.
	Installed() bool
	// Plan lists the exact commands the setup still needs; it changes nothing.
	Plan(ctx context.Context) ([]string, error)
	// Run carries the setup out; it needs root and returns when the service answers.
	Run(ctx context.Context) error
}

// deploySetUp is the result of a finished setup.
type deploySetUp struct{}

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
	if m.euid() != 0 {
		lines = append(lines, "")
		lines = append(lines, m.wrapped(2, kittui.StyleWarn, m.text.Lookup(textSetupNeedsRoot), width)...)
	}
	lines = lines[:min(len(lines), max(height, 1))]
	m.parts = append(m.parts, mousePart{rect: kittui.Rect{Width: width, Height: len(lines)},
		targets: []kittui.Target{{Rect: kittui.Rect{Width: width, Height: len(lines)}, Zone: kittui.ZoneRows, Action: actSetupDeploy}}})
	return strings.Join(lines, "\n")
}

// beginSetup asks for the confirmation of the setup. Without root it only says what is needed.
func (m *Model) beginSetup() tea.Cmd {
	if m.deploy == nil || !m.notSetUp {
		return nil
	}
	if m.euid() != 0 {
		m.warn(m.text.Lookup(textSetupNeedsRoot))
		return nil
	}
	commands, err := m.deploy.Plan(m.ctx)
	if err != nil {
		m.failure = m.text.Lookup(kittui.TextError, m.text.Lookup(textErrHost, err.Error()))
		return nil
	}
	details := make([]kittui.DialogLine, len(commands))
	for i, c := range commands {
		details[i] = kittui.DialogLine{Prefix: "$ ", Text: c, Style: kittui.StyleHint}
	}
	spec := kittui.DialogSpec{Title: textSetupTitle, Question: m.text.Lookup(textAskSetup), Consequence: m.text.Lookup(textWhatSetup),
		Details: details,
		Answers: []kittui.Answer{{Action: actYes, Label: textAnswerYes}, {Action: actNo, Label: textAnswerCancel}},
		Default: actNo, Keys: kittui.DialogKeys{Enter: actEnter}}
	dialog, err := kittui.NewDialog(spec, m.keys, m.text)
	if err != nil {
		m.failure = m.text.Lookup(kittui.TextError, err.Error())
		return nil
	}
	m.modal = &modal{kind: modalAsk, dialog: dialog, yes: func() tea.Cmd {
		return m.writeCall(opDeploySetup, "", func(Backend) (any, error) {
			if err := m.deploy.Run(m.ctx); err != nil {
				return nil, hostError{err}
			}
			return deploySetUp{}, nil
		})
	}}
	return nil
}
