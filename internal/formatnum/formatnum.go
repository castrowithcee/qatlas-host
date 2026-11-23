// Package formatnum is the one refusal every stored state uses when the format number of a file is not one
// this version reads. A file with such a number is never read in part, guessed at, or changed.
package formatnum

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// ErrFormat matches every *Error.
var ErrFormat = errors.New("unsupported format number")

// Error names the file, the number found in it and the number this version reads.
type Error struct {
	// File is the path of the file, or the label of the store when no path is known.
	File string
	// Field is the name of the number in the document.
	Field string
	// Found is the number in the file; Present is false when the file has none.
	Found    int
	Present  bool
	Expected int
}

func (e *Error) Error() string {
	switch {
	case !e.Present:
		return fmt.Sprintf("%s: no format number (field %q), this version reads %d; the file is left unchanged, move it aside to start afresh",
			e.File, e.Field, e.Expected)
	case e.Found > e.Expected:
		return fmt.Sprintf("%s: format number %d is newer than the %d this version reads; the file is left unchanged, use the version that wrote it",
			e.File, e.Found, e.Expected)
	}
	return fmt.Sprintf("%s: format number %d cannot be read, this version reads %d; the file is left unchanged, move it aside to start afresh",
		e.File, e.Found, e.Expected)
}

func (e *Error) Is(target error) bool { return target == ErrFormat }

// Check accepts the number current and the older numbers a transition reads. Numbers start at 1, so found 0
// is a missing number.
func Check(file, field string, found, current int, older ...int) error {
	if found == current {
		return nil
	}
	for _, o := range older {
		if found == o {
			return nil
		}
	}
	return &Error{File: file, Field: field, Found: found, Present: found != 0, Expected: current}
}

// CheckJSON reads only the number from a JSON object, before any strict decoding, so a newer file with fields
// this version does not know is still named by its number. A document that is no JSON object is left to its
// decoder.
func CheckJSON(file, field string, data []byte, current int, older ...int) error {
	return CheckJSONMissing(file, field, data, 0, current, older...)
}

// CheckJSONMissing is CheckJSON for a store that was written without a number before the number was
// introduced: a document without the field counts as number missing, which is 1 for such a store. A field that
// is present but unreadable is still refused.
func CheckJSONMissing(file, field string, data []byte, missing, current int, older ...int) error {
	var top map[string]json.RawMessage
	if json.Unmarshal(bytes.TrimSpace(data), &top) != nil || top == nil {
		return nil
	}
	raw, ok := top[field]
	if !ok {
		return Check(file, field, missing, current, older...)
	}
	var n int
	if json.NewDecoder(bytes.NewReader(raw)).Decode(&n) != nil {
		n = 0
	}
	return Check(file, field, n, current, older...)
}

// Is reports whether err is a format number refusal.
func Is(err error) bool { return errors.Is(err, ErrFormat) }

// CheckFile reads the JSON file at path and checks its number, changing nothing. A file that does not exist
// passes, since the service creates it on the first change; any other read error is returned as it is.
func CheckFile(path, field string, current int, older ...int) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return CheckJSON(path, field, data, current, older...)
}
