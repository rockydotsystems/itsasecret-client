package commands

import (
	"fmt"
	"os"

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

Writes ` + localcfg.ProjectFile + ` (the project ID — commit it) and, with --env,
` + localcfg.EnvFile + ` (the environment name — kept local, added to .gitignore).
Commands look for both files in the current directory and its parents.

With no flags, shows what the current directory resolves to.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			if project == "" && env == "" {
				return printLinkStatus(cwd)
			}

			if project != "" {
				path, err := localcfg.WriteProject(cwd, project)
				if err != nil {
					return err
				}
				fmt.Printf("Linked project %s → %s (commit this file)\n", project, path)
			}
			if env != "" {
				path, err := localcfg.WriteEnv(cwd, env)
				if err != nil {
					return err
				}
				fmt.Printf("Linked environment %s → %s (local only)\n", env, path)
				added, err := localcfg.EnsureGitignored(cwd, localcfg.EnvFile)
				if err != nil {
					return err
				}
				if added {
					fmt.Printf("Added %s to .gitignore\n", localcfg.EnvFile)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project ID to write to "+localcfg.ProjectFile)
	cmd.Flags().StringVar(&env, "env", "", "environment name to write to "+localcfg.EnvFile)
	return cmd
}

func printLinkStatus(cwd string) error {
	scope, err := localcfg.Find(cwd)
	if err != nil {
		return err
	}
	if scope.Project == "" && scope.Env == "" {
		fmt.Println("Not linked. Run `shh link --project <id> [--env <name>]` to pin this directory.")
		return nil
	}
	if scope.Project != "" {
		fmt.Printf("project:     %s (%s)\n", scope.Project, scope.ProjectPath)
	} else {
		fmt.Println("project:     not set — pass --project or run `shh link --project <id>`")
	}
	if scope.Env != "" {
		fmt.Printf("environment: %s (%s)\n", scope.Env, scope.EnvPath)
	} else {
		fmt.Println("environment: production (default)")
	}
	return nil
}
