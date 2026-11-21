package cli

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/tools"
)

// The tool table cannot import this package, so the local read functions register here. Each one runs the code
// of its command line and answers what the command prints with --output json. Admin and root checks stay the
// same as on the command line.
func init() {
	tools.Register("host.users.list", func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		if err := hostEnv.requireAdmin(); err != nil {
			return nil, err
		}
		r, err := usersResult(output.FormatJSON)
		return encodeResult(r, err)
	})
	tools.Register("host.users.show", func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		var a struct{ Name string }
		if err := parseArgs(args, &a); err != nil {
			return nil, err
		}
		if err := hostEnv.requireAdmin(); err != nil {
			return nil, err
		}
		r, err := userResult(output.FormatJSON, a.Name)
		return encodeResult(r, err)
	})
	tools.Register("host.groups.list", func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		if err := hostEnv.requireAdmin(); err != nil {
			return nil, err
		}
		r, err := groupsResult(output.FormatJSON)
		return encodeResult(r, err)
	})
	tools.Register("host.groups.show", func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		var a struct{ Name string }
		if err := parseArgs(args, &a); err != nil {
			return nil, err
		}
		if err := hostEnv.requireAdmin(); err != nil {
			return nil, err
		}
		r, err := groupResult(output.FormatJSON, a.Name)
		return encodeResult(r, err)
	})
	tools.Register("host.folder.show", func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		var a struct{ Path string }
		if err := parseArgs(args, &a); err != nil {
			return nil, err
		}
		if err := hostEnv.requireAdmin(); err != nil {
			return nil, err
		}
		r, err := folderResult(output.FormatJSON, a.Path)
		return encodeResult(r, err)
	})
	tools.Register("host.file.read", func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		var a struct{ Path string }
		if err := parseArgs(args, &a); err != nil {
			return nil, err
		}
		if err := hostEnv.requireAdmin(); err != nil {
			return nil, err
		}
		r, err := fileResult(a.Path) // requires root
		return encodeResult(r, err)
	})
	tools.Register("host.diagnose", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		var a struct{ Out string }
		if err := parseArgs(args, &a); err != nil {
			return nil, err
		}
		r, err := diagnoseResult(ctx, output.FormatJSON, a.Out)
		return encodeResult(r, err)
	})
	tools.Register("host.update.status", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		r, err := updateStatusResult(ctx)
		return encodeResult(r, err)
	})
}

func parseArgs(args json.RawMessage, into any) error {
	return json.Unmarshal(args, into)
}

// encodeResult renders a result as the command line does with --output json, without the final newline.
func encodeResult(r output.Result, err error) (json.RawMessage, error) {
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := output.Encode(&buf, output.FormatJSON, r); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(buf.Bytes()), nil
}
