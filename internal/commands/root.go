package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "itsasecret",
		Short: "Sync env vars & secrets across environments",
	}
	cmd.AddCommand(newLoginCmd())
	cmd.AddCommand(newConfigCmd())
	cmd.AddCommand(newLinkCmd())
	cmd.AddCommand(newPullCmd())
	cmd.AddCommand(newReloadCmd())
	cmd.AddCommand(newSecretCmd())
	cmd.AddCommand(newVarCmd())
	cmd.AddCommand(newForkCmd())
	return cmd
}

func splitKeyValue(arg string) (key, value string, err error) {
	idx := strings.Index(arg, "=")
	if idx <= 0 {
		return "", "", fmt.Errorf("expected KEY=VALUE, got %q", arg)
	}
	return arg[:idx], arg[idx+1:], nil
}
