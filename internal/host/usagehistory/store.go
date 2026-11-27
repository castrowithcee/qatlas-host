// Package usagehistory keeps the history of the host load in its own SQLite database: raw values, minutes and
// hours, each tier with its own retention. Gaps stay empty and a stored row is never replaced.
package usagehistory

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/castrowithcee/qatlas-kit/output"
	"github.com/castrowithcee/qatlas-kit/sqlite"

	"github.com/castrowithcee/qatlas-host/internal/formatnum"
	"github.com/castrowithcee/qatlas-host/internal/host/usage"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// schemaVersion is the number of embedded migrations and so the format number of the file.
const schemaVersion = 1

// Measurement is the load at one moment. Time is stored in UTC at whole seconds.
type Measurement struct {
	Time time.Time
	usage.Usage
}

// Config is the retention of the history. Enabled is read by the caller: a disabled history is not opened.
// Every duration must be positive.
type Config struct {
	Enabled bool
	Raw     time.Duration
	Minutes time.Duration
	Hours   time.Duration
}

// DefaultConfig keeps raw values for 24 hours, minutes for 7 days and hours for 365 days.
func DefaultConfig() Config {
	return Config{Enabled: true, Raw: 24 * time.Hour, Minutes: 7 * 24 * time.Hour, Hours: 365 * 24 * time.Hour}
}

// Store is an open history database. It is safe for concurrent use.
type Store struct {
	db  *sql.DB
	kit *sqlite.DB
	cfg Config
}

// Open opens or creates the database at path. A file that is not this store's, or that a newer version wrote,
// is refused with a *formatnum.Error and not changed.
func Open(path string, cfg Config) (*Store, error) {
	if cfg.Raw <= 0 || cfg.Minutes <= 0 || cfg.Hours <= 0 {
		return nil, errors.New("usage history: every retention must be positive")
	}
	if err := checkOwn(path); err != nil {
		return nil, err
	}
	migrations, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	kit, err := sqlite.Open(path, sqlite.Options{Migrations: migrations, Label: "usage history"})
	if err != nil {
		var se *sqlite.Error
		if errors.As(err, &se) && se.Code == output.CodeConfigInvalid {
			// The kit only reports a history it cannot match to its migrations; for a file of ours that
			// is one a newer version wrote.
			return nil, &formatnum.Error{File: path, Field: "schema_migrations", Found: schemaVersion + 1, Present: true, Expected: schemaVersion}
		}
		return nil, fmt.Errorf("open the usage history: %w", err)
	}
	return &Store{db: kit.SQL(), kit: kit, cfg: cfg}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.kit.Close() }

// Prune deletes the rows of each tier that are older than its retention at now.
func (s *Store) Prune(ctx context.Context, now time.Time) error {
	tiers := []struct {
		tables []string
		keep   time.Duration
	}{
		{[]string{"raw", "raw_fs"}, s.cfg.Raw},
		{[]string{"minute", "minute_fs"}, s.cfg.Minutes},
		{[]string{"hour", "hour_fs"}, s.cfg.Hours},
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, t := range tiers {
		for _, table := range t.tables {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE ts < ?", now.Add(-t.keep).UTC().Unix()); err != nil {
				return fmt.Errorf("prune %s: %w", table, err)
			}
		}
	}
	return tx.Commit()
}
