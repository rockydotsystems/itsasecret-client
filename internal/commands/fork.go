package commands

import (
	"fmt"

	"itsasecret.dev/cli/internal/api"
	"itsasecret.dev/cli/internal/auth"

	"github.com/spf13/cobra"
)

func newForkCmd() *cobra.Command {
	var (
		project string
		env     string
		name    string
	)
	cmd := &cobra.Command{
		Use:   "fork",
		Short: "Fork an environment (e.g. production → staging)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, session, err := auth.LoadSessionConfig()
			if err != nil {
				return err
			}
			if project == "" {
				return fmt.Errorf("--project is required")
			}
			if env == "" {
				env = "production"
			}
			if name == "" {
				return fmt.Errorf("--name is required (new environment name)")
			}

			client := api.NewClient(cfg.APIURL).WithToken(session.Token)
			if err := client.ForkEnv(cmd.Context(), project, env, name); err != nil {
				return err
			}
			fmt.Printf("Forked %s/%s → %s\n", project, env, name)
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project ID (required)")
	cmd.Flags().StringVar(&env, "env", "", "source environment name (default: production)")
	cmd.Flags().StringVar(&name, "name", "", "new fork name (required)")
	return cmd
}
