package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/castrowithcee/qatlas-kit/redact"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/formatnum"
)

const (
	// logDirName is the folder of the service log inside the state directory.
	logDirName = "service-log"
	// The store keeps at most logFileCount files of about logFileBytes each, and drops a file that was last
	// written longer ago than logMaxAge. Together that is a few MiB at most.
	logFileBytes = 1 << 20
	logFileCount = 5
	logMaxAge    = 14 * 24 * time.Hour
	// logSourceKey is the top-level key that marks the service of a stored entry.
	logSourceKey = "svc"
	// Bounds of one answered entry, so that a single huge line cannot fill an answer.
	logMessageBytes = 2 << 10
	logAttrBytes    = 1 << 10
	logAttrCount    = 64
	// logAnswerBudget bounds the encoded entries of one answer, well below control.MaxResponseBytes.
	logAnswerBudget = 640 << 10
	// logSchema is the format number a log file starts with, as the JSON line {"schema":1}.
	logSchema = 1
)

// logHeader is the first line of every log file of the store.
var logHeader = []byte(fmt.Sprintf("{\"schema\":%d}\n", logSchema))

// logFormat checks the format number on the first line of the log file at path. A file without the number
// was written before it existed and is format 1; one with another number is not read, appended to, pruned or
// removed by the store.
func logFormat(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	// The first entry of a file without the header line can be longer than any fixed buffer.
	var first json.RawMessage
	err = json.NewDecoder(io.LimitReader(f, logFileBytes)).Decode(&first)
	var syn *json.SyntaxError
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.As(err, &syn) {
		return err
	}
	if err != nil {
		return formatnum.Check(path, "schema", 0, logSchema)
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(first, &top) != nil || top == nil {
		return formatnum.Check(path, "schema", 0, logSchema)
	}
	return formatnum.CheckJSONMissing(path, "schema", first, 1, logSchema)
}

// usable reports whether the file name of the store has the format number this version reads.
func (s *logStore) usable(name string) bool { return logFormat(filepath.Join(s.dir, name)) == nil }

// logStore is the bounded, rotating file store of the service log. The files hold JSON lines that were masked
// before they got here and are readable by the service user only. Writing never fails the caller's logging: a
// failed write is dropped, and the next write tries again.
type logStore struct {
	dir      string
	maxBytes int64
	maxFiles int
	maxAge   time.Duration
	now      func() time.Time

	mu   sync.Mutex
	cur  *os.File
	size int64
}

func newLogStore(dir string) *logStore {
	return &logStore{dir: dir, maxBytes: logFileBytes, maxFiles: logFileCount, maxAge: logMaxAge, now: time.Now}
}

// handler returns the slog handler that writes records into the store, tagged with the service name.
func (s *logStore) handler() slog.Handler {
	return slog.NewJSONHandler(s, &slog.HandlerOptions{Level: slog.LevelInfo}).
		WithAttrs([]slog.Attr{slog.String(logSourceKey, control.ServiceLogSourceHost)})
}

// files lists the store's files, oldest first. The names are the creation time in nanoseconds, zero-padded.
func (s *logStore) files() []string {
	ents, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range ents {
		if n, ok := strings.CutSuffix(e.Name(), ".jsonl"); ok && e.Type().IsRegular() && validLogFileID(n) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

func validLogFileID(n string) bool {
	if len(n) != 20 {
		return false
	}
	_, err := strconv.ParseUint(n, 10, 64)
	return err == nil
}

// Write appends one complete JSON line, rotating first when the file would grow past its bound.
func (s *logStore) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur != nil && s.size > int64(len(logHeader)) && s.size+int64(len(p)) > s.maxBytes {
		_ = s.cur.Close()
		s.cur = nil
	}
	if s.cur == nil {
		if err := s.open(int64(len(p))); err != nil {
			return 0, err
		}
	}
	n, err := s.cur.Write(p)
	s.size += int64(n)
	if err != nil {
		_ = s.cur.Close()
		s.cur = nil
	}
	return n, err
}

// open continues the newest file if the line still fits, otherwise starts a new one, and prunes.
func (s *logStore) open(next int64) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(s.dir, 0o700); err != nil {
		return err
	}
	all := s.files()
	names := slices.DeleteFunc(slices.Clone(all), func(n string) bool { return !s.usable(n) })
	if n := len(names); n > 0 {
		if st, err := os.Stat(filepath.Join(s.dir, names[n-1])); err == nil && st.Size() <= int64(len(logHeader)) || err == nil && st.Size()+next <= s.maxBytes {
			if f, err := os.OpenFile(filepath.Join(s.dir, names[n-1]), os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
				s.cur, s.size = f, st.Size()
				s.prune(names)
				return nil
			}
		}
	}
	id := uint64(s.now().UnixNano())
	if n := len(all); n > 0 {
		last, _ := strconv.ParseUint(strings.TrimSuffix(all[n-1], ".jsonl"), 10, 64)
		id = max(id, last+1)
	}
	name := fmt.Sprintf("%020d.jsonl", id)
	f, err := os.OpenFile(filepath.Join(s.dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(logHeader); err != nil {
		_ = f.Close()
		_ = os.Remove(filepath.Join(s.dir, name))
		return err
	}
	s.cur, s.size = f, int64(len(logHeader))
	s.prune(append(names, name))
	return nil
}

// prune removes the oldest files beyond the count and the files not written for longer than the age; the file
// being written is kept. names holds only files of the format this version reads: any other file stays.
func (s *logStore) prune(names []string) {
	for len(names) > s.maxFiles {
		_ = os.Remove(filepath.Join(s.dir, names[0]))
		names = names[1:]
	}
	for _, n := range names[:max(len(names)-1, 0)] {
		if st, err := os.Stat(filepath.Join(s.dir, n)); err == nil && s.now().Sub(st.ModTime()) > s.maxAge {
			_ = os.Remove(filepath.Join(s.dir, n))
		}
	}
}

// logRecord is one stored entry as read back; end is the cursor that follows it.
type logRecord struct {
	entry control.ServiceLogEntry
	level slog.Level
	end   string
}

// scan calls visit for every complete stored entry after the cursor, in the order written, until visit returns
// false. last is the cursor behind the last line looked at; it is the argument cursor when nothing was read.
// A cursor is the file name and the byte offset after a line.
func (s *logStore) scan(cursor string, secrets *redact.Redactor, visit func(logRecord) bool) (last string) {
	last = cursor
	cname, coff, _ := parseLogCursor(cursor)
	for _, name := range s.files() {
		if !s.usable(name) {
			continue
		}
		var off int64
		switch {
		case cursor != "" && name < cname:
			continue
		case cursor != "" && name == cname:
			off = coff
		}
		f, err := os.Open(filepath.Join(s.dir, name))
		if err != nil {
			continue
		}
		stop := false
		func() {
			defer f.Close()
			if _, err := f.Seek(off, io.SeekStart); err != nil {
				return
			}
			rd := bufio.NewReader(f)
			for {
				line, err := rd.ReadBytes('\n')
				if err != nil {
					return // end of file, or a line still being written
				}
				off += int64(len(line))
				last = fmt.Sprintf("%s:%d", name, off)
				rec, ok := parseLogLine(line, secrets)
				if !ok {
					continue
				}
				rec.end = last
				if !visit(rec) {
					stop = true
					return
				}
			}
		}()
		if stop {
			break
		}
	}
	return last
}

func parseLogCursor(c string) (name string, off int64, ok bool) {
	n, o, found := strings.Cut(c, ":")
	if !found || !validLogFileID(strings.TrimSuffix(n, ".jsonl")) || !strings.HasSuffix(n, ".jsonl") {
		return "", 0, false
	}
	off, err := strconv.ParseInt(o, 10, 64)
	return n, off, err == nil && off >= 0
}

// parseLogLine reads one JSON line the store wrote; an unreadable line is not an entry.
func parseLogLine(line []byte, secrets *redact.Redactor) (logRecord, bool) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(line), &top); err != nil {
		return logRecord{}, false
	}
	var rec logRecord
	var lvl string
	if json.Unmarshal(top[slog.TimeKey], &rec.entry.Time) != nil || json.Unmarshal(top[slog.LevelKey], &lvl) != nil ||
		rec.level.UnmarshalText([]byte(lvl)) != nil {
		return logRecord{}, false
	}
	var msg, src string
	_ = json.Unmarshal(top[slog.MessageKey], &msg)
	_ = json.Unmarshal(top[logSourceKey], &src)
	delete(top, slog.TimeKey)
	delete(top, slog.LevelKey)
	delete(top, slog.MessageKey)
	delete(top, logSourceKey)
	text := func(v string, limit int) string { return maskControl(secrets.Apply(cutUTF8(v, limit))) }
	rec.entry.Time = rec.entry.Time.UTC()
	rec.entry.Level = strings.ToLower(rec.level.String())
	rec.entry.Source = src
	rec.entry.Message = text(msg, logMessageBytes)
	var attrs []control.ServiceLogAttr
	flattenLogAttrs("", top, &attrs)
	slices.SortFunc(attrs, func(a, b control.ServiceLogAttr) int { return strings.Compare(a.Key, b.Key) })
	if len(attrs) > logAttrCount {
		attrs = attrs[:logAttrCount]
	}
	for i := range attrs {
		attrs[i].Key = text(attrs[i].Key, logAttrBytes)
		attrs[i].Value = text(attrs[i].Value, logAttrBytes)
	}
	rec.entry.Attrs = attrs
	return rec, true
}

func flattenLogAttrs(prefix string, m map[string]json.RawMessage, out *[]control.ServiceLogAttr) {
	for k, raw := range m {
		key := prefix + k
		var sub map[string]json.RawMessage
		var str string
		switch {
		case json.Unmarshal(raw, &sub) == nil && sub != nil:
			flattenLogAttrs(key+".", sub, out)
		case json.Unmarshal(raw, &str) == nil:
			*out = append(*out, control.ServiceLogAttr{Key: key, Value: str})
		default:
			*out = append(*out, control.ServiceLogAttr{Key: key, Value: string(raw)})
		}
	}
}

// cutUTF8 cuts s to at most n bytes at a character boundary and marks the cut.
func cutUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "…"
}

