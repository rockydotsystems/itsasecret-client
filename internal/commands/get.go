package commands

import (
	"fmt"

	"itsasecret.dev/cli/internal/config"

	"github.com/spf13/cobra"
)

func newGetCmd() *cobra.Command {
	var (
		project string
		env     string
	)
	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Get a single var or secret value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.SessionToken == "" {
				return fmt.Errorf("not logged in — run `itsasecret login` first")
			}
		key := args[0]
		if project == "" {
			return fmt.Errorf("--project is required")
		}
		if env == "" {
			env = "production"
		}
		// TODO: fetch from API
		return fmt.Errorf("not yet implemented: get %s", key)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project ID (required)")
	cmd.Flags().StringVar(&env, "env", "", "environment name (default: production)")
	return cmd
}
