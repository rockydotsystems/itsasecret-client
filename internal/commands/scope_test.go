package commands

import (
	"os"
	"path/filepath"
	"testing"

	"itsasecret.dev/cli/internal/config"
	"itsasecret.dev/cli/internal/localcfg"
)

func resolveForTest(t *testing.T, s scopeFlags) (string, string) {
	t.Helper()
	rs, err := s.resolveScope()
	if err != nil {
		t.Fatal(err)
	}
	return rs.project, rs.env
}

func TestResolveFlagsWin(t *testing.T) {
	dir := t.TempDir()
	if _, err := localcfg.WriteProject(dir, "fileproj"); err != nil {
		t.Fatal(err)
	}
	if _, err := localcfg.WriteEnv(dir, "fileenv"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	project, env := resolveForTest(t, scopeFlags{project: "flagproj", env: "flagenv"})
	if project != "flagproj" || env != "flagenv" {
		t.Errorf("got %s/%s, want flags to win over files", project, env)
	}
}

func TestResolveFromFiles(t *testing.T) {
	root := t.TempDir()
	if _, err := localcfg.WriteProject(root, "fileproj"); err != nil {
		t.Fatal(err)
	}
	if _, err := localcfg.WriteEnv(root, "staging"); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "sub")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)

	project, env := resolveForTest(t, scopeFlags{})
	if project != "fileproj" || env != "staging" {
		t.Errorf("got %s/%s, want values from parent-dir files", project, env)
	}
}

func TestResolveEnvDefaultsToProduction(t *testing.T) {
	dir := t.TempDir()
	if _, err := localcfg.WriteProject(dir, "fileproj"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	project, env := resolveForTest(t, scopeFlags{})
	if project != "fileproj" || env != "production" {
		t.Errorf("got %s/%s, want fileproj/production", project, env)
	}
}

func TestResolveMissingProjectErrors(t *testing.T) {
	t.Chdir(t.TempDir())

	var s scopeFlags
	if _, err := s.resolveScope(); err == nil {
		t.Error("expected error when no project is set anywhere")
	}
}

func TestAPIURLProjectOverrideWins(t *testing.T) {
	dir := t.TempDir()
	markerPath, err := localcfg.WriteProject(dir, "fileproj")
	if err != nil {
		t.Fatal(err)
	}
	if err := localcfg.SaveAPI(markerPath, "https://secrets.example.com"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	var s scopeFlags
	rs, err := s.resolveScope()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{APIURL: "https://itsasecret.dev"}
	if got := rs.apiURL(cfg); got != "https://secrets.example.com" {
		t.Errorf("apiURL = %q, want the .shh.project override", got)
	}
}

func TestAPIURLFallsBackToGlobal(t *testing.T) {
	dir := t.TempDir()
	if _, err := localcfg.WriteProject(dir, "fileproj"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	var s scopeFlags
	rs, err := s.resolveScope()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{APIURL: "https://global.example.com"}
	if got := rs.apiURL(cfg); got != "https://global.example.com" {
		t.Errorf("apiURL = %q, want the global config value", got)
	}
}
