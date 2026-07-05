package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeConfigFile(t *testing.T, content string) {
	t.Helper()
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	dir := filepath.Join(cfgHome, "itsasecret")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMigratesLegacySession(t *testing.T) {
	writeConfigFile(t, `{
		"apiUrl": "http://localhost:3000",
		"sessionToken": "legacy-token",
		"sessionKey": "a2V5",
		"orgKeys": {"org1": "b2s"}
	}`)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	s, ok := cfg.Session("http://localhost:3000")
	if !ok || s.Token != "legacy-token" || s.SessionKey != "a2V5" || s.OrgKeys["org1"] != "b2s" {
		t.Errorf("migrated session = %+v (found %v), want the legacy values under the configured URL", s, ok)
	}
	if cfg.LegacySessionToken != "" {
		t.Error("legacy fields should be cleared after migration")
	}

	// Saving persists the migrated shape without the legacy fields.
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "itsasecret", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, has := raw["sessionToken"]; has {
		t.Error("saved config still contains the legacy sessionToken field")
	}
	if _, has := raw["sessions"]; !has {
		t.Error("saved config is missing the sessions map")
	}
}

func TestSessionsPerServerCoexist(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.SetSession("https://itsasecret.dev", StoredSession{Token: "prod-token"})
	cfg.SetSession("http://localhost:3000", StoredSession{Token: "local-token"})
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	prod, _ := cfg.Session("https://itsasecret.dev")
	local, _ := cfg.Session("http://localhost:3000")
	if prod.Token != "prod-token" || local.Token != "local-token" {
		t.Errorf("sessions = prod %q / local %q, want both logins kept", prod.Token, local.Token)
	}
}

func TestSessionLookupNormalizesURL(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.SetSession("https://itsasecret.dev/", StoredSession{Token: "tok"})
	if s, ok := cfg.Session("https://itsasecret.dev"); !ok || s.Token != "tok" {
		t.Errorf("lookup without trailing slash failed: %+v (found %v)", s, ok)
	}
	if s, ok := cfg.Session(" https://itsasecret.dev/ "); !ok || s.Token != "tok" {
		t.Errorf("lookup with whitespace/slash failed: %+v (found %v)", s, ok)
	}
}

func TestLoadMigrationDoesNotClobberExistingSession(t *testing.T) {
	writeConfigFile(t, `{
		"apiUrl": "http://localhost:3000",
		"sessionToken": "stale-legacy",
		"sessions": {"http://localhost:3000": {"token": "newer-token"}}
	}`)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := cfg.Session("http://localhost:3000"); s.Token != "newer-token" {
		t.Errorf("token = %q, want the sessions entry to win over legacy fields", s.Token)
	}
}
