package commands

import (
	"fmt"

	"itsasecret.dev/cli/internal/config"

	"github.com/spf13/cobra"
)

func newSetCmd() *cobra.Command {
	var (
		project string
		env     string
		secret  bool
	)
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a var or secret value",
		Args:  cobra.ExactArgs(2),
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
			// TODO: push to API (encrypt if secret)
			return fmt.Errorf("not yet implemented")
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project ID (required)")
	cmd.Flags().StringVar(&env, "env", "", "environment name (default: production)")
	cmd.Flags().BoolVar(&secret, "secret", false, "treat value as a secret (encrypted)")
	return cmd
}
