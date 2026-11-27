package usagehistory

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"

	"github.com/castrowithcee/qatlas-host/internal/formatnum"
)

// applicationID is set by the first migration (see migrations/0001_usage_history.sql).
const applicationID = 1363085617

const (
	headerSize     = 108
	appIDOffset    = 68
	cellsOffset    = 103
	sqliteMagic    = "SQLite format 3\x00"
	appIDFieldName = "application_id"
)

// checkOwn refuses an existing file that is not this store's before the kit opens it: the kit would back up
// and migrate a foreign SQLite database. It only reads. A missing or empty file passes, and so does a SQLite
// file without any table, which the kit may have left before its first migration ran.
func checkOwn(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return nil // the kit creates the file or refuses what is no file
	}
	f, err := os.Open(path)
	if err != nil {
		return nil // the kit reports the permission error
	}
	defer f.Close()
	hdr := make([]byte, headerSize)
	n, _ := io.ReadFull(f, hdr)
	found := 0
	if n == headerSize && bytes.HasPrefix(hdr, []byte(sqliteMagic)) {
		id := binary.BigEndian.Uint32(hdr[appIDOffset:])
		if id == applicationID || binary.BigEndian.Uint16(hdr[cellsOffset:]) == 0 {
			return nil
		}
		found = int(id)
	}
	return &formatnum.Error{File: path, Field: appIDFieldName, Found: found, Present: found != 0, Expected: applicationID}
}
