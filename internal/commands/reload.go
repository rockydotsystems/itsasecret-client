package commands

import (
	"fmt"
	"path/filepath"
	"strings"

	"itsasecret.dev/cli/internal/localcfg"

	"github.com/spf13/cobra"
)

func newReloadCmd() *cobra.Command {
	var scope scopeFlags
	cmd := &cobra.Command{
		Use:   "reload",
		Short: "Pull again, delivered the way the last pull was",
		Long: `Pull the linked environment's vars & secrets again, delivered the same way
as the last ` + "`shh pull`" + ` here - to the same file, or as shell exports.

The delivery mode is recorded in ` + localcfg.ProjectFile + ` by ` + "`shh pull`" + `. A recorded
file path is interpreted relative to that file's directory, so reload writes
to the same place from anywhere in the tree. For a shell-mode reload, load
the output with: eval "$(shh reload)"`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rs, client, err := scope.resolveClient(cmd)
			if err != nil {
				return err
			}
			pc := rs.files.Pull
			if rs.files.ProjectPath == "" || pc == nil {
				return fmt.Errorf("nothing to reload: no pull mode recorded in %s - run `shh pull` (or `shh pull --shell`) once first", localcfg.ProjectFile)
			}
		delivery := *pc
		if delivery.Mode == localcfg.PullModeFile && !filepath.IsAbs(delivery.Out) {
			delivery.Out = filepath.Join(filepath.Dir(rs.files.ProjectPath), delivery.Out)
			markerDir, err := filepath.EvalSymlinks(filepath.Dir(rs.files.ProjectPath))
			if err != nil {
				return fmt.Errorf("invalid project directory: %w", err)
			}
			absOut, err := filepath.Abs(delivery.Out)
			if err != nil {
				return fmt.Errorf("invalid output path %q: %w", delivery.Out, err)
			}
			resolvedOut, err := filepath.EvalSymlinks(absOut)
			if err != nil {
				parent, perr := filepath.EvalSymlinks(filepath.Dir(absOut))
				if perr != nil {
					return fmt.Errorf("invalid output path %q: %w", delivery.Out, perr)
				}
				resolvedOut = filepath.Join(parent, filepath.Base(absOut))
			}
			if !isWithinDir(resolvedOut, markerDir) {
				return fmt.Errorf("refusing to write secrets outside the project directory: recorded path %q resolves to %s", pc.Out, resolvedOut)
			}
			delivery.Out = resolvedOut
		}
			return runPull(cmd.Context(), client, rs.project, rs.env, delivery, cmd.OutOrStdout())
		},
	}
	addScopeFlags(cmd, &scope)
	return cmd
}

func isWithinDir(path, dir string) bool {
	if path == dir {
		return true
	}
	return strings.HasPrefix(path, dir+string(filepath.Separator))
}
