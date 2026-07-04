package commands

import (
	"fmt"

	"itsasecret.dev/cli/internal/api"
	"itsasecret.dev/cli/internal/auth"

	"github.com/spf13/cobra"
)

func newVarCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "var",
		Short: "Manage env var values (plaintext)",
	}
	cmd.AddCommand(newVarSetCmd())
	cmd.AddCommand(newVarGetCmd())
	return cmd
}

func newVarSetCmd() *cobra.Command {
	var scope scopeFlags
	cmd := &cobra.Command{
		Use:   "set <KEY=VALUE>",
		Short: "Set a plaintext env var",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value, err := splitKeyValue(args[0])
			if err != nil {
				return err
			}
			cfg, session, err := auth.LoadSessionConfig()
			if err != nil {
				return err
			}
			project, env, err := scope.resolve()
			if err != nil {
				return err
			}

			client := api.NewClient(cfg.APIURL).WithToken(session.Token).WithSessionKey(session.SessionKey)
			if err := client.SetVar(cmd.Context(), project, env, key, value); err != nil {
				return err
			}
			fmt.Printf("Set var %s\n", key)
			return nil
		},
	}
	addScopeFlags(cmd, &scope)
	return cmd
}

func newVarGetCmd() *cobra.Command {
	var scope scopeFlags
	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Get a single env var value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, session, err := auth.LoadSessionConfig()
			if err != nil {
				return err
			}
			project, env, err := scope.resolve()
			if err != nil {
				return err
			}

			client := api.NewClient(cfg.APIURL).WithToken(session.Token).WithSessionKey(session.SessionKey)
			val, err := client.GetVar(cmd.Context(), project, env, args[0])
			if err != nil {
				return err
			}
			fmt.Println(val)
			return nil
		},
	}
	addScopeFlags(cmd, &scope)
	return cmd
}
