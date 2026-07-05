package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"itsasecret.dev/cli/internal/localcfg"
)

const sortedExports = "export BAZ='qux'\nexport FOO='bar'\n"

func TestPullRecordsFileMode(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, true)
	dir := t.TempDir()
	t.Chdir(dir)
	if _, err := localcfg.WriteProject(dir, "proj1"); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "", "pull")
	if err != nil {
		t.Fatalf("pull failed: %v\noutput:\n%s", err, out)
	}
	if got := readFileOrEmpty(t, filepath.Join(dir, ".env")); got != sortedExports {
		t.Errorf(".env = %q, want sorted exports", got)
	}
	want := "project = proj1\npull = file:.env\n"
	if got := readFileOrEmpty(t, filepath.Join(dir, localcfg.ProjectFile)); got != want {
		t.Errorf("%s = %q, want %q", localcfg.ProjectFile, got, want)
	}
}

func TestPullRecordsShellMode(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, true)
	dir := t.TempDir()
	t.Chdir(dir)
	if _, err := localcfg.WriteProject(dir, "proj1"); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "", "pull", "--shell")
	if err != nil {
		t.Fatalf("pull failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "export FOO='bar'") {
		t.Errorf("missing exports in output:\n%s", out)
	}
	want := "project = proj1\npull = shell\n"
	if got := readFileOrEmpty(t, filepath.Join(dir, localcfg.ProjectFile)); got != want {
		t.Errorf("%s = %q, want %q", localcfg.ProjectFile, got, want)
	}
}

func TestPullWithEnvOverrideDoesNotRecord(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, true)
	dir := t.TempDir()
	t.Chdir(dir)
	markerPath, err := localcfg.WriteProject(dir, "proj1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := localcfg.WriteEnv(dir, "staging"); err != nil {
		t.Fatal(err)
	}
	if err := localcfg.SavePull(markerPath, localcfg.PullConfig{Mode: localcfg.PullModeFile, Out: ".env"}); err != nil {
		t.Fatal(err)
	}

	// A one-off pull of another env must not change what reload repeats.
	out, err := runCmd(t, "", "pull", "--shell", "--env", "dev-alice")
	if err != nil {
		t.Fatalf("pull failed: %v\noutput:\n%s", err, out)
	}
	want := "project = proj1\npull = file:.env\n"
	if got := readFileOrEmpty(t, markerPath); got != want {
		t.Errorf("%s = %q, want the recorded mode untouched (%q)", localcfg.ProjectFile, got, want)
	}
}

func TestPullWithProjectOverrideDoesNotRecord(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, true)
	dir := t.TempDir()
	t.Chdir(dir)
	markerPath, err := localcfg.WriteProject(dir, "proj1")
	if err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "", "pull", "--shell", "--project", "proj2")
	if err != nil {
		t.Fatalf("pull failed: %v\noutput:\n%s", err, out)
	}
	want := "project = proj1\n"
	if got := readFileOrEmpty(t, markerPath); got != want {
		t.Errorf("%s = %q, want no pull recorded for a foreign project", localcfg.ProjectFile, got)
	}
}

func TestPullMatchingLinkedEnvRecords(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, true)
	dir := t.TempDir()
	t.Chdir(dir)
	markerPath, err := localcfg.WriteProject(dir, "proj1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := localcfg.WriteEnv(dir, "staging"); err != nil {
		t.Fatal(err)
	}

	// Explicit flags that spell out the linked scope still count as a pull
	// of the linked scope.
	out, err := runCmd(t, "", "pull", "--shell", "--env", "staging")
	if err != nil {
		t.Fatalf("pull failed: %v\noutput:\n%s", err, out)
	}
	want := "project = proj1\npull = shell\n"
	if got := readFileOrEmpty(t, markerPath); got != want {
		t.Errorf("%s = %q, want %q", localcfg.ProjectFile, got, want)
	}
}

func TestPullWithoutMarkerRecordsNothing(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, true)
	dir := t.TempDir()
	t.Chdir(dir)

	out, err := runCmd(t, "", "pull", "--project", "proj1", "--shell")
	if err != nil {
		t.Fatalf("pull failed: %v\noutput:\n%s", err, out)
	}
	if got := readFileOrEmpty(t, filepath.Join(dir, localcfg.ProjectFile)); got != "" {
		t.Errorf("%s = %q, want no marker file created by pull", localcfg.ProjectFile, got)
	}
}

func TestReloadFileModeFromSubdir(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, true)
	root := t.TempDir()
	markerPath, err := localcfg.WriteProject(root, "proj1")
	if err != nil {
		t.Fatal(err)
	}
	if err := localcfg.SavePull(markerPath, localcfg.PullConfig{Mode: localcfg.PullModeFile, Out: ".env"}); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)

	out, err := runCmd(t, "", "reload")
	if err != nil {
		t.Fatalf("reload failed: %v\noutput:\n%s", err, out)
	}
	// The recorded path is relative to .shh.project, so the file lands at the
	// root even though reload ran in a subdirectory.
	if got := readFileOrEmpty(t, filepath.Join(root, ".env")); got != sortedExports {
		t.Errorf("root .env = %q, want sorted exports", got)
	}
	if got := readFileOrEmpty(t, filepath.Join(sub, ".env")); got != "" {
		t.Errorf("subdir .env = %q, want it absent", got)
	}
}

