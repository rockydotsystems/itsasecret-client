package auth

import (
	"encoding/base64"
	"errors"
	"fmt"

	"itsasecret.dev/cli/internal/config"
)

func SaveSession(cfg *config.Config, session *Session) error {
	cfg.SessionToken = session.Token
	cfg.SessionKey = base64.RawURLEncoding.EncodeToString(session.SessionKey)
	orgKeys := make(map[string]string, len(session.OrgKeys))
	for orgID, key := range session.OrgKeys {
		orgKeys[orgID] = base64.RawURLEncoding.EncodeToString(key)
	}
	cfg.OrgKeys = orgKeys
	return cfg.Save()
}

func LoadSession() (*Session, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if cfg.SessionToken == "" {
		return nil, errors.New("not logged in — run `itsasecret login` first")
	}
	session := &Session{
		Token:   cfg.SessionToken,
		OrgKeys: make(map[string][]byte),
	}
	if cfg.SessionKey != "" {
		key, err := base64.RawURLEncoding.DecodeString(cfg.SessionKey)
		if err != nil {
			return nil, fmt.Errorf("decode session key: %w", err)
		}
		session.SessionKey = key
	}
	for orgID, encoded := range cfg.OrgKeys {
		key, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("decode org key %s: %w", orgID, err)
		}
		session.OrgKeys[orgID] = key
	}
	return session, nil
}

func LoadSessionConfig() (*config.Config, *Session, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	if cfg.SessionToken == "" {
		return nil, nil, errors.New("not logged in — run `itsasecret login` first")
	}
	session := &Session{
		Token:   cfg.SessionToken,
		OrgKeys: make(map[string][]byte),
	}
	if cfg.SessionKey != "" {
		key, err := base64.RawURLEncoding.DecodeString(cfg.SessionKey)
		if err != nil {
			return nil, nil, fmt.Errorf("decode session key: %w", err)
		}
		session.SessionKey = key
	}
	for orgID, encoded := range cfg.OrgKeys {
		key, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			return nil, nil, fmt.Errorf("decode org key %s: %w", orgID, err)
		}
		session.OrgKeys[orgID] = key
	}
	return cfg, session, nil
}
