package auth

import (
	"encoding/base64"
	"fmt"

	"itsasecret.dev/cli/internal/config"
)

// SaveSession stores the session for the server it was created against.
// Sessions are per-server, so logging in to one server doesn't disturb
// sessions on others.
func SaveSession(cfg *config.Config, apiURL string, session *Session) error {
	orgKeys := make(map[string]string, len(session.OrgKeys))
	for orgID, key := range session.OrgKeys {
		orgKeys[orgID] = base64.RawURLEncoding.EncodeToString(key)
	}
	cfg.SetSession(apiURL, config.StoredSession{
		Token:      session.Token,
		SessionKey: base64.RawURLEncoding.EncodeToString(session.SessionKey),
		OrgKeys:    orgKeys,
	})
	return cfg.Save()
}

// SessionFor returns the session for an API URL, decoded and ready to use.
func SessionFor(cfg *config.Config, apiURL string) (*Session, error) {
	stored, ok := cfg.Session(apiURL)
	if !ok || stored.Token == "" {
		return nil, fmt.Errorf("not logged in to %s — run `shh login`", apiURL)
	}
	session := &Session{
		Token:   stored.Token,
		OrgKeys: make(map[string][]byte, len(stored.OrgKeys)),
	}
	if stored.SessionKey != "" {
		key, err := base64.RawURLEncoding.DecodeString(stored.SessionKey)
		if err != nil {
			return nil, fmt.Errorf("decode session key: %w", err)
		}
		session.SessionKey = key
	}
	for orgID, encoded := range stored.OrgKeys {
		key, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("decode org key %s: %w", orgID, err)
		}
		session.OrgKeys[orgID] = key
	}
	return session, nil
}
