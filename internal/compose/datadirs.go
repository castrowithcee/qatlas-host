package compose

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-host/internal/idmap"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

const (
	// maxImageFile bounds /etc/passwd and /etc/group read from an image.
	maxImageFile = 1 << 20
	// imageCallTimeout bounds each docker call that inspects an image.
	imageCallTimeout = 30 * time.Second
	// HelperImage is the helper image: busybox 1.37.0, pinned by the digest of its multi-platform index.
	HelperImage = "busybox@sha256:a7dbc319b1438115a207e11930dc4bb3e643dcb3ec0bdd694bb36cd8fad289ec"
	// NamespaceScript prints the two mappings of the user namespace a container of the engine runs in, separated
	// by a line with two dashes. It is constant.
	NamespaceScript = "cat /proc/self/uid_map; echo --; cat /proc/self/gid_map"
)

var (
	numberRe = regexp.MustCompile(`^[0-9]{1,10}$`)
	// imageNameRe keeps an image reference that starts with a dash or holds odd bytes away from the docker CLI.
	imageNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+-]*$`)
	containerID = regexp.MustCompile(`^[0-9a-f]{12,64}$`)
)

// MissingDataDirs implements engine.DataDirChecker.
func (d *Deployer) MissingDataDirs(ctx context.Context, st stacks.Stack,
	versioned func(context.Context, string) (bool, error), log *engine.Log) ([]engine.MissingDir, error) {
	return d.Exec.MissingDataDirs(ctx, st, versioned, log)
}

type dataService struct {
	Image   string `json:"image"`
	User    string `json:"user"`
	Volumes []struct {
		Type   string `json:"type"`
		Source string `json:"source"`
		Bind   *struct {
			CreateHostPath *bool `json:"create_host_path"`
		} `json:"bind"`
	} `json:"volumes"`
}

type dataCandidate struct {
	rel     string
	service string
}

// MissingDataDirs finds the bind-mount source directories that do not exist, lie inside the stack folder, are
// not versioned and belong to a service whose user is not root: Docker would create them owned by root. The
// proposed owner comes from the service's user or the image's. Nothing is created and no image code runs.
//
// The rendered configuration holds interpolated secrets; only service names, images, users and bind sources
// are read from it, and none of it is logged. A user that cannot be determined means "as before": no finding.
func (x *Executor) MissingDataDirs(ctx context.Context, st stacks.Stack,
	versioned func(context.Context, string) (bool, error), log Logger) ([]engine.MissingDir, error) {
	raw, err := x.renderConfig(ctx, st, log)
	if err != nil {
		return nil, err
	}
	var cfg struct {
		Services map[string]dataService `json:"services"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, errors.New("docker compose config: unexpected output")
	}
	dir, err := filepath.Abs(st.Dir)
	if err != nil {
		return nil, fmt.Errorf("stack directory: %w", err)
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, fmt.Errorf("stack directory: %w", err)
	}

	names := make([]string, 0, len(cfg.Services))
	for n := range cfg.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	var found []dataCandidate
	var unreadable []string
	seen := map[string]bool{}
	for _, n := range names {
		for _, v := range cfg.Services[n].Volumes {
			if v.Type != "bind" || (v.Bind != nil && v.Bind.CreateHostPath != nil && !*v.Bind.CreateHostPath) {
				continue
			}
			rel, ok, err := missingBelow(dir, real, v.Source)
			if err != nil {
				unreadable = append(unreadable, err.Error())
				continue
			}
			if !ok || seen[rel] {
				continue
			}
			seen[rel] = true
			found = append(found, dataCandidate{rel: rel, service: n})
		}
	}
	if len(unreadable) > 0 {
		sort.Strings(unreadable)
		return nil, fmt.Errorf("a data directory cannot be checked: %s", strings.Join(slices.Compact(unreadable), "; "))
	}
	if len(found) == 0 {
		return nil, nil
	}

	users := &imageUsers{x: x, st: st, log: log, cfg: cfg.Services, cache: map[string]*imageInfo{}}
	var out []engine.MissingDir
	host := &hostIDs{x: x, log: log, dir: dir}
	for _, c := range found {
		if versioned != nil {
			v, err := versioned(ctx, c.rel)
			if err != nil {
				return nil, fmt.Errorf("check whether %s is versioned: %w", c.rel, err)
			}
			if v {
				continue
			}
		}
		owner, needed := users.owner(ctx, c.service)
		if !needed {
			continue
		}
		d := engine.MissingDir{Path: c.rel}
		if owner != "" {
			d.Container = owner
			d.Owner, d.Warning = host.translate(ctx, owner)
			if d.Warning != "" {
				log.Printf("no owner is proposed for %s: %s", c.rel, d.Warning)
			}
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// missingBelow returns the path of source relative to the stack folder dir when it is a candidate: absolute,
// strictly inside dir, absent, and with the deepest existing ancestor resolving inside realDir, the stack
// folder with symlinks resolved. Anything else is no candidate, except a path that cannot be examined (no
// access to a parent, say): that is an error, so it is reported and not passed over.
func missingBelow(dir, realDir, source string) (string, bool, error) {
	if !filepath.IsAbs(source) || strings.ContainsRune(source, 0) {
		return "", false, nil
	}
	p := filepath.Clean(source)
	rel, err := filepath.Rel(dir, p)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false, nil
	}
	rel = filepath.ToSlash(rel)
	if _, err := os.Lstat(p); !errors.Is(err, fs.ErrNotExist) {
		return "", false, examineError(rel, err)
	}
	anc := filepath.Dir(p)
	for {
		_, err := os.Lstat(anc)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrNotExist) || anc == dir {
			return "", false, examineError(rel, err)
		}
		anc = filepath.Dir(anc)
	}
	res, err := filepath.EvalSymlinks(anc)
	if err != nil {
		return "", false, examineError(rel, err)
	}
	if res != realDir && !strings.HasPrefix(res, realDir+string(filepath.Separator)) {
		return "", false, nil
	}
	return rel, true, nil
}

