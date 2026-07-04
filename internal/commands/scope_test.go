package commands

import (
	"os"
	"path/filepath"
	"testing"

	"itsasecret.dev/cli/internal/localcfg"
)

func TestResolveFlagsWin(t *testing.T) {
	dir := t.TempDir()
	if _, err := localcfg.WriteProject(dir, "fileproj"); err != nil {
		t.Fatal(err)
	}
	if _, err := localcfg.WriteEnv(dir, "fileenv"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	s := scopeFlags{project: "flagproj", env: "flagenv"}
	project, env, err := s.resolve()
	if err != nil {
		t.Fatal(err)
	}
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

	var s scopeFlags
	project, env, err := s.resolve()
	if err != nil {
		t.Fatal(err)
	}
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

	var s scopeFlags
	project, env, err := s.resolve()
	if err != nil {
		t.Fatal(err)
	}
	if project != "fileproj" || env != "production" {
		t.Errorf("got %s/%s, want fileproj/production", project, env)
	}
}

func TestResolveMissingProjectErrors(t *testing.T) {
	t.Chdir(t.TempDir())

	var s scopeFlags
	if _, _, err := s.resolve(); err == nil {
		t.Error("expected error when no project is set anywhere")
	}
}
