package commands

import (
	"strings"
	"testing"
)

func TestShellInitEmitsSnippetPerShell(t *testing.T) {
	cases := map[string]string{
		"bash": "shh() {",
		"zsh":  "shh() {",
		"sh":   "shh() {",
		"fish": "function shh",
		"pwsh": "function shh {",
		"nu":   `def --env "shh load"`,
	}
	for shell, marker := range cases {
		out, err := runCmd(t, "", "shell-init", shell)
		if err != nil {
			t.Fatalf("shell-init %s: %v", shell, err)
		}
		if !strings.Contains(out, marker) {
			t.Errorf("shell-init %s missing %q, got:\n%s", shell, marker, out)
		}
		// Every snippet must define `shh load` and eval/load, and say so loudly.
		if !strings.Contains(out, "load") {
			t.Errorf("shell-init %s has no load path:\n%s", shell, out)
		}
		if !strings.Contains(strings.ToLower(out), "current shell") {
			t.Errorf("shell-init %s missing the current-shell notice:\n%s", shell, out)
		}
	}
}

// The eval-fed snippets must never contain a raw backtick: they are documented
// to be captured via eval "$(...)" / command substitution, where a stray
// backtick would itself be a command substitution.
func TestShellInitSnippetsHaveNoBacktick(t *testing.T) {
	for _, shell := range []string{"bash", "fish", "pwsh", "nu"} {
		out, err := runCmd(t, "", "shell-init", shell)
		if err != nil {
			t.Fatalf("shell-init %s: %v", shell, err)
		}
		if strings.Contains(out, "`") {
			t.Errorf("shell-init %s output contains a backtick", shell)
		}
	}
}

func TestShellInitUnknownShellErrors(t *testing.T) {
	_, err := runCmd(t, "", "shell-init", "cmd.exe")
	if err == nil || !strings.Contains(err.Error(), "unknown shell") {
		t.Fatalf("want unknown-shell error, got %v", err)
	}
}

func TestShellInitAutoDetectsFromSHELL(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/fish")
	out, err := runCmd(t, "", "shell-init")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "function shh") {
		t.Errorf("auto-detect fish failed, got:\n%s", out)
	}
}

// Without the shell integration, `shh load` can't touch the parent shell, so it
// must fail with guidance pointing at shell-init rather than doing nothing.
func TestLoadWithoutIntegrationExplains(t *testing.T) {
	_, err := runCmd(t, "", "load")
	if err == nil {
		t.Fatal("want an error explaining the missing integration")
	}
	msg := err.Error()
	if !strings.Contains(msg, "shell-init") || !strings.Contains(msg, "current shell") {
		t.Errorf("load error should point at shell-init and the current shell, got:\n%s", msg)
	}
}
