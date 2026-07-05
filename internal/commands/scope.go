package commands

import (
	"fmt"
	"os"

	"itsasecret.dev/cli/internal/api"
	"itsasecret.dev/cli/internal/config"
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

// resolvedScope is the effective project/env plus the .shh.* files they were
// resolved against, for commands that also read or record state there.
type resolvedScope struct {
	project string
	env     string
	// files is what localcfg.Find discovered; zero-valued when both values
	// came from flags and the files were unreadable or absent.
	files *localcfg.Scope
}

func (s *scopeFlags) resolveScope() (*resolvedScope, error) {
	rs := &resolvedScope{project: s.project, env: s.env, files: &localcfg.Scope{}}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	found, err := localcfg.Find(cwd)
	if err != nil {
		// A broken marker file only matters if we need it: with both values
		// given as flags the command can still run.
		if s.project == "" || s.env == "" {
			return nil, err
		}
	} else {
		rs.files = found
	}
	if rs.project == "" {
		rs.project = rs.files.Project
	}
	if rs.env == "" {
		rs.env = rs.files.Env
	}
	if rs.project == "" {
		return nil, fmt.Errorf("project not set: pass --project <id> or run `shh link --project <id>` to write %s", localcfg.ProjectFile)
	}
	if rs.env == "" {
		rs.env = "production"
	}
	return rs, nil
}

// apiURL returns the API base URL for this scope: a `url =` override in the
// resolved .shh.project wins over the machine-global config.
func (rs *resolvedScope) apiURL(cfg *config.Config) string {
	if rs.files.URL != "" {
		return rs.files.URL
	}
	return cfg.APIURL
}

// resolveClient resolves the scope and the effective server URL, ensures a
// live session for it (prompting for the master password when the rolling
// session has idled out), and returns a ready API client that persists
// rolled tokens - the preamble shared by every authenticated,
// environment-scoped command.
func (s *scopeFlags) resolveClient(cmd *cobra.Command) (*resolvedScope, *api.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	rs, err := s.resolveScope()
	if err != nil {
		return nil, nil, err
	}
	apiURL := rs.apiURL(cfg)
	session, err := ensureSession(cmd.Context(), cmd, cfg, apiURL)
	if err != nil {
		return nil, nil, err
	}
	return rs, authedClient(cmd, cfg, apiURL, session), nil
}
