// Package rootless finds the rootless Docker engines of the host's users and asks each one for its container
// list, so a caller running as root can report containers of a stack folder that run outside the engine of
// the deploy service. It only lists containers and never changes, starts or stops anything.
package rootless

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/sys/unix"
)

const (
	// DefaultRunDir is the parent of the per-user runtime folders.
	DefaultRunDir = "/run/user"
	// DefaultPasswd is the user database.
	DefaultPasswd = "/etc/passwd"
	// DefaultTimeout bounds the container list of one engine.
	DefaultTimeout = 5 * time.Second

	// maxAnswer bounds the answer of one engine; a larger answer makes the engine not checked.
	maxAnswer = 8 << 20

	labelProject    = "com.docker.compose.project"
	labelWorkingDir = "com.docker.compose.project.working_dir"

	socketName = "docker.sock"
	unitName   = "docker.service"
)

// User is one entry of the user database.
type User struct {
	Name string
	UID  uint32
	Home string
}

// Container is the part of an engine's container entry this package reads.
type Container struct {
	Name       string // without the leading slash
	Project    string // Compose project label, empty if the container has none
	WorkingDir string // Compose working directory label
	State      string // engine state word such as running or exited
	Image      string // image reference as the engine lists it
}

// Client asks one engine for its containers with a single request and nothing else.
type Client interface {
	List(ctx context.Context) ([]Container, error)
	Close()
}

// Dialer opens a client for the engine behind socket. The path may be a /proc/self/fd path that stays valid
// until the client is closed.
type Dialer func(socket string, timeout time.Duration) (Client, error)

var (
	errTooLarge = errors.New("the engine answer is too large")
	errStatus   = errors.New("the engine answered with an unexpected status")
)

type httpClient struct{ c *http.Client }

