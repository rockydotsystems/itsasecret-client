package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// StoredSession is one server's session as persisted in config.json. Key
// material is base64-encoded; decoding lives in the auth package.
type StoredSession struct {
	Token      string            `json:"token"`
	SessionKey string            `json:"sessionKey,omitempty"`
	OrgKeys    map[string]string `json:"orgKeys,omitempty"`
}

type Config struct {
	APIURL string `json:"apiUrl"`
	// Sessions is keyed by canonical API URL, so logins against different
	// servers (production, self-hosted, local dev) coexist.
	Sessions map[string]StoredSession `json:"sessions,omitempty"`

	// Legacy single-session fields from before per-server sessions; Load
	// migrates them into Sessions.
	LegacySessionToken string            `json:"sessionToken,omitempty"`
	LegacySessionKey   string            `json:"sessionKey,omitempty"`
	LegacyOrgKeys      map[string]string `json:"orgKeys,omitempty"`
}

// canonicalURL normalizes an API URL for use as a session key.
func canonicalURL(apiURL string) string {
	return strings.TrimRight(strings.TrimSpace(apiURL), "/")
}

// Session returns the stored session for an API URL.
func (c *Config) Session(apiURL string) (StoredSession, bool) {
	s, ok := c.Sessions[canonicalURL(apiURL)]
	return s, ok
}

// SetSession stores the session for an API URL.
func (c *Config) SetSession(apiURL string, s StoredSession) {
	if c.Sessions == nil {
		c.Sessions = make(map[string]StoredSession, 1)
	}
	c.Sessions[canonicalURL(apiURL)] = s
}

func dir() (string, error) {
	home, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "itsasecret"), nil
}

func path() (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.json"), nil
}

func Load() (*Config, error) {
	p, err := path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{APIURL: "https://itsasecret.dev"}, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.APIURL == "" {
		cfg.APIURL = "https://itsasecret.dev"
	}
	// Migrate a pre-multi-session config: the flat session belonged to the
	// configured server. Persisted by the next Save.
	if cfg.LegacySessionToken != "" {
		if _, ok := cfg.Session(cfg.APIURL); !ok {
			cfg.SetSession(cfg.APIURL, StoredSession{
				Token:      cfg.LegacySessionToken,
				SessionKey: cfg.LegacySessionKey,
				OrgKeys:    cfg.LegacyOrgKeys,
			})
		}
		cfg.LegacySessionToken, cfg.LegacySessionKey, cfg.LegacyOrgKeys = "", "", nil
	}
	return &cfg, nil
}

func (c *Config) Save() error {
	d, err := dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return err
	}
	p, err := path()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}
