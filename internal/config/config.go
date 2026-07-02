package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	APIURL       string `json:"apiUrl"`
	SessionToken string `json:"sessionToken"`
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
