package usagehistory

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Write stores a batch in one transaction and then closes the minutes and hours that are complete at the
// newest time of the batch. An existing row is kept: a time that is already stored is ignored.
func (s *Store) Write(ctx context.Context, batch []Measurement) error {
	if len(batch) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var newest int64
	for _, m := range batch {
		ts := m.Time.UTC().Unix()
		newest = max(newest, ts)
		if err := insertRaw(ctx, tx, ts, m); err != nil {
			return err
		}
	}
	if err := rollUp(ctx, tx, newest); err != nil {
		return err
	}
	return tx.Commit()
}

func insertRaw(ctx context.Context, tx *sql.Tx, ts int64, m Measurement) error {
	var load, mem, swap any
	if m.Load != nil {
		load = m.Load.One
	}
	if m.Memory != nil {
		mem = m.Memory.UsedBytes
	}
	if m.Swap != nil {
		swap = m.Swap.UsedBytes
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO raw (ts, cpu_percent, load_one, memory_used, swap_used)
		VALUES (?, ?, ?, ?, ?)`, ts, m.CPUPercent, load, mem, swap); err != nil {
		return fmt.Errorf("store a raw value: %w", err)
	}
	for _, fsys := range m.Filesystems {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO raw_fs (ts, mount, used, total) VALUES (?, ?, ?, ?)`,
			ts, fsys.Mount, fsys.UsedBytes, fsys.TotalBytes); err != nil {
			return fmt.Errorf("store a file system value: %w", err)
		}
	}
	return nil
}

const aggregates = `AVG(cpu_percent%[1]s), MAX(cpu_percent%[2]s), AVG(load_one%[1]s), MAX(load_one%[2]s),
	AVG(memory_used%[1]s), MAX(memory_used%[2]s), AVG(swap_used%[1]s), MAX(swap_used%[2]s)`

// rollUp builds the minutes from the raw values and the hours from the minutes. A bucket is complete when it
// ends at or before the bucket that holds newest. The hours of buckets already stored are ignored, so a
// clock that stepped back never rewrites them.
func rollUp(ctx context.Context, tx *sql.Tx, newest int64) error {
	steps := []struct {
		from, to string
		width    int64
		avg, max string
	}{
		{"raw", "minute", int64(time.Minute / time.Second), "", ""},
		{"minute", "hour", int64(time.Hour / time.Second), "_avg", "_max"},
	}
	for _, st := range steps {
		limit := newest / st.width * st.width
		cols := fmt.Sprintf(aggregates, st.avg, st.max)
		q := fmt.Sprintf(`INSERT OR IGNORE INTO %s SELECT ts / %d * %d, %s FROM %s WHERE ts < ? GROUP BY ts / %d`,
			st.to, st.width, st.width, cols, st.from, st.width)
		if _, err := tx.ExecContext(ctx, q, limit); err != nil {
			return fmt.Errorf("close the %ss: %w", st.to, err)
		}
		fsCols := "AVG(used), MAX(used), MAX(total)"
		if st.from != "raw" {
			fsCols = "AVG(used_avg), MAX(used_max), MAX(total)"
		}
		q = fmt.Sprintf(`INSERT OR IGNORE INTO %s_fs SELECT ts / %d * %d, mount, %s FROM %s_fs WHERE ts < ? GROUP BY ts / %d, mount`,
			st.to, st.width, st.width, fsCols, st.from, st.width)
		if _, err := tx.ExecContext(ctx, q, limit); err != nil {
			return fmt.Errorf("close the %ss of the file systems: %w", st.to, err)
		}
	}
	return nil
}
