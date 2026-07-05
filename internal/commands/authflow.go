package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"itsasecret.dev/cli/internal/api"
	"itsasecret.dev/cli/internal/auth"
	"itsasecret.dev/cli/internal/config"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

// ensureSession returns a live session for apiURL, re-authenticating inline
// (master-password prompt) when the stored one is expired or absent. Sessions
// roll on every successful command and expire after ~30 idle minutes, so this
// prompt is the "unlock" moment; while you're active it never fires.
func ensureSession(ctx context.Context, cmd *cobra.Command, cfg *config.Config, apiURL string) (*auth.Session, error) {
	stored, ok := cfg.Session(apiURL)
	if ok && stored.Token != "" && !stored.Expired() {
		return auth.SessionFor(cfg, apiURL)
	}

	in, out := cmd.InOrStdin(), cmd.OutOrStdout()
	if !canPrompt(in) {
		if ok && stored.Token != "" {
			return nil, fmt.Errorf("session for %s expired — run any shh command in a terminal to unlock (or `shh login`)", apiURL)
		}
		return nil, fmt.Errorf("not logged in to %s — run `shh login`", apiURL)
	}

	if ok && stored.Token != "" {
		say(out, "Session for %s expired — enter your master password to unlock.\n", apiURL)
	} else {
		say(out, "Not logged in to %s yet.\n", apiURL)
	}
	email := stored.Email
	session, email, err := promptLogin(ctx, cmd, apiURL, email)
	if err != nil {
		return nil, err
	}
	if err := auth.SaveSession(cfg, apiURL, email, session); err != nil {
		return nil, fmt.Errorf("saving session: %w", err)
	}
	say(out, "Unlocked %s.\n", apiURL)
	return session, nil
}

// promptLogin asks for credentials (email skipped when already known) and
// performs the full login handshake, which also refreshes org keys — both
// the server-side session map and the master-wrapped local copies.
func promptLogin(ctx context.Context, cmd *cobra.Command, apiURL, email string) (*auth.Session, string, error) {
	in, out := cmd.InOrStdin(), cmd.OutOrStdout()
	if email == "" {
		field := huh.NewInput().Title("Email").Value(&email).Validate(func(s string) error {
			if s == "" {
				return fmt.Errorf("enter the account email")
			}
			return nil
		})
		if err := runField(ctx, in, out, field); err != nil {
			return nil, "", err
		}
	}
	var password string
	// Masked input needs a real terminal; huh's accessible mode (pipes,
	// tests) can only read plain lines — real non-TTY runs never get here
	// (see canPrompt).
	echoMode := huh.EchoModePassword
	if !isTerminalReader(in) {
		echoMode = huh.EchoModeNormal
	}
	field := huh.NewInput().
		Title(fmt.Sprintf("Master password (%s)", email)).
		EchoMode(echoMode).
		Value(&password).
		Validate(func(s string) error {
			if s == "" {
				return fmt.Errorf("enter your master password")
			}
			return nil
		})
	if err := runField(ctx, in, out, field); err != nil {
		return nil, "", err
	}

	if email == "" || password == "" {
		return nil, "", fmt.Errorf("not logged in to %s — no credentials entered (run `shh login`)", apiURL)
	}
	session, err := auth.Login(ctx, api.NewClient(apiURL), email, password)
	if err != nil {
		return nil, "", fmt.Errorf("login failed: %w", err)
	}
	return session, email, nil
}

// isTerminalReader reports whether the reader is a real terminal.
func isTerminalReader(in io.Reader) bool {
	f, ok := in.(*os.File)
	return ok && term.IsTerminal(f.Fd())
}

// canPrompt reports whether interactive prompting is possible: a real
// terminal, or a non-stdin reader (tests). Piped stdin (direnv, scripts)
// can't prompt for a master password.
func canPrompt(in io.Reader) bool {
	if _, ok := in.(*os.File); !ok {
		return true
	}
	return isTerminalReader(in)
}

// clientFor builds the API client every authenticated command must use: it
// carries the token and transport key, and persists rolled tokens the moment
// they arrive — losing one would lock the session out after the grace window.
func clientFor(cfg *config.Config, apiURL string, session *auth.Session) *api.Client {
	return api.NewClient(apiURL).
		WithToken(session.Token).
		WithSessionKey(session.SessionKey).
		WithTokenSaver(func(token string, expiresAt time.Time) {
			stored, _ := cfg.Session(apiURL)
			stored.Token = token
			if !expiresAt.IsZero() {
				stored.ExpiresAt = expiresAt
			}
			cfg.SetSession(apiURL, stored)
			// Best-effort: if the write fails the worst case is an early
			// re-prompt for the master password.
			_ = cfg.Save()
		})
}
