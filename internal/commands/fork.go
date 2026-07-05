package commands

import (
	"fmt"

	"itsasecret.dev/cli/internal/api"
	"itsasecret.dev/cli/internal/auth"

	"github.com/spf13/cobra"
)

func newForkCmd() *cobra.Command {
	var (
		scope scopeFlags
		name  string
	)
	cmd := &cobra.Command{
		Use:   "fork",
		Short: "Fork an environment (e.g. production → staging)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, session, err := auth.LoadSessionConfig()
			if err != nil {
				return err
			}
			rs, err := scope.resolveScope()
			if err != nil {
				return err
			}
			project, env := rs.project, rs.env
			if name == "" {
				return fmt.Errorf("--name is required (new environment name)")
			}

			client := api.NewClient(rs.apiURL(cfg)).WithToken(session.Token)
			if err := client.ForkEnv(cmd.Context(), project, env, name); err != nil {
				return err
			}
			fmt.Printf("Forked %s/%s → %s\n", project, env, name)
			return nil
		},
	}
	addScopeFlags(cmd, &scope)
	cmd.Flags().StringVar(&name, "name", "", "new fork name (required)")
	return cmd
}
