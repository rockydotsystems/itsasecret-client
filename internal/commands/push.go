package commands

import (
	"fmt"

	"itsasecret.dev/cli/internal/config"

	"github.com/spf13/cobra"
)

func newPushCmd() *cobra.Command {
	var (
		project string
		env     string
		file    string
	)
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push local .env to a remote environment",
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
			if file == "" {
				file = ".env"
			}
			// TODO: parse file, classify vars vs secrets, push to API
			return fmt.Errorf("not yet implemented")
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project ID (required)")
	cmd.Flags().StringVar(&env, "env", "", "environment name (default: production)")
	cmd.Flags().StringVar(&file, "file", "", "input file (default: .env)")
	return cmd
}
