package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"itsasecret.dev/cli/internal/api"
	"itsasecret.dev/cli/internal/auth"
	"itsasecret.dev/cli/internal/config"
	"itsasecret.dev/cli/internal/localcfg"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/term"
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
				files, err := localcfg.Find(cwd)
				if err != nil {
					return err
				}
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				apiURL := cfg.APIURL
				if files.URL != "" {
					apiURL = files.URL
				}
				session, err := auth.SessionFor(cfg, apiURL)
				if err != nil {
					// Not logged in to this server — show the current state.
					if err := printLinkStatus(out, cwd); err != nil {
						return err
					}
					say(out, "Log in to %s (`shh login`) to link interactively.\n", apiURL)
					return nil
				}
				client := api.NewClient(apiURL).WithToken(session.Token)
				return interactiveLink(cmd.Context(), client, cmd.InOrStdin(), out, cwd)
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
func interactiveLink(ctx context.Context, client *api.Client, in io.Reader, out io.Writer, cwd string) error {
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
		opts := make([]huh.Option[int], len(orgs))
		for i, o := range orgs {
			opts[i] = huh.NewOption(o.Name, i)
		}
		orgIdx, err = selectIndex(ctx, in, out, "Select an org", opts)
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
		opts := make([]huh.Option[int], len(projects))
		for i, p := range projects {
			opts[i] = huh.NewOption(fmt.Sprintf("%s (%s)", p.Name, p.ID), i)
		}
		projIdx, err = selectIndex(ctx, in, out, "Select a project", opts)
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
	opts := make([]huh.Option[int], 0, len(envs)+1)
	for i, e := range envs {
		opts = append(opts, huh.NewOption(e.Name, i))
	}
	opts = append(opts, huh.NewOption("skip — don't pin an environment", -1))
	envIdx, err := selectIndex(ctx, in, out, "Select an environment", opts)
	if err != nil {
		return err
	}
	if envIdx < 0 {
		sayln(out, "Environment not linked — commands default to production.")
		return nil
	}
	return linkEnv(out, cwd, envs[envIdx].Name)
}

// runField runs a single huh field: a full TUI when the input is a terminal,
// huh's accessible line-prompt mode when it's a pipe or a test reader.
func runField(ctx context.Context, in io.Reader, out io.Writer, field huh.Field) error {
	form := huh.NewForm(huh.NewGroup(field)).WithInput(in).WithOutput(out)
	if f, ok := in.(*os.File); !ok || !term.IsTerminal(f.Fd()) {
		// Accessible mode reads one line per prompt, but buffers the reader
		// per field — byte-wise reads keep the rest of a piped script intact
		// for the next prompt.
		form = form.WithInput(oneByteReader{in}).WithAccessible(true)
	}
	if err := form.RunWithContext(ctx); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return errors.New("aborted")
		}
		return err
	}
	return nil
}

// selectIndex prompts for one of options with a huh select.
func selectIndex(ctx context.Context, in io.Reader, out io.Writer, title string, options []huh.Option[int]) (int, error) {
	var idx int
	field := huh.NewSelect[int]().Title(title).Options(options...).Value(&idx)
	if err := runField(ctx, in, out, field); err != nil {
		return 0, err
	}
	return idx, nil
}

// oneByteReader yields a single byte per Read so line-buffered consumers
// never read past the newline they stop at.
type oneByteReader struct{ r io.Reader }

func (r oneByteReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return r.r.Read(p[:1])
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
