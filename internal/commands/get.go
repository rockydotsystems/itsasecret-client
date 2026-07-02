package commands

import (
	"fmt"

	"itsasecret.dev/cli/internal/api"
	"itsasecret.dev/cli/internal/auth"

	"github.com/spf13/cobra"
)

func newGetCmd() *cobra.Command {
	var (
		project string
		env     string
		secret  bool
	)
	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Get a single var or secret value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, session, err := auth.LoadSessionConfig()
			if err != nil {
				return err
			}
			key := args[0]
			if project == "" {
				return fmt.Errorf("--project is required")
			}
			if env == "" {
				env = "production"
			}

			client := api.NewClient(cfg.APIURL).WithToken(session.Token).WithSessionKey(session.SessionKey)
			ctx := cmd.Context()

			if secret {
				val, err := client.GetSecret(ctx, project, env, key)
				if err != nil {
					return err
				}
				fmt.Println(val)
				return nil
			}
			val, err := client.GetVar(ctx, project, env, key)
			if err != nil {
				return err
			}
			fmt.Println(val)
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project ID (required)")
	cmd.Flags().StringVar(&env, "env", "", "environment name (default: production)")
	cmd.Flags().BoolVar(&secret, "secret", false, "treat key as a secret (decrypt)")
	return cmd
}
