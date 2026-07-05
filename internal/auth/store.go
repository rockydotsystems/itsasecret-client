package auth

import (
	"encoding/base64"
	"fmt"

	"itsasecret.dev/cli/internal/config"
)

// SaveSession stores the session for the server it was created against.
// Sessions are per-server, so logging in to one server doesn't disturb
// sessions on others. Only master-password-wrapped org keys are persisted —
// never unwrapped key material.
func SaveSession(cfg *config.Config, apiURL, email string, session *Session) error {
	cfg.SetSession(apiURL, config.StoredSession{
		Token:          session.Token,
		ExpiresAt:      session.ExpiresAt,
		Email:          email,
		SessionKey:     base64.RawURLEncoding.EncodeToString(session.SessionKey),
		WrappedOrgKeys: session.WrappedOrgKeys,
	})
	return cfg.Save()
}

// SessionFor returns the session stored for an API URL, decoded and ready to
// use. It does not check expiry — callers decide whether an expired session
// warrants a re-auth prompt (see StoredSession.Expired via cfg.Session).
func SessionFor(cfg *config.Config, apiURL string) (*Session, error) {
	stored, ok := cfg.Session(apiURL)
	if !ok || stored.Token == "" {
		return nil, fmt.Errorf("not logged in to %s — run `shh login`", apiURL)
	}
	session := &Session{
		Token:          stored.Token,
		ExpiresAt:      stored.ExpiresAt,
		WrappedOrgKeys: stored.WrappedOrgKeys,
	}
	if stored.SessionKey != "" {
		key, err := base64.RawURLEncoding.DecodeString(stored.SessionKey)
		if err != nil {
			return nil, fmt.Errorf("decode session key: %w", err)
		}
		session.SessionKey = key
	}
	return session, nil
}