func TestReloadShellMode(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, true)
	dir := t.TempDir()
	markerPath, err := localcfg.WriteProject(dir, "proj1")
	if err != nil {
		t.Fatal(err)
	}
	if err := localcfg.SavePull(markerPath, localcfg.PullConfig{Mode: localcfg.PullModeShell}); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	out, err := runCmd(t, "", "reload")
	if err != nil {
		t.Fatalf("reload failed: %v\noutput:\n%s", err, out)
	}
	if out != sortedExports {
		t.Errorf("output = %q, want sorted exports only", out)
	}
}

func TestWriteExportsDialects(t *testing.T) {
	vars := map[string]string{"FOO": "it's a $ecret\\"}
	cases := map[string]string{
		"posix": `export FOO='it'\''s a $ecret\'` + "\n",
		"fish":  `set -gx FOO 'it\'s a $ecret\\'` + "\n",
		"pwsh":  `$env:FOO = 'it''s a $ecret\'` + "\n",
		"nu":    `{"FOO":"it's a $ecret\\"}` + "\n",
	}
	for dialect, want := range cases {
		var buf strings.Builder
		if err := writeExports(&buf, vars, dialect); err != nil {
			t.Fatalf("%s: %v", dialect, err)
		}
		if buf.String() != want {
			t.Errorf("%s = %q, want %q", dialect, buf.String(), want)
		}
	}
}

func TestDetectShellDialect(t *testing.T) {
	t.Setenv("DIRENV_IN_ENVRC", "")
	for shellPath, want := range map[string]string{
		"/bin/bash":                       "posix",
		"/usr/bin/zsh":                    "posix",
		"/usr/bin/fish":                   "fish",
		"/run/current-system/sw/bin/nu":   "nu",
		"/usr/local/bin/pwsh":             "pwsh",
		"":                                "posix",
	} {
		t.Setenv("SHELL", shellPath)
		if got := detectShellDialect(); got != want {
			t.Errorf("SHELL=%q → %q, want %q", shellPath, got, want)
		}
	}
	// direnv always evaluates .envrc with bash, whatever $SHELL says.
	t.Setenv("SHELL", "/run/current-system/sw/bin/nu")
	t.Setenv("DIRENV_IN_ENVRC", "1")
	if got := detectShellDialect(); got != "posix" {
		t.Errorf("inside direnv → %q, want posix", got)
	}
}

func TestPullShellDialectFlagRecordsAndEmits(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, true)
	dir := t.TempDir()
	t.Chdir(dir)
	if _, err := localcfg.WriteProject(dir, "proj1"); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "", "pull", "--shell=fish")
	if err != nil {
		t.Fatalf("pull failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "set -gx FOO 'bar'") {
		t.Errorf("expected fish output, got:\n%s", out)
	}
	want := "project = proj1\npull = shell:fish\n"
	if got := readFileOrEmpty(t, filepath.Join(dir, localcfg.ProjectFile)); got != want {
		t.Errorf("%s = %q, want %q", localcfg.ProjectFile, got, want)
	}

	// Reload replays the recorded dialect.
	out, err = runCmd(t, "", "reload")
	if err != nil {
		t.Fatalf("reload failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "set -gx FOO 'bar'") {
		t.Errorf("expected reload to replay fish output, got:\n%s", out)
	}
}

func TestPullShellAutoUsesLoginShell(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, true)
	t.Setenv("SHELL", "/run/current-system/sw/bin/nu")
	dir := t.TempDir()
	t.Chdir(dir)
	if _, err := localcfg.WriteProject(dir, "proj1"); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "", "pull", "--shell")
	if err != nil {
		t.Fatalf("pull failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, `"FOO":"bar"`) {
		t.Errorf("expected nu JSON output, got:\n%s", out)
	}
	// Auto stays auto in the record: reload re-detects for its own context.
	want := "project = proj1\npull = shell\n"
	if got := readFileOrEmpty(t, filepath.Join(dir, localcfg.ProjectFile)); got != want {
		t.Errorf("%s = %q, want %q", localcfg.ProjectFile, got, want)
	}
}

func TestReloadWithoutRecordedModeErrors(t *testing.T) {
	srv := startFakeServer(t)
	setupConfig(t, srv.URL, true)
	dir := t.TempDir()
	if _, err := localcfg.WriteProject(dir, "proj1"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	_, err := runCmd(t, "", "reload")
	if err == nil || !strings.Contains(err.Error(), "no pull mode recorded") {
		t.Errorf("err = %v, want a no-pull-recorded error", err)
	}
}
