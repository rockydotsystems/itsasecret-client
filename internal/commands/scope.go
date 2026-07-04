package commands

import (
	"fmt"
	"os"

	"itsasecret.dev/cli/internal/localcfg"

	"github.com/spf13/cobra"
)

// scopeFlags holds the --project/--env values shared by every command that
// targets an environment. Resolution order: flag > .shh.* file (found by
// walking up from cwd) > default ("production" for env).
type scopeFlags struct {
	project string
	env     string
}

func addScopeFlags(cmd *cobra.Command, s *scopeFlags) {
	cmd.Flags().StringVar(&s.project, "project", "", "project ID (overrides "+localcfg.ProjectFile+")")
	cmd.Flags().StringVar(&s.env, "env", "", "environment name (overrides "+localcfg.EnvFile+"; default: production)")
}

func (s *scopeFlags) resolve() (project, env string, err error) {
	project, env = s.project, s.env
	if project == "" || env == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", "", err
		}
		found, err := localcfg.Find(cwd)
		if err != nil {
			return "", "", err
		}
		if project == "" {
			project = found.Project
		}
		if env == "" {
			env = found.Env
		}
	}
	if project == "" {
		return "", "", fmt.Errorf("project not set: pass --project <id> or run `shh link --project <id>` to write %s", localcfg.ProjectFile)
	}
	if env == "" {
		env = "production"
	}
	return project, env, nil
}