// examineError is nil for the outcomes that only make a path no candidate: it exists (err is nil), a parent is a
// file, or a link dangles. Any other error, in particular a refused access, is returned with the path.
func examineError(rel string, err error) error {
	if err == nil || errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) || errors.Is(err, syscall.ELOOP) {
		return nil
	}
	return fmt.Errorf("%s: %w", rel, err)
}

// hostIDs converts the UID:GID of a container user into the host IDs they stand for. The engine's mapping is
// read once, on the first need, through a helper container that runs in the namespace of the engine's
// containers.
type hostIDs struct {
	x      *Executor
	log    Logger
	dir    string
	loaded bool
	uid    idmap.Map
	gid    idmap.Map
	err    error
}

// translate returns the host "UID:GID" of the container "UID:GID" owner. When the mapping cannot be read or
// an ID lies outside it, the owner is empty and the warning says why.
func (h *hostIDs) translate(ctx context.Context, owner string) (string, string) {
	if !h.loaded {
		h.loaded = true
		h.uid, h.gid, h.err = h.x.idMaps(ctx, h.log, h.dir)
	}
	if h.err != nil {
		return "", "the user namespace of the engine cannot be read: " + h.err.Error()
	}
	us, gs, _ := strings.Cut(owner, ":")
	hostUID, okU := h.uid.Outside(uint64(atoi(us)))
	hostGID, okG := h.gid.Outside(uint64(atoi(gs)))
	switch {
	case !okU:
		return "", "container user " + us + " is outside the user ID range of the engine"
	case !okG:
		return "", "container group " + gs + " is outside the group ID range of the engine"
	case hostUID >= 1<<32-1 || hostGID >= 1<<32-1:
		return "", "the host ID of container user " + owner + " is out of range"
	}
	return strconv.FormatUint(hostUID, 10) + ":" + strconv.FormatUint(hostGID, 10), ""
}

// idMaps returns the UID and GID mapping of the engine's containers: IDMaps when set, else read through a
// container of the helper image, which runs no code of the stacks.
func (x *Executor) idMaps(ctx context.Context, log Logger, dir string) (idmap.Map, idmap.Map, error) {
	if x.IDMaps != nil {
		return x.IDMaps(ctx, log)
	}
	img := x.HelperImage
	if img == "" {
		img = HelperImage
	}
	var out bytes.Buffer
	c := Command{Name: dockerProgram, Dir: dir, Env: x.Env, Stdout: &out, Args: []string{"run", "--rm", "--network", "none",
		"--read-only", "--security-opt", "no-new-privileges", img, "sh", "-c", NamespaceScript}}
	if _, err := x.runNamed(ctx, log, c, orDefault(x.PullTimeout, DefaultPullTimeout), "engine namespace"); err != nil {
		return nil, nil, err
	}
	uidText, gidText, ok := strings.Cut(out.String(), "\n--\n")
	if !ok {
		return nil, nil, errors.New("unexpected output of the helper container")
	}
	uid, err := idmap.Parse(uidText)
	if err != nil {
		return nil, nil, err
	}
	gid, err := idmap.Parse(gidText)
	if err != nil {
		return nil, nil, err
	}
	ownUID, ownGID, err := readMaps(ownMapFiles)
	if err != nil {
		return nil, nil, fmt.Errorf("the user namespace of the service cannot be read: %w", err)
	}
	uid, gid = hostView(uid, gid, ownUID, ownGID)
	return uid, gid, nil
}

