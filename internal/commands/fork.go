package commands

import (
	"fmt"

	"itsasecret.dev/cli/internal/config"

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
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.SessionToken == "" {
				return fmt.Errorf("not logged in — run `itsasecret login` first")
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
			// TODO: call API fork endpoint
			return fmt.Errorf("not yet implemented")
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project ID (required)")
	cmd.Flags().StringVar(&env, "env", "", "source environment name (default: production)")
	cmd.Flags().StringVar(&name, "name", "", "new fork name (required)")
	return cmd
}
