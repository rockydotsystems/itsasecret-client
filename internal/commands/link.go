package commands

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"itsasecret.dev/cli/internal/api"
	"itsasecret.dev/cli/internal/auth"
	"itsasecret.dev/cli/internal/localcfg"

	"github.com/spf13/cobra"
)

func newLinkCmd() *cobra.Command {
	var (
		project string
		env     string
	)
	cmd := &cobra.Command{
		Use:   "link",
		Short: "Pin this directory to a project and environment",
		Long: `Pin this directory to a project (and optionally an environment) so other
commands don't need --project/--env flags.

Writes ` + localcfg.ProjectFile + ` (the project ID — commit it) and ` + localcfg.EnvFile + `
(the environment name — kept local, added to .gitignore). Commands look for
both files in the current directory and its parents.

With no flags: when logged in, links interactively (pick a project and
environment from your orgs); otherwise shows what the current directory
resolves to.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			if project == "" && env == "" {
				cfg, session, err := auth.LoadSessionConfig()
				if err != nil {
					// Not logged in — show the current state instead.
					if err := printLinkStatus(out, cwd); err != nil {
						return err
					}
					sayln(out, "Run `shh login` to link interactively.")
					return nil
				}
				client := api.NewClient(cfg.APIURL).WithToken(session.Token)
				return interactiveLink(cmd.Context(), client, bufio.NewReader(cmd.InOrStdin()), out, cwd)
			}

			if project != "" {
				if err := linkProject(out, cwd, project); err != nil {
					return err
				}
			}
			if env != "" {
				if err := linkEnv(out, cwd, env); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project ID to write to "+localcfg.ProjectFile)
	cmd.Flags().StringVar(&env, "env", "", "environment name to write to "+localcfg.EnvFile)
	return cmd
}

// say / sayln write user-facing output, deliberately ignoring write errors —
// this is best-effort UX text, not data.
func say(out io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(out, format, args...)
}

func sayln(out io.Writer, args ...any) {
	_, _ = fmt.Fprintln(out, args...)
}

func linkProject(out io.Writer, cwd, project string) error {
	path, err := localcfg.WriteProject(cwd, project)
	if err != nil {
		return err
	}
	say(out, "Linked project %s → %s (commit this file)\n", project, path)
	return nil
}

func linkEnv(out io.Writer, cwd, env string) error {
	path, err := localcfg.WriteEnv(cwd, env)
	if err != nil {
		return err
	}
	say(out, "Linked environment %s → %s (local only)\n", env, path)
	added, err := localcfg.EnsureGitignored(cwd, localcfg.EnvFile)
	if err != nil {
		return err
	}
	if added {
		say(out, "Added %s to .gitignore\n", localcfg.EnvFile)
	}
	return nil
}

// interactiveLink walks the logged-in user through org → project → env
// selection and writes the marker files.
func interactiveLink(ctx context.Context, client *api.Client, in *bufio.Reader, out io.Writer, cwd string) error {
	scope, err := localcfg.Find(cwd)
	if err != nil {
		return err
	}
	if scope.Project != "" {
		say(out, "Currently linked to project %s (%s).\n", scope.Project, scope.ProjectPath)
	}

	orgs, err := client.ListOrgs(ctx)
	if err != nil {
		return err
	}
	if len(orgs) == 0 {
		return fmt.Errorf("this account has no orgs — create one on the website first")
	}
	orgIdx := 0
	if len(orgs) > 1 {
		names := make([]string, len(orgs))
		for i, o := range orgs {
			names[i] = o.Name
		}
		orgIdx, err = promptChoice(in, out, "Select an org:", names, false)
		if err != nil {
			return err
		}
	} else {
		say(out, "Org: %s\n", orgs[0].Name)
	}

	projects, err := client.ListProjects(ctx, orgs[orgIdx].ID)
	if err != nil {
		return err
	}
	if len(projects) == 0 {
		return fmt.Errorf("org %s has no projects — create one on the website first", orgs[orgIdx].Name)
	}
	projIdx := 0
	if len(projects) > 1 {
		names := make([]string, len(projects))
		for i, p := range projects {
			names[i] = fmt.Sprintf("%s (%s)", p.Name, p.ID)
		}
		projIdx, err = promptChoice(in, out, "Select a project:", names, false)
		if err != nil {
			return err
		}
	} else {
		say(out, "Project: %s (%s)\n", projects[0].Name, projects[0].ID)
	}
	if err := linkProject(out, cwd, projects[projIdx].ID); err != nil {
		return err
	}

	envs, err := client.ListEnvs(ctx, projects[projIdx].ID)
	if err != nil {
		return err
	}
	if len(envs) == 0 {
		sayln(out, "Project has no environments — skipping environment link.")
		return nil
	}
	names := make([]string, len(envs))
	for i, e := range envs {
		names[i] = e.Name
	}
	envIdx, err := promptChoice(in, out, "Select an environment:", names, true)
	if err != nil {
		return err
	}
	if envIdx < 0 {
		sayln(out, "Environment not linked — commands default to production.")
		return nil
	}
	return linkEnv(out, cwd, envs[envIdx].Name)
}

// promptChoice prints a numbered list and reads a 1-based selection,
// re-prompting on invalid input. With allowSkip, an empty line returns -1.
func promptChoice(in *bufio.Reader, out io.Writer, label string, options []string, allowSkip bool) (int, error) {
	sayln(out, label)
	for i, o := range options {
		say(out, "  [%d] %s\n", i+1, o)
	}
	for {
		if allowSkip {
			say(out, "Choice [1-%d, empty to skip]: ", len(options))
		} else {
			say(out, "Choice [1-%d]: ", len(options))
		}
		line, err := in.ReadString('\n')
		if err != nil && line == "" {
			return 0, fmt.Errorf("reading selection: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			if allowSkip {
				return -1, nil
			}
			continue
		}
		n, convErr := strconv.Atoi(line)
		if convErr != nil || n < 1 || n > len(options) {
			say(out, "Enter a number between 1 and %d.\n", len(options))
			if err != nil {
				// The reader is exhausted (EOF with a trailing partial line);
				// re-prompting would loop forever.
				return 0, fmt.Errorf("invalid selection %q", line)
			}
			continue
		}
		return n - 1, nil
	}
}

func printLinkStatus(out io.Writer, cwd string) error {
	scope, err := localcfg.Find(cwd)
	if err != nil {
		return err
	}
	if scope.Project == "" && scope.Env == "" {
		sayln(out, "Not linked. Run `shh link --project <id> [--env <name>]` to pin this directory.")
		return nil
	}
	if scope.Project != "" {
		say(out, "project:     %s (%s)\n", scope.Project, scope.ProjectPath)
	} else {
		sayln(out, "project:     not set — pass --project or run `shh link --project <id>`")
	}
	if scope.Env != "" {
		say(out, "environment: %s (%s)\n", scope.Env, scope.EnvPath)
	} else {
		sayln(out, "environment: production (default)")
	}
	return nil
}
