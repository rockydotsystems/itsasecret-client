package commands

import (
	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "itsasecret",
		Short: "Sync env vars & secrets across environments",
	}
	cmd.AddCommand(newLoginCmd())
	cmd.AddCommand(newPullCmd())
	cmd.AddCommand(newPushCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newSetCmd())
	cmd.AddCommand(newForkCmd())
	return cmd
}
