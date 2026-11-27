package cli

import (
	"math"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/host/usage"
)

func newHostUsageCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	return &cobra.Command{
		Use:   "usage",
		Short: "Show the current load of the host: CPU, load average, memory, swap and file systems",
		Long: "usage asks the host service for the current load and changes nothing. cpu_percent is the busy share of all\n" +
			"cores over a measurement of about 200 ms. load_* is the load average of 1, 5 and 15 minutes, next to the\n" +
			"number of cores. memory_* and swap_* are in bytes, used being the total minus what is available. Each\n" +
			"real file system is listed once as filesystem.<n> with mount, device, type and total, used and free bytes;\n" +
			"pseudo and overlay file systems and a file system that does not answer within a second are left out. A value the host does not provide is left out. Root and\n" +
			"administrators in the group qhost may read; run it without sudo.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			var res control.HostUsageResult
			if err := hostCall(c, state, control.HostUsageCommand, nil, &res); err != nil {
				return err
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: usageFields(res)})
		},
	}
}

// usageFields is the flat form of a result; empty values are dropped by the encoder.
func usageFields(u usage.Usage) []output.Field {
	var f []output.Field
	if u.CPUPercent != nil {
		f = append(f, output.Field{Name: "cpu_percent", Value: math.Round(*u.CPUPercent*10) / 10})
	}
	f = append(f, output.Field{Name: "cores", Value: u.Cores})
	if l := u.Load; l != nil {
		f = append(f, output.Field{Name: "load_1", Value: l.One}, output.Field{Name: "load_5", Value: l.Five},
			output.Field{Name: "load_15", Value: l.Fifteen})
	}
	f = append(f, memoryFields("memory", u.Memory)...)
	f = append(f, memoryFields("swap", u.Swap)...)
	for i, fs := range u.Filesystems {
		p := "filesystem." + strconv.Itoa(i+1) + "."
		f = append(f, output.Field{Name: p + "mount", Value: fs.Mount}, output.Field{Name: p + "device", Value: fs.Device},
			output.Field{Name: p + "type", Value: fs.Type}, output.Field{Name: p + "total_bytes", Value: fs.TotalBytes},
			output.Field{Name: p + "used_bytes", Value: fs.UsedBytes}, output.Field{Name: p + "free_bytes", Value: fs.FreeBytes})
	}
	if u.UptimeSeconds != nil {
		f = append(f, output.Field{Name: "uptime_seconds", Value: *u.UptimeSeconds})
	}
	return f
}

func memoryFields(name string, m *usage.Memory) []output.Field {
	if m == nil {
		return nil
	}
	return []output.Field{{Name: name + "_total_bytes", Value: m.TotalBytes}, {Name: name + "_used_bytes", Value: m.UsedBytes},
		{Name: name + "_available_bytes", Value: m.AvailableBytes}}
}