// ownMapFiles are the UID and GID map files of the namespace this process runs in.
var ownMapFiles = [2]string{"/proc/self/uid_map", "/proc/self/gid_map"}

// readMaps reads and parses a UID and a GID map file.
func readMaps(files [2]string) (idmap.Map, idmap.Map, error) {
	var maps [2]idmap.Map
	for i, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, nil, err
		}
		if maps[i], err = idmap.Parse(string(data)); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", f, err)
		}
	}
	return maps[0], maps[1], nil
}

// hostView returns the engine's mappings relative to the namespace of this process. A container lists its
// mapping against the parent of its namespace. When it shares the namespace of this process, as the containers
// of a system engine do, that parent lies above the host, which happens on a host that is itself a container,
// and its IDs already are host IDs.
func hostView(uid, gid, ownUID, ownGID idmap.Map) (idmap.Map, idmap.Map) {
	if uid.Equal(ownUID) && gid.Equal(ownGID) {
		return idmap.Default(), idmap.Default()
	}
	return uid, gid
}

// imageInfo is what is known about an image: the user it runs as, and /etc/passwd and /etc/group once read.
type imageInfo struct {
	user         string
	userKnown    bool
	files        bool // passwd and group were tried
	passwd, grps []string
}

type imageUsers struct {
	x     *Executor
	st    stacks.Stack
	log   Logger
	cfg   map[string]dataService
	cache map[string]*imageInfo
}

// owner returns the proposed owner of the service's data directories and whether the service runs as a
// non-root user at all.
func (u *imageUsers) owner(ctx context.Context, service string) (string, bool) {
	svc := u.cfg[service]
	img := func() *imageInfo { return u.image(ctx, service, svc.Image) }
	user, known := svc.User, svc.User != ""
	if !known {
		if i := img(); i != nil && i.userKnown {
			user, known = i.user, true
		}
	}
	if !known {
		u.log.Printf("the user of service %s is unknown; its data directories are left to Docker", service)
		return "", false
	}
	return u.resolve(ctx, img, svc.Image, user)
}

