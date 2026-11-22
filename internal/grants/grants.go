// Package grants keeps the standing grants of one deploy instance in a single private file below its state
// directory, replaced atomically. It decides when a stored grant is valid and knows nothing of what a grant
// allows at run time. A file it cannot read is never written: every grant then counts as invalid.
package grants

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"github.com/castrowithcee/qatlas-kit/configfile"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/formatnum"
)

const (
	fileName = "grants.json"
	// fileSchema is the format number of the grants file.
	fileSchema = 1
)

var (
	// ErrNotFound answers the removal of an ID no grant has.
	ErrNotFound = errors.New("no such grant")
	// ErrFull answers the addition of a grant to a file that holds control.GrantMax already.
	ErrFull = errors.New("the instance has the most grants it may have")
)

var idRe = regexp.MustCompile(`^` + regexp.QuoteMeta(control.GrantIDPrefix) + `[0-9a-f]{8}$`)

type document struct {
	Schema int             `json:"schema"`
	Grants []control.Grant `json:"grants"`
}

// Store is the file of the grants of one instance.
type Store struct {
	store *configfile.Store[document]
}

// Open prepares the file below stateDir; nothing is read or written yet.
func Open(stateDir string) (*Store, error) {
	path := filepath.Join(stateDir, fileName)
	st, err := configfile.NewStore(path, configfile.Codec[document]{
		Validate: func(d document) error {
			if err := formatnum.Check(path, "schema", d.Schema, fileSchema); err != nil {
				return err
			}
			return validate(d.Grants)
		},
		Encode: func(d document) ([]byte, error) { return json.MarshalIndent(d, "", "  ") },
		Decode: func(b []byte) (document, error) {
			var d document
			if err := formatnum.CheckJSON(path, "schema", b, fileSchema); err != nil {
				return d, err
			}
			dec := json.NewDecoder(bytes.NewReader(b))
			dec.DisallowUnknownFields()
			return d, dec.Decode(&d)
		},
	})
	if err != nil {
		return nil, err
	}
	return &Store{store: st}, nil
}

// validate checks the shape of every grant: the form of its parts and the uniqueness of its ID and of its
// user, action and stack. What the parts mean is the business of the caller.
func validate(list []control.Grant) error {
	if len(list) > control.GrantMax {
		return errors.New("too many grants")
	}
	ids := map[string]bool{}
	keys := map[string]bool{}
	for _, g := range list {
		if !idRe.MatchString(g.ID) || ids[g.ID] {
			return errors.New("invalid or repeated grant ID")
		}
		ids[g.ID] = true
		key := fmt.Sprintf("%d\x00%s\x00%s", g.UID, g.Action, g.Stack)
		if keys[key] {
			return errors.New("grant stored twice")
		}
		keys[key] = true
		if !slices.Contains(control.GrantActions, g.Action) || g.User == "" || g.Stack == "" {
			return errors.New("invalid grant")
		}
		if g.Expires != "" {
			if _, err := ParseExpires(g.Expires); err != nil {
				return errors.New("invalid expiry")
			}
		}
	}
	return nil
}

// ParseExpires reads an expiry date of the form control.GrantExpiresLayout and returns its start in UTC.
func ParseExpires(s string) (time.Time, error) {
	return time.ParseInLocation(control.GrantExpiresLayout, s, time.UTC)
}

// List returns the grants in the order they were added. A missing file holds none. Any other failure, a file
// that is damaged or of an unknown format number included, is the error; the file stays as it is.
func (s *Store) List() ([]control.Grant, error) {
	d, _, err := s.store.LoadVersioned()
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return d.Grants, nil
}

// Upsert stores g. A grant with the same user ID, action and stack is the same grant: it keeps its ID and its
// origin and takes the user name and the expiry of g, and replaced is true. A new grant gets a new ID.
func (s *Store) Upsert(g control.Grant) (stored control.Grant, replaced bool, err error) {
	_, err = s.store.Update(func(d document) (document, error) {
		d.Schema = fileSchema
		for i, x := range d.Grants {
			if x.UID == g.UID && x.Action == g.Action && x.Stack == g.Stack {
				d.Grants[i].User, d.Grants[i].Expires = g.User, g.Expires
				stored, replaced = d.Grants[i], true
				return d, nil
			}
		}
		if len(d.Grants) >= control.GrantMax {
			return d, ErrFull
		}
		id, err := newID(d.Grants)
		if err != nil {
			return d, err
		}
		g.ID = id
		d.Grants = append(d.Grants, g)
		stored = g
		return d, nil
	})
	if err != nil {
		return control.Grant{}, false, err
	}
	return stored, replaced, nil
}

// Remove deletes the grant with the ID and returns it; ErrNotFound when there is none.
func (s *Store) Remove(id string) (control.Grant, error) {
	var gone control.Grant
	_, err := s.store.Update(func(d document) (document, error) {
		i := slices.IndexFunc(d.Grants, func(x control.Grant) bool { return x.ID == id })
		if i < 0 {
			return d, ErrNotFound
		}
		d.Schema = fileSchema
		gone = d.Grants[i]
		d.Grants = slices.Delete(d.Grants, i, i+1)
		return d, nil
	})
	return gone, err
}

func newID(taken []control.Grant) (string, error) {
	for range 8 {
		var b [4]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		id := control.GrantIDPrefix + hex.EncodeToString(b[:])
		if !slices.ContainsFunc(taken, func(x control.Grant) bool { return x.ID == id }) {
			return id, nil
		}
	}
	return "", errors.New("no free grant ID")
}

// Invalid returns why g does not hold at now, one of the control.Grant reasons, or "" for a valid grant. The
// first reason that applies is named: expired, then stack missing, then user changed. stackKnown tells whether
// the instance has the stack; nameOf returns the name the user ID has today and false for nobody.
func Invalid(g control.Grant, now time.Time, stackKnown func(string) bool, nameOf func(uint32) (string, bool)) string {
	if g.Expires != "" {
		day, err := ParseExpires(g.Expires)
		if err != nil || !now.UTC().Before(day.AddDate(0, 0, 1)) {
			return control.GrantExpired
		}
	}
	if !stackKnown(g.Stack) {
		return control.GrantStackMissing
	}
	if name, ok := nameOf(g.UID); !ok || name != g.User {
		return control.GrantUserChanged
	}
	return ""
}
