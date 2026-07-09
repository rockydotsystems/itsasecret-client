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
	reason := fmt.Sprintf("not logged in to %s yet", apiURL)
	if ok && stored.Token != "" {
		reason = fmt.Sprintf("session for %s expired", apiURL)
	}
	return unlock(ctx, cmd, cfg, apiURL, reason)
}

// unlock prompts for the master password (on the controlling terminal when
// stdio is redirected) and performs a full re-login, which also refreshes
// org keys.
func unlock(ctx context.Context, cmd *cobra.Command, cfg *config.Config, apiURL, reason string) (*auth.Session, error) {
	in, out, cleanup, err := promptIO(cmd)
	if err != nil {
		return nil, fmt.Errorf("%s and no terminal is available to ask for the master password - run any shh command in a terminal to unlock (or `shh login`)", reason)
	}
	defer cleanup()

	say(out, "%s - enter your master password to unlock.\n", reason)
	stored, _ := cfg.Session(apiURL)
	session, email, err := promptLogin(ctx, in, out, apiURL, stored.Email)
	if err != nil {
		return nil, err
	}
	if err := auth.SaveSession(cfg, apiURL, email, session); err != nil {
		return nil, fmt.Errorf("saving session: %w", err)
	}
	say(out, "Unlocked %s.\n", apiURL)
	return session, nil
}

// authedClient is clientFor plus 401 recovery: when the server rejects the
// session mid-command (rolled token lost before it was saved, revocation,
// server-side expiry), the command prompts for the master password and the
// failed request is retried once.
func authedClient(cmd *cobra.Command, cfg *config.Config, apiURL string, session *auth.Session) *api.Client {
	return clientFor(cfg, apiURL, session).WithReauth(func(ctx context.Context) (string, []byte, error) {
		s, err := unlock(ctx, cmd, cfg, apiURL, fmt.Sprintf("session for %s was rejected by the server", apiURL))
		if err != nil {
			return "", nil, err
		}
		return s.Token, s.SessionKey, nil
	})
}

// promptIO picks where the unlock prompt talks to the user. Stdout may be
// captured (eval "$(shh pull --shell)", direnv) and stdin may be a pipe, so
// whenever either isn't a terminal the prompt goes to the controlling
// terminal (/dev/tty) directly, sudo-style. Only a genuinely headless run
// (CI, no controlling terminal) errors. Non-file readers (tests) pass
// through unchanged.
func promptIO(cmd *cobra.Command) (io.Reader, io.Writer, func(), error) {
	in, out := cmd.InOrStdin(), cmd.OutOrStdout()
	if _, ok := in.(*os.File); !ok {
		return in, out, func() {}, nil
	}
	inTTY := isTerminalReader(in)
	outTTY := false
	if f, ok := out.(*os.File); ok && term.IsTerminal(f.Fd()) {
		outTTY = true
	}
	if inTTY && outTTY {
		return in, out, func() {}, nil
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("no terminal available: %w", err)
	}
	return tty, tty, func() { _ = tty.Close() }, nil
}

// promptLogin asks for credentials (email skipped when already known) and
// performs the full login handshake, which also refreshes org keys - both
// the server-side session map and the master-wrapped local copies.
func promptLogin(ctx context.Context, in io.Reader, out io.Writer, apiURL, email string) (*auth.Session, string, error) {
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
	// tests) can only read plain lines - real non-TTY runs never get here
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
		return nil, "", fmt.Errorf("not logged in to %s - no credentials entered (run `shh login`)", apiURL)
	}
	// Last line of defense before the master password leaves the machine: the
	// apiURL may come from a committed .shh.project `url =` line, so never POST
	// credentials over plaintext http to a non-loopback host. This covers every
	// path that reaches a login (shh login, and the inline unlock prompt used by
	// link and by authedClient's 401 re-auth).
	if err := requireSecureURL(apiURL); err != nil {
		return nil, "", err
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

// clientFor builds the API client every authenticated command must use: it
// carries the token and transport key, and persists rolled tokens the moment
// they arrive - losing one would lock the session out after the grace window.
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
