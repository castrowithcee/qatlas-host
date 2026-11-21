// Package service holds the long-running loop of the qhost service and the channel the command line talks to
// it over.
package service

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/castrowithcee/qatlas-kit/docker"
	"github.com/castrowithcee/qatlas-kit/localipc"
	"github.com/castrowithcee/qatlas-kit/notify"
	"github.com/castrowithcee/qatlas-kit/redact"

	"github.com/castrowithcee/qatlas-host/internal/changelog"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/credentials"
	"github.com/castrowithcee/qatlas-host/internal/engine"
	registryclient "github.com/castrowithcee/qatlas-host/internal/registry"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

// DefaultStateDir is the state directory of the deploy service under systemd's StateDirectory convention.
const DefaultStateDir = "/var/lib/qhost-deploy"

// Config is what Run needs from its caller.
type Config struct {
	// Version is reported by the status command.
	Version string
	// Endpoint is the channel to listen on; control.DeployEndpoint() in production.
	Endpoint localipc.ServiceEndpoint
	// StateDir holds the credential store; it is required. DefaultStateDir in production.
	StateDir string
	// PeerCheck admits the peers of the channel and tells their user. Nil means the user the kernel reports; tests
	// inject their own. Whatever it admits, every request is still checked against the group database.
	PeerCheck PeerCheck
	// GroupFile and PasswdFile are the group database of that check; empty means /etc/group and /etc/passwd.
	// Tests point them at files of their own.
	GroupFile, PasswdFile string
	// Git is the git layer of the repository commands. Nil means the system git through internal/gitrepo;
	// tests inject their own.
	Git Git
	// Executable is the program the credential helper link points at; empty means the running program. Tests
	// inject their own.
	Executable string
	// Deployer brings a stack to a commit. Nil means the engine only polls, updates the working copy and
	// reports pending stacks; it never deploys.
	Deployer engine.Deployer
	// Backups saves and restores the data of a stack. Nil means the backup commands answer that backups are
	// not available.
	Backups Backups
	// Registry configures the registry clients of the update detection; tests point it at a local server. Its
	// Credential is ignored: logins come from registryCredential.
	Registry registryclient.Options
	// Changelog configures the GitHub client of the changelogs; tests point it at a local server. Its Token and
	// CacheDir are ignored: the token comes from the credential "github.com" and the cache lives in StateDir.
	Changelog changelog.Options
	// DockerSocket is the socket of the container engine; empty means DefaultDockerSocket. It is read only
	// when Docker is nil.
	DockerSocket string
	// Docker reads the containers of the host. Nil means the container engine at DockerSocket; tests inject
	// their own.
	Docker Docker
	// ContainerWait bounds how long a container start or restart waits for the container to be ready; zero means
	// one minute. Tests shorten it.
	ContainerWait time.Duration
	// PollInterval is the time between repository polls; zero means engine.DefaultInterval.
	PollInterval time.Duration
	// Clock drives the poll loop and the maintenance window; nil means the system clock. Tests inject their own.
	Clock engine.Clock
	// Notify configures the notification sender; tests give it the roots of a local server and short waits. Its
	// Redactor is ignored: the service's own redactor is used.
	Notify notify.Options
	// MonitorInterval is the time between container readings of the failure monitor; zero means 30 seconds.
	MonitorInterval time.Duration
	// OnMonitorEvent, when set, is told every failed and recovered transition of an own stack's containers and
	// every halted repair. It runs in the monitor's goroutine and must return quickly.
	OnMonitorEvent func(MonitorEvent)
	// OnUpdateAvailable, when set, is told every stack in manual mode the maintenance window found an update
	// for. It runs in the window's goroutine and must return quickly.
	OnUpdateAvailable func(UpdateNotice)

	// registryHook, when set, sees the registry once every command is registered. Tests only.
	registryHook func(*control.Registry)
}

