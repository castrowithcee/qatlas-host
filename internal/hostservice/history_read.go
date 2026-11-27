package hostservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/host/usagehistory"
)

// answerReserve is what the frame around the result and the series names take of control.MaxResponseBytes.
const answerReserve = 64 << 10

var (
	errHistoryOff      = control.NewError(control.CodeNoHistory, "the usage history is turned off on this host. Ask an administrator to turn it on; it then fills from that moment")
	errHistoryStarting = control.NewError(control.CodeNoHistory, "the usage history is not open yet. Run the command again in a moment")
	errHistoryClosed   = control.NewError(control.CodeNoHistory, "the usage history is being reconfigured or the host service is stopping. Run the command again in a moment")
)

// historyOpenError is the answer while the database could not be opened: the reason and what to do.
func historyOpenError(err error) *control.Error {
	return control.NewError(control.CodeNoHistory, fmt.Sprintf("the usage history database could not be opened (%v). "+
		"Read the log of the host service for the cause and fix it; the service opens the database again when its configuration is reloaded or the service restarts", err))
}

// setReader publishes the store the readers use, or the reason there is none. It waits for readers that are
// still inside a query.
func (c *collector) setReader(store historyStore, err error) {
	c.readMu.Lock()
	c.reader, c.readErr = store, err
	c.readMu.Unlock()
}

// read answers q from the store of the running collector, under the retention it was opened with. It fails
// with a *control.Error when the history is off or its database is not open.
func (c *collector) read(ctx context.Context, q usagehistory.Query) (usagehistory.Result, error) {
	c.readMu.RLock()
	defer c.readMu.RUnlock()
	if c.reader == nil {
		return usagehistory.Result{}, c.readErr
	}
	return c.reader.Read(ctx, c.now(), q)
}

// usageHistory reads the history of the host load. Only this service opens the database.
func (s *server) usageHistory(ctx context.Context, raw json.RawMessage) (any, error) {
	var a control.HostUsageHistoryArgs
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, badRequest()
		}
	}
	if s.history == nil {
		return nil, errHistoryStarting
	}
	q, err := historyQuery(a, s.now())
	if err != nil {
		return nil, err
	}
	res, err := s.history.read(ctx, q)
	var qe *usagehistory.QueryError
	if errors.As(err, &qe) {
		return nil, control.NewError(control.CodeBadRequest, qe.Message)
	}
	if err != nil {
		return nil, err
	}
	return historyResult(res), nil
}

func historyQuery(a control.HostUsageHistoryArgs, now time.Time) (usagehistory.Query, error) {
	q := usagehistory.Query{To: now, Resolution: a.Resolution, Metrics: a.Metrics, Budget: control.MaxResponseBytes - answerReserve}
	if q.Resolution == "" {
		q.Resolution = usagehistory.ResolutionAuto
	}
	var err error
	if a.To != "" {
		if q.To, err = time.Parse(time.RFC3339, a.To); err != nil {
			return q, control.NewError(control.CodeBadRequest, "to must be an RFC 3339 time")
		}
	}
	q.From = q.To.Add(-time.Hour)
	if a.From != "" {
		if q.From, err = time.Parse(time.RFC3339, a.From); err != nil {
			return q, control.NewError(control.CodeBadRequest, "from must be an RFC 3339 time")
		}
	}
	return q, nil
}

func historyResult(res usagehistory.Result) control.HostUsageHistoryResult {
	out := control.HostUsageHistoryResult{Resolution: res.Resolution, From: res.From, To: res.To,
		Series: make([]control.HostUsageSeries, len(res.Series))}
	raw := res.Resolution == usagehistory.ResolutionRaw
	for i, s := range res.Series {
		pts := make([]control.HostUsagePoint, len(s.Points))
		for j, p := range s.Points {
			mean, peak := round2(p.Mean), round2(p.Max)
			if raw {
				pts[j] = control.HostUsagePoint{Time: p.Time, Value: &mean}
			} else {
				pts[j] = control.HostUsagePoint{Time: p.Time, Mean: &mean, Max: &peak}
			}
		}
		out.Series[i] = control.HostUsageSeries{Metric: s.Metric, Mount: s.Mount, TotalBytes: s.TotalBytes, Points: pts}
	}
	return out
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
