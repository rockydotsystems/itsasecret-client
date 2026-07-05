package commands

import (
	"fmt"
	"os"

	"itsasecret.dev/cli/internal/api"
	"itsasecret.dev/cli/internal/auth"
	"itsasecret.dev/cli/internal/config"
	"itsasecret.dev/cli/internal/localcfg"

	"github.com/spf13/cobra"
)

func newLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate and store a session token locally",
		Long: `Authenticate and store a session token locally.

The server to log in to comes from an ` + "`api =`" + ` line in ` + localcfg.ProjectFile + `
(if the current directory tree has one) or the machine-global config —
change either with ` + "`shh config`" + `.`,
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
			apiURL := cfg.APIURL
			if files.API != "" {
				apiURL = files.API
				fmt.Printf("Logging in to %s (from %s)\n", apiURL, files.ProjectPath)
			} else {
				fmt.Printf("Logging in to %s\n", apiURL)
			}
			fmt.Print("Email: ")
			var email string
			fmt.Scanln(&email)
			fmt.Print("Password: ")
			var password string
			fmt.Scanln(&password)

			client := api.NewClient(apiURL)
			session, err := auth.Login(cmd.Context(), client, email, password)
			if err != nil {
				return fmt.Errorf("login failed: %w", err)
			}
			if err := auth.SaveSession(cfg, session); err != nil {
				return fmt.Errorf("saving session: %w", err)
			}
			fmt.Println("Logged in.")
			return nil
		},
	}
	return cmd
}
