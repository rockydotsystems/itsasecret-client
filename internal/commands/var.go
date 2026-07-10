package commands

import (
	"fmt"


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
			rs, client, err := scope.resolveClient(cmd)
			if err != nil {
				return err
			}
			project, env := rs.project, rs.env
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
			if err := validateKey(args[0]); err != nil {
				return err
			}
			rs, client, err := scope.resolveClient(cmd)
			if err != nil {
				return err
			}
			project, env := rs.project, rs.env
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
