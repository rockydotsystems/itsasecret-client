package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"itsasecret.dev/cli/internal/api"
	"itsasecret.dev/cli/internal/localcfg"

	"github.com/spf13/cobra"
)

// shellQuote single-quotes a value so it survives `source`/`eval` and direnv's
// dotenv parser intact, including spaces, $, and quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func newPullCmd() *cobra.Command {
	var (
		scope     scopeFlags
		shellMode bool
		outFile   string
	)
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull env vars & secrets into a file or shell",
		RunE: func(cmd *cobra.Command, args []string) error {
			rs, client, err := scope.resolveClient(cmd)
			if err != nil {
				return err
			}

			pc := localcfg.PullConfig{Mode: localcfg.PullModeShell}
			if !shellMode {
				path := outFile
				if path == "" {
					path = ".env"
				}
				pc = localcfg.PullConfig{Mode: localcfg.PullModeFile, Out: path}
			}
			if err := runPull(cmd.Context(), client, rs.project, rs.env, pc, cmd.OutOrStdout()); err != nil {
				return err
			}
			recordPull(cmd.ErrOrStderr(), rs, pc)
			return nil
		},
	}
	addScopeFlags(cmd, &scope)
	cmd.Flags().BoolVar(&shellMode, "shell", false, "emit exports to stdout for direnv/.envrc")
	cmd.Flags().StringVar(&outFile, "out", "", "output file (default: .env)")
	return cmd
}

// runPull fetches the environment's values and delivers them per pc: export
// lines on out for PullModeShell, written to pc.Out for PullModeFile.
func runPull(ctx context.Context, client *api.Client, project, env string, pc localcfg.PullConfig, out io.Writer) error {
	vars, err := client.Pull(ctx, project, env)
	if err != nil {
		return err
	}

	if pc.Mode == localcfg.PullModeShell {
		return writeExports(out, vars)
	}

	f, err := os.Create(pc.Out)
	if err != nil {
		return err
	}
	if err := writeExports(f, vars); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	say(out, "Wrote %s\n", pc.Out)
	return nil
}

func writeExports(w io.Writer, vars map[string]string) error {
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, err := fmt.Fprintf(w, "export %s=%s\n", k, shellQuote(vars[k])); err != nil {
			return err
		}
	}
	return nil
}

// recordPull remembers the delivery mode in the resolved .shh.project so
// `shh reload` can repeat it. Only a pull of the linked scope is recorded —
// a one-off --project/--env override describes a different pull than the one
// `shh reload` (which always targets the linked project and env) would
// repeat. Best-effort: a pull that worked shouldn't fail because the marker
// file can't be written.
func recordPull(errOut io.Writer, rs *resolvedScope, pc localcfg.PullConfig) {
	if rs.files.ProjectPath == "" || rs.project != rs.files.Project {
		return
	}
	linkedEnv := rs.files.Env
	if linkedEnv == "" {
		linkedEnv = "production"
	}
	if rs.env != linkedEnv {
		return
	}
	if pc.Mode == localcfg.PullModeFile {
		pc.Out = pathRelativeToMarker(rs.files.ProjectPath, pc.Out)
	}
	if err := localcfg.SavePull(rs.files.ProjectPath, pc); err != nil {
		say(errOut, "warning: could not record pull mode in %s: %v\n", rs.files.ProjectPath, err)
	}
}

// pathRelativeToMarker rewrites out relative to the .shh.project directory,
// so the recorded path means the same thing wherever reload later runs.
// Absolute paths (given or as fallback) are kept as-is.
func pathRelativeToMarker(markerPath, out string) string {
	abs, err := filepath.Abs(out)
	if err != nil {
		return out
	}
	if filepath.IsAbs(out) {
		return out
	}
	rel, err := filepath.Rel(filepath.Dir(markerPath), abs)
	if err != nil {
		return abs
	}
	return rel
}
