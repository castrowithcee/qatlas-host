package cli

import (
	"math"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func newHostUsageHistoryCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	var from, to, resolution string
	var metrics []string
	cmd := &cobra.Command{
		Use:   "usage-history",
		Short: "Show the history of the host load for a period: CPU, load average, memory, swap and file systems",
		Long: "usage-history asks the host service for the history of the load it keeps and changes nothing. --from and --to\n" +
			"take a duration back from now (90m, 6h) or an RFC 3339 time; without them the period is the last hour.\n" +
			"--resolution is auto (the default), raw, minute or hour. Auto takes the finest resolution that still keeps\n" +
			"the whole period and gives at most 1500 points per series; a resolution you name that cannot answer is an\n" +
			"error that names the one that can. Raw values come every 10 seconds, minutes and hours as mean and maximum.\n" +
			"--metric selects series and may repeat: cpu_percent, load_one, memory_used_bytes, swap_used_bytes and\n" +
			"filesystem_used_bytes, which has one series per mount.\n\n" +
			"The text shows the resolution, the period and, per series, the number of points and the lowest, mean and\n" +
			"highest value; for minutes and hours the lowest and the mean are those of the bucket means. Times with no\n" +
			"measurement are missing, never filled. --output json lists every point as t (unix seconds) with v, or with\n" +
			"mean and max. The history is read by the host service only. It is an error when the history is off or its\n" +
			"database cannot be opened. Root and administrators in the group qhost may read; run it without sudo.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			now := time.Now()
			start, err := parseLogTime("--from", from, now)
			if err != nil {
				return err
			}
			end, err := parseLogTime("--to", to, now)
			if err != nil {
				return err
			}
			a := control.HostUsageHistoryArgs{Resolution: resolution, Metrics: metrics}
			if !start.IsZero() {
				a.From = start.UTC().Format(time.RFC3339)
			}
			if !end.IsZero() {
				a.To = end.UTC().Format(time.RFC3339)
			}
			var res control.HostUsageHistoryResult
			if err := hostCall(c, state, control.HostUsageHistoryCommand, a, &res); err != nil {
				return err
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: usageHistoryFields(res, program.Format())})
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "start of the period: a duration back from now or an RFC 3339 time (default: one hour before --to)")
	cmd.Flags().StringVar(&to, "to", "", "end of the period: a duration back from now or an RFC 3339 time (default: now)")
	cmd.Flags().StringVar(&resolution, "resolution", "auto", "auto, raw, minute or hour")
	cmd.Flags().StringSliceVar(&metrics, "metric", nil, "only this series; may repeat (default: all)")
	return cmd
}

// usageHistoryFields is every point for JSON and a flat summary per series for the text formats.
func usageHistoryFields(res control.HostUsageHistoryResult, format output.Format) []output.Field {
	f := []output.Field{{Name: "resolution", Value: res.Resolution},
		{Name: "from", Value: res.From.UTC().Format(time.RFC3339)}, {Name: "to", Value: res.To.UTC().Format(time.RFC3339)}}
	if format == output.FormatJSON {
		return append(f, output.Field{Name: "series", Value: res.Series})
	}
	n := 0
	for _, s := range res.Series {
		p := s.Metric + "_"
		if s.Mount != "" {
			n++
			p = "filesystem." + strconv.Itoa(n) + "."
			f = append(f, output.Field{Name: p + "mount", Value: s.Mount}, output.Field{Name: p + "total_bytes", Value: s.TotalBytes})
		}
		f = append(f, output.Field{Name: p + "points", Value: len(s.Points)})
		if len(s.Points) > 0 {
			if s.Mount != "" {
				p += "used_"
			}
			lo, mean, hi := seriesSummary(s.Points)
			f = append(f, output.Field{Name: p + "min", Value: lo}, output.Field{Name: p + "mean", Value: mean}, output.Field{Name: p + "max", Value: hi})
		}
	}
	return f
}

// seriesSummary returns the lowest and the mean of the values (the bucket means for minutes and hours) and
// the highest of the maxima.
func seriesSummary(points []control.HostUsagePoint) (lo, mean, hi float64) {
	lo = pointValue(points[0])
	hi = pointPeak(points[0])
	var sum float64
	for _, p := range points {
		sum += pointValue(p)
		lo, hi = min(lo, pointValue(p)), max(hi, pointPeak(p))
	}
	return round2(lo), round2(sum / float64(len(points))), round2(hi)
}

func pointValue(p control.HostUsagePoint) float64 {
	if p.Value != nil {
		return *p.Value
	}
	if p.Mean != nil {
		return *p.Mean
	}
	return 0
}

func pointPeak(p control.HostUsagePoint) float64 {
	if p.Max != nil {
		return *p.Max
	}
	return pointValue(p)
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
