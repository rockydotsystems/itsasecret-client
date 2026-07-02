package commands

import (
	"fmt"

	"itsasecret.dev/cli/internal/api"
	"itsasecret.dev/cli/internal/config"

	"github.com/spf13/cobra"
)

func newLoginCmd() *cobra.Command {
	var apiURL string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate and store a session token locally",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if apiURL != "" {
				cfg.APIURL = apiURL
			}
			fmt.Printf("Logging in to %s\n", cfg.APIURL)
			fmt.Print("Email: ")
			var email string
			fmt.Scanln(&email)
			fmt.Print("Password: ")
			var password string
			fmt.Scanln(&password)

			client := api.NewClient(cfg.APIURL)
			resp, err := client.Login(cmd.Context(), email, password)
			if err != nil {
				return fmt.Errorf("login failed: %w", err)
			}
			cfg.SessionToken = resp.Token
			if err := cfg.Save(); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}
			fmt.Println("Logged in.")
			return nil
		},
	}
	cmd.Flags().StringVar(&apiURL, "api", "", "API URL (default: https://itsasecret.dev)")
	return cmd
}
