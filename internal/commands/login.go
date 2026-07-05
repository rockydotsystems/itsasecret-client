package commands

import (
	"fmt"
	"os"

	"itsasecret.dev/cli/internal/auth"
	"itsasecret.dev/cli/internal/config"
	"itsasecret.dev/cli/internal/localcfg"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

func newLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate and start a rolling session for this server",
		Long: `Authenticate and start a rolling session for this server.

The server comes from a ` + "`url =`" + ` line in ` + localcfg.ProjectFile + ` (if the current
directory tree has one) or the machine-global config — change either with
` + "`shh config`" + `. Sessions are per-server and roll: every successful command
refreshes the session, and after ~30 idle minutes the next command asks for
your master password again.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			files, err := localcfg.Find(cwd)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			apiURL := cfg.APIURL
			if files.URL != "" {
				apiURL = files.URL
				say(out, "Logging in to %s (from %s)\n", apiURL, files.ProjectPath)
			} else {
				say(out, "Logging in to %s\n", apiURL)
			}

			// Prefill the email from a previous session on this server.
			stored, _ := cfg.Session(apiURL)
			email := stored.Email
			field := huh.NewInput().Title("Email").Value(&email).Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("enter the account email")
				}
				return nil
			})
			if err := runField(cmd.Context(), cmd.InOrStdin(), out, field); err != nil {
				return err
			}

			session, email, err := promptLogin(cmd.Context(), cmd.InOrStdin(), out, apiURL, email)
			if err != nil {
				return err
			}
			if err := auth.SaveSession(cfg, apiURL, email, session); err != nil {
				return fmt.Errorf("saving session: %w", err)
			}
			sayln(out, "Logged in.")
			return nil
		},
	}
	return cmd
}