// HTTPDialer is the default Dialer: plain HTTP over the unix socket, no proxy, no redirects.
func HTTPDialer(socket string, timeout time.Duration) (Client, error) {
	if !filepath.IsAbs(socket) {
		return nil, errors.New("the socket path must be absolute")
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: timeout}).DialContext(ctx, "unix", socket)
		},
		DisableKeepAlives: true,
	}
	return httpClient{&http.Client{Transport: tr, Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (h httpClient) Close() { h.c.CloseIdleConnections() }

// List sends exactly one request, GET /containers/json?all=1.
func (h httpClient) List(ctx context.Context) ([]Container, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/json?all=1", nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errStatus
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAnswer+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxAnswer {
		return nil, errTooLarge
	}
	var items []struct {
		Names  []string
		Labels map[string]string
		State  string
		Image  string
	}
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, err
	}
	out := make([]Container, 0, len(items))
	for _, it := range items {
		out = append(out, Container{Name: itemName(it.Names), Project: it.Labels[labelProject],
			WorkingDir: it.Labels[labelWorkingDir], State: it.State, Image: it.Image})
	}
	return out, nil
}

// itemName picks the container's own name: the first one without a second slash (the others are link aliases).
func itemName(names []string) string {
	for _, n := range names {
		if t := strings.TrimPrefix(n, "/"); !strings.Contains(t, "/") {
			return t
		}
	}
	if len(names) > 0 {
		return strings.TrimPrefix(names[0], "/")
	}
	return ""
}

// Detector finds and asks the rootless engines. The zero value uses the real host.
type Detector struct {
	RunDir  string        // default DefaultRunDir
	Users   []User        // default: the users of DefaultPasswd
	Dial    Dialer        // default HTTPDialer
	Timeout time.Duration // per engine, default DefaultTimeout
}

// NotChecked is an engine that is set up but was not asked, with the reason.
type NotChecked struct {
	Engine string // "rootless:<user>"
	Reason string
}

// Conflict is one container in a rootless engine that belongs to a stack folder.
type Conflict struct {
	Container string
	Project   string
	Engine    string // "rootless:<user>"
}

type engineList struct {
	id         string
	containers []Container
}

// Scan is the answer of all rootless engines. Text taken from an engine is masked when a method returns it.
type Scan struct {
	NotChecked []NotChecked
	engines    []engineList
}

// Engine is one scanned rootless engine with its containers; the text is masked.
type Engine struct {
	ID         string // "rootless:<user>"
	Containers []Container
}

// Engines returns the engines that answered, with the name, project, working directory, state and image of
// each container, in the order of the users. It is the same single list request per engine as Conflicts uses.
func (s Scan) Engines() []Engine {
	out := make([]Engine, 0, len(s.engines))
	for _, e := range s.engines {
		list := make([]Container, len(e.containers))
		for i, c := range e.containers {
			list[i] = Container{Name: maskControl(c.Name), Project: maskControl(c.Project),
				WorkingDir: maskControl(c.WorkingDir), State: maskControl(c.State), Image: maskControl(c.Image)}
		}
		out = append(out, Engine{ID: e.id, Containers: list})
	}
	return out
}

// Conflicts returns every container of a Compose project in any scanned engine whose working directory label
// equals dir, whatever the project's name. Results are sorted by engine, project and container.
func (s Scan) Conflicts(dir string) []Conflict {
	if dir == "" {
		return nil
	}
	dir = filepath.Clean(dir)
	var out []Conflict
	for _, e := range s.engines {
		for _, c := range e.containers {
			if c.Project == "" || c.WorkingDir == "" || filepath.Clean(c.WorkingDir) != dir {
				continue
			}
			out = append(out, Conflict{Container: maskControl(c.Name), Project: maskControl(c.Project), Engine: e.id})
		}
	}
	slices.SortStableFunc(out, func(a, b Conflict) int {
		return cmp.Or(cmp.Compare(a.Engine, b.Engine), cmp.Compare(a.Project, b.Project), cmp.Compare(a.Container, b.Container))
	})
	return out
}

// Scan asks every rootless engine of the users once. A failing engine only becomes NotChecked.
func (d Detector) Scan(ctx context.Context) Scan {
	users := d.Users
	if users == nil {
		var err error
		if users, err = ReadUsers(DefaultPasswd); err != nil {
			return Scan{NotChecked: []NotChecked{{Engine: "rootless", Reason: "the user database could not be read"}}}
		}
	}
	runDir := cmp.Or(d.RunDir, DefaultRunDir)
	dial := d.Dial
	if dial == nil {
		dial = HTTPDialer
	}
	timeout := d.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	var s Scan
	for _, u := range users {
		id := "rootless:" + maskControl(u.Name)
		path := filepath.Join(runDir, strconv.FormatUint(uint64(u.UID), 10), socketName)
		list, state, reason := query(ctx, path, u.UID, dial, timeout)
		switch state {
		case found:
			s.engines = append(s.engines, engineList{id: id, containers: list})
		case failed:
			s.NotChecked = append(s.NotChecked, NotChecked{Engine: id, Reason: reason})
		case missing:
			if setUp(u) {
				s.NotChecked = append(s.NotChecked, NotChecked{Engine: id, Reason: "the rootless Docker service is set up but its socket is not there, so the engine is not running"})
			}
		case unsafe:
			if setUp(u) {
				s.NotChecked = append(s.NotChecked, NotChecked{Engine: id, Reason: reason})
			}
		}
	}
	return s
}

type state int

const (
	missing state = iota // no socket path at all
	unsafe               // something is there that is not the user's own socket; never contacted
	failed               // the engine was contacted or could not be, with reason
	found
)

// query opens the socket path without following a link, checks type and owner on the opened object and
// connects through that same object, so a swap of the path after the check does not redirect the connection.
func query(ctx context.Context, path string, uid uint32, dial Dialer, timeout time.Duration) ([]Container, state, string) {
	fd, err := unix.Open(path, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, unix.ENOTDIR) {
			return nil, missing, ""
		}
		if errors.Is(err, unix.ELOOP) {
			return nil, unsafe, "the engine socket is a link, so it was not contacted"
		}
		return nil, failed, "the engine socket could not be opened"
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return nil, failed, "the engine socket could not be inspected"
	}
	if st.Mode&unix.S_IFMT == unix.S_IFLNK {
		return nil, unsafe, "the engine socket is a link, so it was not contacted"
	}
	if st.Mode&unix.S_IFMT != unix.S_IFSOCK {
		return nil, unsafe, "the engine socket path is not a socket, so it was not contacted"
	}
	if st.Uid != uid {
		return nil, unsafe, "the engine socket belongs to another user, so it was not contacted"
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client, err := dial(fmt.Sprintf("/proc/self/fd/%d", fd), timeout)
	if err != nil {
		return nil, failed, "the engine client could not be prepared"
	}
	defer client.Close()
	type answer struct {
		list []Container
		err  error
	}
	ch := make(chan answer, 1)
	go func() {
		l, e := client.List(ctx)
		ch <- answer{l, e}
	}()
	select {
	case a := <-ch:
		if a.err != nil {
			if ctx.Err() != nil || errors.Is(a.err, context.DeadlineExceeded) {
				return nil, failed, "the engine did not answer in time"
			}
			if errors.Is(a.err, errTooLarge) {
				return nil, failed, "the engine answer was too large"
			}
			if errors.Is(a.err, errStatus) {
				return nil, failed, "the engine answered with an error status"
			}
			return nil, failed, "the engine could not be asked"
		}
		return a.list, found, ""
	case <-ctx.Done():
		return nil, failed, "the engine did not answer in time"
	}
}

// setUp reports whether the user has a user unit docker.service, the mark of a rootless Docker setup.
func setUp(u User) bool {
	if u.Home == "" || !filepath.IsAbs(u.Home) {
		return false
	}
	_, err := os.Lstat(filepath.Join(u.Home, ".config", "systemd", "user", unitName))
	return err == nil
}

// ReadUsers parses a passwd file; malformed lines are skipped.
func ReadUsers(path string) ([]User, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseUsers(f)
}

func parseUsers(r io.Reader) ([]User, error) {
	var out []User
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Split(sc.Text(), ":")
		if len(f) < 7 || f[0] == "" || strings.HasPrefix(f[0], "#") {
			continue
		}
		uid, err := strconv.ParseUint(f[2], 10, 32)
		if err != nil {
			continue
		}
		out = append(out, User{Name: f[0], UID: uint32(uid), Home: f[5]})
	}
	return out, sc.Err()
}

// maskControl replaces every rune that is not printable with U+FFFD, like the container text elsewhere.
func maskControl(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return unicode.ReplacementChar
	}, s)
}