// logLevelOf parses a level name of the command; empty means the lowest.
func logLevelOf(name string) (slog.Level, bool) {
	switch name {
	case "":
		return slog.LevelDebug - 1000, true
	case control.LogLevelDebug:
		return slog.LevelDebug, true
	case control.LogLevelInfo:
		return slog.LevelInfo, true
	case control.LogLevelWarn:
		return slog.LevelWarn, true
	case control.LogLevelError:
		return slog.LevelError, true
	}
	return 0, false
}

type logService struct {
	store   *logStore
	secrets *redact.Redactor
}

func registerServiceLogCommands(r *control.Registry, store *logStore, secrets *redact.Redactor) error {
	s := &logService{store: store, secrets: secrets}
	return r.Register(control.ServiceLogCommand, reading(s.handle))
}

func (s *logService) handle(_ context.Context, raw json.RawMessage) (any, error) {
	var a control.ServiceLogArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	min, okLevel := logLevelOf(a.Level)
	_, _, okCursor := parseLogCursor(a.Cursor)
	if !okLevel || a.Tail < 0 || a.Tail > control.ServiceLogMaxTail || a.Tail > 0 && a.Cursor != "" ||
		a.Cursor != "" && !okCursor || !a.Since.IsZero() && a.Since.Unix() < 0 ||
		a.Source != "" && a.Source != control.ServiceLogSourceHost {
		return nil, control.NewError(control.CodeInvalid, "the log arguments are not valid")
	}
	res := control.ServiceLogResult{Entries: []control.ServiceLogEntry{}}
	var ends []string
	used := 0
	res.Cursor = s.store.scan(a.Cursor, s.secrets, func(rec logRecord) bool {
		if rec.level < min || !a.Since.IsZero() && rec.entry.Time.Before(a.Since) {
			return true
		}
		b, err := json.Marshal(rec.entry)
		if err != nil {
			return true
		}
		if a.Tail == 0 && used+len(b)+1 > logAnswerBudget && len(res.Entries) > 0 {
			res.More = true
			return false
		}
		used += len(b) + 1
		res.Entries = append(res.Entries, rec.entry)
		ends = append(ends, rec.end)
		if a.Tail > 0 && len(res.Entries) > a.Tail {
			res.Entries, ends = res.Entries[1:], ends[1:]
		}
		return true
	})
	if res.More {
		res.Cursor = ends[len(ends)-1]
	}
	if a.Tail > 0 {
		// The kept entries may hold more than one answer; the oldest give way.
		for {
			b, err := json.Marshal(res.Entries)
			if err != nil || len(b) <= logAnswerBudget || len(res.Entries) <= 1 {
				break
			}
			res.Entries = res.Entries[1:]
		}
	}
	return res, nil
}

// teeHandler writes every record to the stderr handler first and then to the store. The store never takes
// the record from stderr: its failures are dropped, and its panics are not expected from a file write.
type teeHandler struct{ primary, store slog.Handler }

func (h teeHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.primary.Enabled(ctx, l) || h.store.Enabled(ctx, l)
}

func (h teeHandler) Handle(ctx context.Context, rec slog.Record) error {
	var err error
	if h.primary.Enabled(ctx, rec.Level) {
		err = h.primary.Handle(ctx, rec.Clone())
	}
	if h.store.Enabled(ctx, rec.Level) {
		_ = h.store.Handle(ctx, rec.Clone())
	}
	return err
}

func (h teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return teeHandler{primary: h.primary.WithAttrs(attrs), store: h.store.WithAttrs(attrs)}
}

func (h teeHandler) WithGroup(name string) slog.Handler {
	return teeHandler{primary: h.primary.WithGroup(name), store: h.store.WithGroup(name)}
}
