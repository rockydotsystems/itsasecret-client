package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"itsasecret.dev/cli/internal/config"
	"itsasecret.dev/cli/internal/localcfg"
)

func readGlobalAPIURL(t *testing.T) string {
	t.Helper()
	path := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "itsasecret", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		APIURL string `json:"apiUrl"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg.APIURL
}

func TestConfigSetGlobal(t *testing.T) {
	setupConfig(t, "https://old.example.com", false)
	t.Chdir(t.TempDir())

	out, err := runCmd(t, "", "config", "set", "url", "http://localhost:3000/")
	if err != nil {
		t.Fatalf("config set failed: %v\noutput:\n%s", err, out)
	}
	// Trailing slash is normalized away.
	if got := readGlobalAPIURL(t); got != "http://localhost:3000" {
		t.Errorf("global apiUrl = %q, want normalized http://localhost:3000", got)
	}
	if !strings.Contains(out, "shh login") {
		t.Errorf("expected a login hint, got:\n%s", out)
	}
}

func TestConfigSetProject(t *testing.T) {
	setupConfig(t, "https://global.example.com", false)
	dir := t.TempDir()
	if _, err := localcfg.WriteProject(dir, "proj1"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	out, err := runCmd(t, "", "config", "set", "url", "https://secrets.example.com", "--project")
	if err != nil {
		t.Fatalf("config set failed: %v\noutput:\n%s", err, out)
	}
	want := "project = proj1\nurl = https://secrets.example.com\n"
	if got := readFileOrEmpty(t, filepath.Join(dir, localcfg.ProjectFile)); got != want {
		t.Errorf("%s = %q, want %q", localcfg.ProjectFile, got, want)
	}
	if got := readGlobalAPIURL(t); got != "https://global.example.com" {
		t.Errorf("global apiUrl = %q, want it untouched", got)
	}
}

func TestConfigSetProjectWithoutMarkerErrors(t *testing.T) {
	setupConfig(t, "https://global.example.com", false)
	t.Chdir(t.TempDir())

	_, err := runCmd(t, "", "config", "set", "url", "https://x.example.com", "--project")
	if err == nil || !strings.Contains(err.Error(), "shh link") {
		t.Errorf("err = %v, want a no-marker error pointing at shh link", err)
	}
}

func TestConfigSetInvalidURLErrors(t *testing.T) {
	setupConfig(t, "https://global.example.com", false)
	t.Chdir(t.TempDir())

	for _, bad := range []string{"localhost:3000", "ftp://x", "not a url"} {
		if _, err := runCmd(t, "", "config", "set", "url", bad); err == nil {
			t.Errorf("config set url %q: expected an error", bad)
		}
	}
}

func TestConfigGetShowsOverrideSource(t *testing.T) {
	setupConfig(t, "https://global.example.com", false)
	dir := t.TempDir()
	markerPath, err := localcfg.WriteProject(dir, "proj1")
	if err != nil {
		t.Fatal(err)
	}
	if err := localcfg.SaveURL(markerPath, "https://secrets.example.com"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	out, err := runCmd(t, "", "config", "get", "url")
	if err != nil {
		t.Fatalf("config get failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "https://secrets.example.com") || !strings.Contains(out, markerPath) {
		t.Errorf("expected override value + source path, got:\n%s", out)
	}
}

func TestConfigMenuGlobal(t *testing.T) {
	setupConfig(t, "https://old.example.com", false)
	t.Chdir(t.TempDir())

	// Action 1 = set the server URL; no .shh.project → no scope question,
	// just the URL input.
	out, err := runCmd(t, "1\nhttp://localhost:3000\n", "config")
	if err != nil {
		t.Fatalf("config menu failed: %v\noutput:\n%s", err, out)
	}
	if got := readGlobalAPIURL(t); got != "http://localhost:3000" {
		t.Errorf("global apiUrl = %q, want http://localhost:3000", got)
	}
}

func TestConfigMenuProjectScope(t *testing.T) {
	setupConfig(t, "https://global.example.com", false)
	dir := t.TempDir()
	if _, err := localcfg.WriteProject(dir, "proj1"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	// Action 1 = set; scope option 2 = this project; then the URL.
	out, err := runCmd(t, "1\n2\nhttps://secrets.example.com\n", "config")
	if err != nil {
		t.Fatalf("config menu failed: %v\noutput:\n%s", err, out)
	}
	want := "project = proj1\nurl = https://secrets.example.com\n"
	if got := readFileOrEmpty(t, filepath.Join(dir, localcfg.ProjectFile)); got != want {
		t.Errorf("%s = %q, want %q", localcfg.ProjectFile, got, want)
	}
	if got := readGlobalAPIURL(t); got != "https://global.example.com" {
		t.Errorf("global apiUrl = %q, want it untouched", got)
	}
}

func TestConfigMenuShow(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, true)
	dir := t.TempDir()
	markerPath, err := localcfg.WriteProject(dir, "proj1")
	if err != nil {
		t.Fatal(err)
	}
	if err := localcfg.SaveURL(markerPath, "https://secrets.example.com"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	// Action 2 = show the current configuration; the session is verified
	// live against the server, not just assumed from the config file.
	out, err := runCmd(t, "2\n", "config")
	if err != nil {
		t.Fatalf("config menu failed: %v\noutput:\n%s", err, out)
	}
	for _, want := range []string{
		"server url: " + srv.URL + " (global)",
		"https://secrets.example.com (override from " + markerPath,
		srv.URL + " — logged in as you@example.com (session verified)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestConfigMenuShowExpiredSession(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, false)
	addSessionToken(t, srv.URL, "expired-token")
	t.Chdir(t.TempDir())

	out, err := runCmd(t, "2\n", "config")
	if err != nil {
		t.Fatalf("config menu failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, srv.URL+" — session expired — run `shh login`") {
		t.Errorf("expected an expired-session line, got:\n%s", out)
	}
}

func TestConfigSetVerifiesExistingSession(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, true)
	t.Chdir(t.TempDir())

	// Re-setting the URL of a server we're logged in to verifies the
	// session instead of telling the user to log in.
	out, err := runCmd(t, "", "config", "set", "url", srv.URL)
	if err != nil {
		t.Fatalf("config set failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "logged in as you@example.com (session verified)") {
		t.Errorf("expected a verified-session line, got:\n%s", out)
	}
}

func TestPullUsesProjectAPIOverride(t *testing.T) {
	srv := startFakeServer(t)
	// Global config points at a dead port; only the .shh.project override
	// (and its per-server session) can make the pull succeed.
	setupConfig(t, "http://127.0.0.1:1", true)
	addSession(t, srv.URL)
	dir := t.TempDir()
	markerPath, err := localcfg.WriteProject(dir, "proj1")
	if err != nil {
		t.Fatal(err)
	}
	if err := localcfg.SaveURL(markerPath, srv.URL); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	out, err := runCmd(t, "", "pull", "--shell")
	if err != nil {
		t.Fatalf("pull failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "export FOO='bar'") {
		t.Errorf("missing exports in output:\n%s", out)
	}
}

// addSession stores a test session for one more server, alongside whatever
// setupConfig wrote.
func addSession(t *testing.T, apiURL string) {
	t.Helper()
	addSessionToken(t, apiURL, "test-token")
}

func addSessionToken(t *testing.T, apiURL, token string) {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.SetSession(apiURL, config.StoredSession{Token: token})
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
}

func TestNotLoggedInToOverrideServerErrors(t *testing.T) {
	// Logged in globally, but the repo points at a server with no session.
	setupConfig(t, "https://global.example.com", true)
	dir := t.TempDir()
	markerPath, err := localcfg.WriteProject(dir, "proj1")
	if err != nil {
		t.Fatal(err)
	}
	if err := localcfg.SaveURL(markerPath, "https://secrets.example.com"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	_, err = runCmd(t, "", "secret", "list")
	if err == nil || !strings.Contains(err.Error(), "not logged in to https://secrets.example.com") {
		t.Errorf("err = %v, want a per-server not-logged-in error", err)
	}
}
