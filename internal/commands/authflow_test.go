package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"itsasecret.dev/cli/internal/config"
	"itsasecret.dev/cli/internal/localcfg"
)

func linkedDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := localcfg.WriteProject(dir, "proj1"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return dir
}

func storedSession(t *testing.T, apiURL string) config.StoredSession {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	s, ok := cfg.Session(apiURL)
	if !ok {
		t.Fatalf("no session stored for %s", apiURL)
	}
	return s
}

func TestRolledTokenIsPersisted(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, true)
	linkedDir(t)

	// The fake pull handler rotates the token on success.
	out, err := runCmd(t, "", "pull", "--shell")
	if err != nil {
		t.Fatalf("pull failed: %v\noutput:\n%s", err, out)
	}
	s := storedSession(t, srv.URL)
	if s.Token != "rotated-token" {
		t.Errorf("stored token = %q, want the rolled token persisted", s.Token)
	}
	if s.Expired() {
		t.Error("rolled session should carry a fresh future expiry")
	}
}

func TestExpiredSessionPromptsForMasterPassword(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, false)
	// A stale session: token present, expiry in the past, email known.
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.SetSession(srv.URL, config.StoredSession{
		Token:     "stale-token",
		Email:     "you@example.com",
		ExpiresAt: time.Now().Add(-time.Hour),
	})
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	linkedDir(t)

	// Email is remembered, so only the master password is asked; the command
	// proceeds after the unlock.
	out, err := runCmd(t, "hunter2\n", "pull", "--shell")
	if err != nil {
		t.Fatalf("pull failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "expired - enter your master password") {
		t.Errorf("expected an unlock prompt, got:\n%s", out)
	}
	if !strings.Contains(out, "Unlocked "+srv.URL) {
		t.Errorf("expected unlock confirmation, got:\n%s", out)
	}
	if !strings.Contains(out, "export FOO='bar'") {
		t.Errorf("expected the command to proceed after unlock, got:\n%s", out)
	}
	s := storedSession(t, srv.URL)
	// The pull that followed the unlock rolled the fresh login token again.
	if s.Token != "rotated-token" || s.Expired() {
		t.Errorf("re-auth + rolling should store a fresh session, got %+v", s)
	}
	if s.WrappedOrgKeys["org1"] != "master-wrapped-blob" {
		t.Errorf("re-auth should refresh master-wrapped org keys, got %+v", s.WrappedOrgKeys)
	}
}

func TestRejectedTokenPromptsAndRetries(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, false)
	// The token looks fresh locally (future expiry) but the server rejects
	// it - e.g. a rolled token that was never saved, past its grace window.
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.SetSession(srv.URL, config.StoredSession{
		Token:     "clobbered-token",
		Email:     "you@example.com",
		ExpiresAt: time.Now().Add(20 * time.Minute),
	})
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	linkedDir(t)

	out, err := runCmd(t, "hunter2\n", "pull", "--shell")
	if err != nil {
		t.Fatalf("pull failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "rejected by the server - enter your master password") {
		t.Errorf("expected a rejected-session unlock prompt, got:\n%s", out)
	}
	if !strings.Contains(out, "export FOO='bar'") {
		t.Errorf("expected the retried pull to succeed, got:\n%s", out)
	}
	s := storedSession(t, srv.URL)
	if s.Token != "rotated-token" {
		t.Errorf("stored token = %q, want the retried pull's rolled token", s.Token)
	}
}

func TestExpiredSessionWrongPasswordFails(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, false)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.SetSession(srv.URL, config.StoredSession{
		Token:     "stale-token",
		Email:     "you@example.com",
		ExpiresAt: time.Now().Add(-time.Hour),
	})
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	linkedDir(t)

	_, err = runCmd(t, "wrong-password\n", "secret", "list")
	if err == nil || !strings.Contains(err.Error(), "login failed") {
		t.Errorf("err = %v, want a login failure", err)
	}
}

func TestLoginCommandStoresRollingSession(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, false)
	t.Chdir(t.TempDir())

	out, err := runCmd(t, "you@example.com\nhunter2\n", "login")
	if err != nil {
		t.Fatalf("login failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "Logged in.") {
		t.Errorf("expected login confirmation, got:\n%s", out)
	}
	s := storedSession(t, srv.URL)
	if s.Token != "test-token" || s.Email != "you@example.com" || s.Expired() {
		t.Errorf("stored session = %+v, want fresh rolling session with email", s)
	}
	if s.WrappedOrgKeys["org1"] != "master-wrapped-blob" {
		t.Errorf("wrappedOrgKeys = %+v, want master-wrapped blobs persisted", s.WrappedOrgKeys)
	}
	if s.SessionKey == "" {
		t.Error("session key missing from stored session")
	}
}

func TestConfigNeverStoresPlaintextOrgKeys(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, false)
	t.Chdir(t.TempDir())

	if out, err := runCmd(t, "you@example.com\nhunter2\n", "login"); err != nil {
		t.Fatalf("login failed: %v\noutput:\n%s", err, out)
	}
	data := readFileOrEmpty(t, filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "itsasecret", "config.json"))
	if strings.Contains(data, `"orgKeys"`) {
		t.Errorf("config.json contains a plaintext orgKeys field:\n%s", data)
	}
	if !strings.Contains(data, `"wrappedOrgKeys"`) {
		t.Errorf("config.json missing wrappedOrgKeys:\n%s", data)
	}
}
