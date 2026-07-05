package commands

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"

	"itsasecret.dev/cli/internal/api"
	"itsasecret.dev/cli/internal/auth"
	"itsasecret.dev/cli/internal/config"
	"itsasecret.dev/cli/internal/localcfg"

	"github.com/spf13/cobra"
)

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth <token>",
		Short: "Authenticate with a long-lived access token (for headless machines)",
		Long: `Authenticate with a long-lived access token instead of a master password.

Create the token on the website (Tokens, in the dashboard) and paste it here
once. Made for headless machines - CI runners, servers, containers: unlike
` + "`shh login`" + `, no master password is needed and the session neither rolls nor
idles out. It lasts until the expiry picked at creation (or forever) or until
the token is revoked on the website.

The server comes from a ` + "`url =`" + ` line in ` + localcfg.ProjectFile + ` (if the current
directory tree has one) or the machine-global config, same as ` + "`shh login`" + `.`,
		Args: cobra.ExactArgs(1),
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
				say(out, "Authenticating to %s (from %s)\n", apiURL, files.ProjectPath)
			}

			bearer, sessionKey, err := auth.ParseAccessToken(args[0])
			if err != nil {
				return err
			}

			// Validate the token against the server before persisting anything;
			// this also tells us who it belongs to and when it expires.
			client := api.NewClient(apiURL).WithToken(bearer).WithSessionKey(sessionKey)
			details, err := client.MeDetails(cmd.Context())
			if errors.Is(err, api.ErrUnauthorized) {
				return fmt.Errorf("token rejected by %s - it may be expired or revoked", apiURL)
			}
			if err != nil {
				return fmt.Errorf("verifying token with %s: %w", apiURL, err)
			}

			cfg.SetSession(apiURL, config.StoredSession{
				Token:      bearer,
				ExpiresAt:  details.SessionExpiresAt,
				Email:      details.Email,
				SessionKey: base64.RawURLEncoding.EncodeToString(sessionKey),
			})
			if err := cfg.Save(); err != nil {
				return fmt.Errorf("saving session: %w", err)
			}

			// "Does not expire" tokens carry a far-future sentinel expiry.
			expiry := "does not expire"
			if details.SessionExpiresAt.Year() < 9000 {
				expiry = "expires " + details.SessionExpiresAt.Local().Format("2006-01-02")
			}
			say(out, "Authenticated to %s as %s (token %s).\n", apiURL, details.Email, expiry)
			return nil
		},
	}
	return cmd
}
