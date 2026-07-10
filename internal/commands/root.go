package commands

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// Version is stamped by the release build via
// -ldflags "-X itsasecret.dev/cli/internal/commands.Version=<rev>".
var Version = "dev"

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "itsasecret",
		Short:   "Sync env vars & secrets across environments",
		Version: Version,
	}
	cmd.AddCommand(newLoginCmd())
	cmd.AddCommand(newAuthCmd())
	cmd.AddCommand(newConfigCmd())
	cmd.AddCommand(newLinkCmd())
	cmd.AddCommand(newPullCmd())
	cmd.AddCommand(newLoadCmd())
	cmd.AddCommand(newShellInitCmd())
	cmd.AddCommand(newReloadCmd())
	cmd.AddCommand(newSecretCmd())
	cmd.AddCommand(newVarCmd())
	cmd.AddCommand(newForkCmd())
	return cmd
}

var keyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validateKey(key string) error {
	if !keyPattern.MatchString(key) {
		return fmt.Errorf("invalid key %q: must be a valid identifier (letters, digits, underscore; not starting with a digit)", key)
	}
	return nil
}

func splitKeyValue(arg string) (key, value string, err error) {
	idx := strings.Index(arg, "=")
	if idx <= 0 {
		return "", "", fmt.Errorf("expected KEY=VALUE, got %q", arg)
	}
	key = arg[:idx]
	if !keyPattern.MatchString(key) {
		return "", "", fmt.Errorf("invalid key %q: must be a valid identifier (letters, digits, underscore; not starting with a digit)", key)
	}
	return key, arg[idx+1:], nil
}
