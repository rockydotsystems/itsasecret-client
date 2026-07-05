package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"itsasecret.dev/cli/internal/localcfg"
)

// startFakeServer serves the org/project/env list routes the interactive
// link flow hits: two orgs, projects only in the first, envs per project.
func startFakeServer(t *testing.T) *httptest.Server {
	t.Helper()
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(v); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}
	requireAuth := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		return true
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/auth/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeJSON(w, map[string]any{"user": map[string]string{"email": "you@example.com"}})
	})
	mux.HandleFunc("GET /api/orgs/{$}", func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		writeJSON(w, []map[string]string{
			{"id": "org1", "name": "acme"},
			{"id": "org2", "name": "beta"},
		})
	})
	mux.HandleFunc("GET /api/orgs/{orgID}/projects", func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		switch r.PathValue("orgID") {
		case "org1":
			writeJSON(w, []map[string]string{
				{"id": "proj1", "name": "www"},
				{"id": "proj2", "name": "client"},
			})
		default:
			writeJSON(w, []map[string]string{})
		}
	})
	mux.HandleFunc("GET /api/projects/{projectID}/envs", func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		writeJSON(w, []map[string]string{
			{"id": "env1", "name": "production"},
			{"id": "env2", "name": "staging"},
			{"id": "env3", "name": "dev-alice"},
		})
	})
	mux.HandleFunc("GET /api/projects/{projectID}/envs/{envName}/pull", func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		writeJSON(w, map[string]any{
			"vars": []map[string]string{
				{"key": "FOO", "value": "bar"},
				{"key": "BAZ", "value": "qux"},
			},
			"secrets": map[string]string{},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// setupConfig points the CLI config (via XDG_CONFIG_HOME) at apiURL, with a
// stored session for that server when loggedIn.
func setupConfig(t *testing.T, apiURL string, loggedIn bool) {
	t.Helper()
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	dir := filepath.Join(cfgHome, "itsasecret")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]any{"apiUrl": apiURL}
	if loggedIn {
		cfg["sessions"] = map[string]any{
			apiURL: map[string]string{"token": "test-token"},
		}
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// runCmd executes a CLI command, feeding input as stdin, and returns
// (combined output, error).
func runCmd(t *testing.T, input string, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs(args)
	root.SetIn(strings.NewReader(input))
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	err := root.Execute()
	return buf.String(), err
}

func runLink(t *testing.T, input string) (string, error) {
	t.Helper()
	return runCmd(t, input, "link")
}

func readFileOrEmpty(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestLinkInteractiveFullFlow(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, true)
	dir := t.TempDir()
	t.Chdir(dir)

	// org acme → project client (proj2) → env dev-alice
	out, err := runLink(t, "1\n2\n3\n")
	if err != nil {
		t.Fatalf("link failed: %v\noutput:\n%s", err, out)
	}
	if got := readFileOrEmpty(t, filepath.Join(dir, localcfg.ProjectFile)); got != "project = proj2\n" {
		t.Errorf("%s = %q, want proj2 recorded", localcfg.ProjectFile, got)
	}
	if got := readFileOrEmpty(t, filepath.Join(dir, localcfg.EnvFile)); got != "dev-alice\n" {
		t.Errorf("%s = %q, want dev-alice", localcfg.EnvFile, got)
	}
	gitignore := readFileOrEmpty(t, filepath.Join(dir, ".gitignore"))
	if !strings.Contains(gitignore, localcfg.EnvFile) {
		t.Errorf(".gitignore = %q, want it to contain %s", gitignore, localcfg.EnvFile)
	}
}

func TestLinkInteractiveSkipEnv(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, true)
	dir := t.TempDir()
	t.Chdir(dir)

	// org acme → project www (proj1) → option 4 is the explicit env skip
	out, err := runLink(t, "1\n1\n4\n")
	if err != nil {
		t.Fatalf("link failed: %v\noutput:\n%s", err, out)
	}
	if got := readFileOrEmpty(t, filepath.Join(dir, localcfg.ProjectFile)); got != "project = proj1\n" {
		t.Errorf("%s = %q, want proj1 recorded", localcfg.ProjectFile, got)
	}
	if got := readFileOrEmpty(t, filepath.Join(dir, localcfg.EnvFile)); got != "" {
		t.Errorf("%s = %q, want it absent when the env is skipped", localcfg.EnvFile, got)
	}
	if !strings.Contains(out, "default to production") {
		t.Errorf("output missing production-default note:\n%s", out)
	}
}

func TestLinkInteractiveReprompsOnInvalidInput(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, true)
	dir := t.TempDir()
	t.Chdir(dir)

	// "9" and "nope" are invalid org choices; the prompt retries until "1".
	out, err := runLink(t, "9\nnope\n1\n1\n4\n")
	if err != nil {
		t.Fatalf("link failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "must be a number between 1 and 2") {
		t.Errorf("expected re-prompt message in output:\n%s", out)
	}
	if got := readFileOrEmpty(t, filepath.Join(dir, localcfg.ProjectFile)); got != "project = proj1\n" {
		t.Errorf("%s = %q, want proj1 recorded", localcfg.ProjectFile, got)
	}
}

func TestLinkInteractiveOrgWithoutProjectsErrors(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, true)
	dir := t.TempDir()
	t.Chdir(dir)

	// org beta has no projects
	_, err := runLink(t, "2\n")
	if err == nil || !strings.Contains(err.Error(), "no projects") {
		t.Errorf("err = %v, want a no-projects error", err)
	}
	if got := readFileOrEmpty(t, filepath.Join(dir, localcfg.ProjectFile)); got != "" {
		t.Errorf("%s = %q, want no file written", localcfg.ProjectFile, got)
	}
}

func TestLinkNotLoggedInShowsStatus(t *testing.T) {
	setupConfig(t, "http://unused.invalid", false)
	dir := t.TempDir()
	t.Chdir(dir)

	out, err := runLink(t, "")
	if err != nil {
		t.Fatalf("link failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "Not linked.") {
		t.Errorf("expected status output, got:\n%s", out)
	}
	if !strings.Contains(out, "shh login") {
		t.Errorf("expected login hint, got:\n%s", out)
	}
}
