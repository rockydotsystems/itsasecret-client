package commands

import (
	"fmt"

	"itsasecret.dev/cli/internal/api"
	"itsasecret.dev/cli/internal/auth"

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
			key := args[0]
			value := args[1]

			client := api.NewClient(cfg.APIURL).WithToken(session.Token).WithSessionKey(session.SessionKey)
			ctx := cmd.Context()

			if secret {
				if err := client.SetSecret(ctx, project, env, key, value); err != nil {
					return err
				}
				fmt.Printf("Set secret %s\n", key)
			} else {
				if err := client.SetVar(ctx, project, env, key, value); err != nil {
					return err
				}
				fmt.Printf("Set var %s\n", key)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project ID (required)")
	cmd.Flags().StringVar(&env, "env", "", "environment name (default: production)")
	cmd.Flags().BoolVar(&secret, "secret", false, "treat value as a secret (encrypted)")
	return cmd
}