// resolve turns a user specification (name or uid, optionally ":group") into "UID:GID". The second result is
// false for root, which Docker's own directory creation serves.
func (u *imageUsers) resolve(ctx context.Context, img func() *imageInfo, ref, user string) (string, bool) {
	uPart, gPart, _ := strings.Cut(user, ":")
	if uPart == "" || uPart == "root" || (numberRe.MatchString(uPart) && atoi(uPart) == 0) {
		return "", false
	}
	var uid, gid string
	if numberRe.MatchString(uPart) {
		uid = uPart
	}
	needFiles := uid == "" || gPart == "" || !numberRe.MatchString(gPart)
	var passwd, groups []string
	if needFiles {
		if i := img(); i != nil {
			u.readFiles(ctx, i, ref)
			passwd, groups = i.passwd, i.grps
		}
	}
	if uid == "" {
		var ok bool
		if uid, gid, ok = lookupPasswd(passwd, 0, uPart); !ok {
			return "", true
		}
		if atoi(uid) == 0 {
			return "", false
		}
	} else if _, g, ok := lookupPasswd(passwd, 2, uid); ok {
		gid = g
	}
	switch {
	case gPart == "":
		if gid == "" {
			gid = uid
		}
	case numberRe.MatchString(gPart):
		gid = gPart
	default:
		g, ok := lookupGroup(groups, gPart)
		if !ok {
			return "", true
		}
		gid = g
	}
	return uid + ":" + gid, true
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

// lookupPasswd returns uid and gid of the first passwd record whose name (field 0) or uid (field 2) equals
// value.
func lookupPasswd(lines []string, field int, value string) (uid, gid string, ok bool) {
	for _, l := range lines {
		f := strings.Split(l, ":")
		if len(f) >= 4 && f[field] == value && numberRe.MatchString(f[2]) && numberRe.MatchString(f[3]) {
			return f[2], f[3], true
		}
	}
	return "", "", false
}

// lookupGroup returns the gid of the group with the given name.
func lookupGroup(lines []string, name string) (string, bool) {
	for _, l := range lines {
		f := strings.Split(l, ":")
		if len(f) >= 3 && f[0] == name && numberRe.MatchString(f[2]) {
			return f[2], true
		}
	}
	return "", false
}

// image returns the user of the service's image, pulling it first when it is not local.
func (u *imageUsers) image(ctx context.Context, service, ref string) *imageInfo {
	if ref == "" || !imageNameRe.MatchString(ref) {
		return nil
	}
	if info, ok := u.cache[ref]; ok {
		return info
	}
	info := &imageInfo{}
	u.cache[ref] = info
	user, err := u.inspectUser(ctx, ref)
	if err != nil {
		// The new images are pulled only after the stack stopped; the user is needed before. Pulling
		// an image changes no running container.
		if perr := u.x.step(ctx, u.st, u.log, orDefault(u.x.PullTimeout, DefaultPullTimeout), "pull", "pull", "--quiet", service); perr == nil {
			user, err = u.inspectUser(ctx, ref)
		}
	}
	if err != nil {
		return info
	}
	info.user, info.userKnown = user, true
	return info
}

func (u *imageUsers) inspectUser(ctx context.Context, ref string) (string, error) {
	out, err := u.docker(ctx, "image inspect", "image", "inspect", "--format", "{{json .Config.User}}", ref)
	if err != nil {
		return "", err
	}
	var user string
	if err := json.Unmarshal(bytes.TrimSpace(out), &user); err != nil {
		return "", errors.New("docker image inspect: unexpected output")
	}
	return user, nil
}

func (u *imageUsers) docker(ctx context.Context, name string, args ...string) ([]byte, error) {
	return u.x.dockerOut(ctx, u.log, name, args...)
}

// dockerOut runs one bounded docker call and returns its standard output.
func (x *Executor) dockerOut(ctx context.Context, log Logger, name string, args ...string) ([]byte, error) {
	buf := &limitBuffer{limit: maxImageFile}
	c := Command{Name: dockerProgram, Args: args, Env: x.Env, Stdout: buf}
	if _, err := x.runNamed(ctx, log, c, imageCallTimeout, "docker "+name); err != nil {
		return nil, err
	}
	if buf.over {
		return nil, fmt.Errorf("docker %s: the output is too large", name)
	}
	return buf.b.Bytes(), nil
}

// readFiles reads /etc/passwd and /etc/group from the image through a created, never started container, so no
// code of the image runs. A failure leaves the lists empty.
func (u *imageUsers) readFiles(ctx context.Context, img *imageInfo, ref string) {
	if img.files {
		return
	}
	img.files = true
	out, err := u.docker(ctx, "create", "create", "--pull", "never", "--network", "none", "--entrypoint", "/nonexistent", ref)
	id := strings.TrimSpace(string(out))
	if err != nil || !containerID.MatchString(id) {
		u.log.Printf("the users of image %s cannot be read", ref)
		return
	}
	defer func() {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), imageCallTimeout)
		defer cancel()
		if _, err := u.x.dockerOut(rctx, u.log, "rm", "rm", "-f", id); err != nil {
			u.log.Printf("the temporary container %s could not be removed", id)
		}
	}()
	img.passwd = u.imageFile(ctx, id, "/etc/passwd")
	img.grps = u.imageFile(ctx, id, "/etc/group")
}

func (u *imageUsers) imageFile(ctx context.Context, id, path string) []string {
	raw, err := u.docker(ctx, "cp", "cp", "-L", id+":"+path, "-")
	if err != nil {
		return nil
	}
	return firstTarFile(raw)
}

// firstTarFile returns the lines of the first regular file of a tar stream.
func firstTarFile(raw []byte) []string {
	tr := tar.NewReader(bytes.NewReader(raw))
	for {
		h, err := tr.Next()
		if err != nil {
			return nil
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(tr, maxImageFile))
		if err != nil {
			return nil
		}
		return strings.Split(string(body), "\n")
	}
}
