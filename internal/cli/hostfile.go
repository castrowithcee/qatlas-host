package cli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

func newHostFileCommand(program *cobracli.Program) *cobra.Command {
	return &cobra.Command{
		Use:   "file <path>",
		Short: "List a folder or show a file of the host (needs root)",
		Long: fmt.Sprintf("Reads as root and changes nothing. A folder is listed with the name, kind (file, folder, link or other),\n"+
			"size, owner, group, permissions and modification time of each entry and, for a link, its target as text.\n"+
			"A file shows the same and its content; a file larger than %d bytes or with binary content (a NUL byte\n"+
			"or no valid UTF-8) shows only that, with too_large or binary set and an empty content. A link is never\n"+
			"followed, neither on the way nor at the end: a link as the path shows only itself and its target.",
			hostchange.MaxFileContent),
		Args: exactlyOne("path"),
		RunE: func(c *cobra.Command, args []string) error {
			if err := requireRoot("host file"); err != nil {
				return err
			}
			read, err := hostchange.ReadPath(args[0])
			if err != nil {
				return hostError("host file", err)
			}
			names := newOwnerNames()
			if read.Entries != nil || read.Entry.Kind == hostchange.KindFolder {
				result := output.Collection{Columns: fileColumns}
				for _, e := range read.Entries {
					result.Rows = append(result.Rows, names.row(e))
				}
				return output.Encode(c.OutOrStdout(), program.Format(), result)
			}
			e := read.Entry
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
				{Name: "path", Value: args[0]},
				{Name: "kind", Value: e.Kind},
				{Name: "size", Value: e.Size},
				{Name: "owner", Value: names.user(e.UID)},
				{Name: "group", Value: names.group(e.GID)},
				{Name: "mode", Value: modeText(e.Mode)},
				{Name: "modified", Value: e.ModTime.Format(time.RFC3339)},
				{Name: "target", Value: e.Target},
				{Name: "too_large", Value: read.TooLarge},
				{Name: "binary", Value: read.Binary},
				{Name: "content", Value: string(read.Content)},
			}})
		},
	}
}

var fileColumns = []string{"name", "kind", "size", "owner", "group", "mode", "modified", "target"}

func modeText(mode uint32) string { return fmt.Sprintf("%04o", mode) }

// ownerNames turns owner and group numbers into names; a number without an account stays a number.
type ownerNames struct{ users, groups map[int]string }

func newOwnerNames() ownerNames {
	n := ownerNames{users: map[int]string{}, groups: map[int]string{}}
	h := hostEnv.reader()
	if users, err := h.Users(); err == nil {
		for _, u := range users {
			n.users[u.UID] = u.Name
		}
	}
	if groups, err := h.Groups(); err == nil {
		for _, g := range groups {
			n.groups[g.GID] = g.Name
		}
	}
	return n
}

func (n ownerNames) user(uid int) string {
	if name, ok := n.users[uid]; ok {
		return name
	}
	return strconv.Itoa(uid)
}

func (n ownerNames) group(gid int) string {
	if name, ok := n.groups[gid]; ok {
		return name
	}
	return strconv.Itoa(gid)
}

func (n ownerNames) row(e hostchange.PathEntry) output.Row {
	return output.Row{"name": e.Name, "kind": e.Kind, "size": e.Size, "owner": n.user(e.UID), "group": n.group(e.GID),
		"mode": modeText(e.Mode), "modified": e.ModTime.Format(time.RFC3339), "target": e.Target}
}