// Run serves the channel until ctx ends and then returns nil: a stop request is the normal way for the service
// to end. It fails if the channel cannot be opened.
func Run(ctx context.Context, log *slog.Logger, cfg Config) error {
	secrets := &redact.Redactor{}
	next := log.Handler()
	var logs *logStore
	if cfg.StateDir != "" {
		// The store sits behind the masking, so only masked lines reach the files.
		logs = newLogStore(filepath.Join(cfg.StateDir, logDirName))
		next = teeHandler{primary: next, store: logs.handler()}
	} else {
		logs = newLogStore("")
	}
	log = slog.New(redactHandler{next: next, r: secrets})
	// The channel's lock must keep a second instance away before opening the store sweeps the temporary
	// files of a write the first instance may be doing.
	l, err := cfg.Endpoint.Listen()
	if err != nil {
		return fmt.Errorf("open the service channel: %w", err)
	}
	defer func() { _ = l.Close() }()
	store, err := credentials.Open(cfg.StateDir, secrets.Add)
	if err != nil {
		return fmt.Errorf("open the credential store: %w", err)
	}
	if cfg.Deployer != nil {
		exe := cfg.Executable
		if exe == "" {
			if exe, err = resolvedExecutable(); err != nil {
				return fmt.Errorf("find the running program: %w", err)
			}
		}
		if err := prepareDockerConfig(cfg.StateDir, exe, log); err != nil {
			return fmt.Errorf("prepare the docker configuration: %w", err)
		}
	}
	registry := control.NewRegistry()
	if err := registerServiceLogCommands(registry, logs, secrets); err != nil {
		return err
	}
	started := time.Now().UTC().Truncate(time.Second)
	notifier, err := newNotifyService(cfg.StateDir, store, cfg.Notify, secrets, cfg.Version, log)
	if err != nil {
		return fmt.Errorf("prepare the notifications: %w", err)
	}
	// Deferred first, so it runs after the engine has stopped and queued its last events.
	defer notifier.close()
	if err := registerNotifyCommands(registry, notifier); err != nil {
		return err
	}
	deployer := cfg.Deployer
	if deployer != nil {
		deployer = redactingDeployer{next: deployer, r: secrets}
	}
	eng, err := openSources(cfg.StateDir, store, cfg.Git, engine.Config{
		Deployer: deployer, Events: notifier.runEvent, Redact: secrets.Apply, Interval: cfg.PollInterval, Clock: cfg.Clock,
	}, log)
	if err != nil {
		return fmt.Errorf("prepare the stack folders: %w", err)
	}
	notifier.bindRunLog(eng.AppendLog)
	if err := registerCredentialCommands(registry, store, eng); err != nil {
		return err
	}
	repo := newRepoService(cfg.StateDir, store, eng)
	if err := registerRepoCommands(registry, repo); err != nil {
		return err
	}
	upd, err := registerUpdateCommands(registry, repo, eng, store, cfg.Registry, cfg.Changelog)
	if err != nil {
		return err
	}
	win, err := newWindowService(cfg.StateDir, upd, cfg.Clock, log, func(n UpdateNotice) {
		if cfg.OnUpdateAvailable != nil {
			cfg.OnUpdateAvailable(n)
		}
		notifier.updateAvailable(n)
	})
	if err != nil {
		return fmt.Errorf("prepare the maintenance window: %w", err)
	}
	if err := registerWindowCommands(registry, win); err != nil {
		return err
	}
	if err := registerRunCommands(registry, eng, cfg.Version, started, win.status); err != nil {
		return err
	}
	if err := registerBackupCommands(registry, eng, cfg.Backups, secrets.Apply,
		func(st stacks.Stack) { registerStackSecrets(secrets, st) }, log); err != nil {
		return err
	}
	if err := registerStackActionCommands(registry, eng, secrets.Apply, log); err != nil {
		return err
	}
	db := groupDB{group: cmp.Or(cfg.GroupFile, defaultGroupFile), passwd: cmp.Or(cfg.PasswdFile, defaultPasswdFile)}
	var proposalNow func() time.Time
	if cfg.Clock != nil {
		proposalNow = cfg.Clock.Now
	}
	hooks := &proposalHooks{log: log, stackKnown: eng.hasStack, channelKnown: notifier.hasChannel}
	if err := registerProposalCommands(registry, cfg.StateDir, db.passwd, proposalNow, hooks); err != nil {
		return err
	}
	if err := registerSettingsCommands(registry, eng, secrets.Apply, log); err != nil {
		return err
	}
	repo.stopped = func(name string) *control.HoldInfo {
		if st, ok, err := eng.Stack(name); err == nil && ok && st.Stopped != nil {
			return &control.HoldInfo{Since: st.Stopped.Since}
		}
		return nil
	}
	if err := registerSecretCommands(registry, eng, secrets, log); err != nil {
		return err
	}
	dock := cfg.Docker
	if dock == nil {
		socket := cfg.DockerSocket
		if socket == "" {
			socket = DefaultDockerSocket
		}
		client, err := docker.New(docker.Options{Socket: socket})
		if err != nil {
			return fmt.Errorf("prepare the container engine client: %w", err)
		}
		defer client.Close()
		dock = engineClient{client}
	}
	if err := registerContainerCommands(registry, repo, eng, secrets, dock, cfg.ContainerWait, log); err != nil {
		return err
	}
	if err := registerInspectCommands(registry, repo, dock, log); err != nil {
		return err
	}
	mon := newContainerMonitor(monitorConfig{
		Docker: dock, Engine: eng, Clock: cfg.Clock, Interval: cfg.MonitorInterval, Log: log,
		StackDirs: func() map[string]string { return repo.stackDirs(log, nil) },
		Hook: func(ev MonitorEvent) {
			logMonitorEvent(log, ev)
			notifier.monitorEvent(ev)
			if cfg.OnMonitorEvent != nil {
				cfg.OnMonitorEvent(ev)
			}
		},
	})
	engineDone := make(chan struct{})
	go func() {
		defer close(engineDone)
		eng.run(ctx)
	}()
	defer func() { <-engineDone }()
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		mon.run(ctx)
	}()
	defer func() { <-monitorDone }()
	windowDone := make(chan struct{})
	go func() {
		defer close(windowDone)
		win.sched.Run(ctx)
	}()
	defer func() { <-windowDone }()
	if cfg.registryHook != nil {
		cfg.registryHook(registry)
	}
	check := cfg.PeerCheck
	if check == nil {
		check = kernelPeer
	}
	log.Info("service started")
	serve(ctx, l, registry, check, db.authorize, log)
	log.Info("service stopped")
	return nil
}
