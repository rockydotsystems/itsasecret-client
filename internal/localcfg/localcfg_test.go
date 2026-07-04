package localcfg

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindNothing(t *testing.T) {
	scope, err := Find(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if scope.Project != "" || scope.Env != "" {
		t.Errorf("expected empty scope, got %+v", scope)
	}
}

func TestFindInCwd(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ProjectFile), "heyq1dpc\n")
	writeFile(t, filepath.Join(dir, EnvFile), "  staging  \n")

	scope, err := Find(dir)
	if err != nil {
		t.Fatal(err)
	}
	if scope.Project != "heyq1dpc" {
		t.Errorf("project = %q, want heyq1dpc", scope.Project)
	}
	if scope.Env != "staging" {
		t.Errorf("env = %q, want staging (trimmed)", scope.Env)
	}
	if scope.ProjectPath != filepath.Join(dir, ProjectFile) {
		t.Errorf("projectPath = %q", scope.ProjectPath)
	}
}

func TestFindWalksUp(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ProjectFile), "rootproj")
	writeFile(t, filepath.Join(root, EnvFile), "production")
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	scope, err := Find(nested)
	if err != nil {
		t.Fatal(err)
	}
	if scope.Project != "rootproj" || scope.Env != "production" {
		t.Errorf("got %+v, want values from root", scope)
	}
}

func TestFindClosestWinsIndependently(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ProjectFile), "rootproj")
	writeFile(t, filepath.Join(root, EnvFile), "production")
	nested := filepath.Join(root, "sub")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	// Only the env is overridden in the subdirectory; the project should
	// still come from the root.
	writeFile(t, filepath.Join(nested, EnvFile), "dev-alice")

	scope, err := Find(nested)
	if err != nil {
		t.Fatal(err)
	}
	if scope.Project != "rootproj" {
		t.Errorf("project = %q, want rootproj from parent", scope.Project)
	}
	if scope.Env != "dev-alice" {
		t.Errorf("env = %q, want dev-alice from subdir", scope.Env)
	}
}

func TestFindEmptyFileIsUnset(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ProjectFile), "rootproj")
	nested := filepath.Join(root, "sub")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(nested, ProjectFile), "\n")

	scope, err := Find(nested)
	if err != nil {
		t.Fatal(err)
	}
	if scope.Project != "rootproj" {
		t.Errorf("project = %q, want rootproj (empty file skipped)", scope.Project)
	}
}

func TestFindMultilineFileErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ProjectFile), "one\ntwo\n")

	if _, err := Find(dir); err == nil {
		t.Error("expected error for multi-line marker file")
	}
}

func TestWriteThenFindRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, err := WriteProject(dir, "heyq1dpc"); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteEnv(dir, "staging"); err != nil {
		t.Fatal(err)
	}

	scope, err := Find(dir)
	if err != nil {
		t.Fatal(err)
	}
	if scope.Project != "heyq1dpc" || scope.Env != "staging" {
		t.Errorf("round trip got %+v", scope)
	}
}

func TestEnsureGitignoredCreates(t *testing.T) {
	dir := t.TempDir()
	added, err := EnsureGitignored(dir, EnvFile)
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Error("expected entry to be added")
	}
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != EnvFile+"\n" {
		t.Errorf(".gitignore = %q", data)
	}
}

func TestEnsureGitignoredAppendsWithoutTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".gitignore"), "node_modules")

	added, err := EnsureGitignored(dir, EnvFile)
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Error("expected entry to be added")
	}
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	want := "node_modules\n" + EnvFile + "\n"
	if string(data) != want {
		t.Errorf(".gitignore = %q, want %q", data, want)
	}
}

func TestEnsureGitignoredIdempotent(t *testing.T) {
	dir := t.TempDir()
	for i, content := range []string{EnvFile + "\n", "/" + EnvFile + "\n"} {
		writeFile(t, filepath.Join(dir, ".gitignore"), content)
		added, err := EnsureGitignored(dir, EnvFile)
		if err != nil {
			t.Fatal(err)
		}
		if added {
			t.Errorf("case %d: entry added twice", i)
		}
		data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != content {
			t.Errorf("case %d: .gitignore modified to %q", i, data)
		}
	}
}
