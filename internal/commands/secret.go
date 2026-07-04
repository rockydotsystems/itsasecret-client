package commands

import (
	"fmt"

	"itsasecret.dev/cli/internal/api"
	"itsasecret.dev/cli/internal/auth"

	"github.com/spf13/cobra"
)

func newSecretCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret",
		Short: "Manage secret values (encrypted)",
	}
	cmd.AddCommand(newSecretSetCmd())
	cmd.AddCommand(newSecretGetCmd())
	cmd.AddCommand(newSecretListCmd())
	return cmd
}

func newSecretListCmd() *cobra.Command {
	var scope scopeFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List secret keys in an environment (values not shown)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, session, err := auth.LoadSessionConfig()
			if err != nil {
				return err
			}
			project, env, err := scope.resolve()
			if err != nil {
				return err
			}

			client := api.NewClient(cfg.APIURL).WithToken(session.Token)
			keys, err := client.ListSecrets(cmd.Context(), project, env)
			if err != nil {
				return err
			}
			for _, k := range keys {
				fmt.Println(k)
			}
			return nil
		},
	}
	addScopeFlags(cmd, &scope)
	return cmd
}

func newSecretSetCmd() *cobra.Command {
	var scope scopeFlags
	cmd := &cobra.Command{
		Use:   "set <KEY=VALUE>",
		Short: "Set a secret, encrypted on this machine before it syncs",
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
			if err := client.SetSecret(cmd.Context(), project, env, key, value); err != nil {
				return err
			}
			fmt.Printf("Set secret %s\n", key)
			return nil
		},
	}
	addScopeFlags(cmd, &scope)
	return cmd
}

func newSecretGetCmd() *cobra.Command {
	var scope scopeFlags
	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Get a single secret value (decrypted)",
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
			val, err := client.GetSecret(cmd.Context(), project, env, args[0])
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
