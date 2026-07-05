package commands

import (
	"fmt"


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
			rs, client, err := scope.resolveClient(cmd)
			if err != nil {
				return err
			}
			project, env := rs.project, rs.env
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
			rs, client, err := scope.resolveClient(cmd)
			if err != nil {
				return err
			}
			project, env := rs.project, rs.env
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
			rs, client, err := scope.resolveClient(cmd)
			if err != nil {
				return err
			}
			project, env := rs.project, rs.env
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
