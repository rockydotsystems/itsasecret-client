package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// Shells we can print an integration snippet for. bash/zsh/sh share the POSIX
// snippet; the rest are their own.
var shellInitShells = []string{"bash", "zsh", "sh", "fish", "pwsh", "nu"}

// Loading secrets into the *current* shell can only be done by the shell
// itself: a child process (this binary) cannot mutate its parent's
// environment. So `shh load` is really a shell function, installed once via
// `shh shell-init`, that captures `shh pull --shell` and evals it in place -
// then prints a loud notice that it just put live secret values into the
// running shell.

func newShellInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell-init [bash|zsh|fish|pwsh|nu]",
		Short: "Print the shell integration that enables `shh load`",
		Long: `Print a shell function that adds ` + "`shh load`" + `: it pulls this directory's
env and evaluates it into your CURRENT shell (a subprocess can't do that on its
own), then prints a clear notice that live secret values were loaded.

Install it once by sourcing it from your shell startup file:

  bash   echo 'eval "$(shh shell-init bash)"' >> ~/.bashrc
  zsh    echo 'eval "$(shh shell-init zsh)"'  >> ~/.zshrc
  fish   echo 'shh shell-init fish | source'  >> ~/.config/fish/config.fish
  pwsh   Add-Content $PROFILE 'Invoke-Expression (& shh shell-init pwsh | Out-String)'
  nu     shh shell-init nu | save --append ($nu.'config-path')

Then open a new shell and run ` + "`shh load`" + `. With no argument the shell is
detected from $SHELL.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			shell := ""
			if len(args) == 1 {
				shell = args[0]
			} else {
				shell = detectInitShell()
			}
			snippet, err := shellInitSnippet(shell)
			if err != nil {
				return err
			}
			// Snippet only - it is meant to be sourced/eval'd, so nothing else
			// may land on stdout.
			fmt.Fprint(cmd.OutOrStdout(), snippet)
			return nil
		},
	}
	return cmd
}

// newLoadCmd is the binary-side fallback. When the shell integration is active
// the `shh` shell function intercepts `load` and this never runs; reaching it
// means the integration isn't installed, so explain how to enable it rather
// than silently failing (we cannot touch the parent shell from here).
func newLoadCmd() *cobra.Command {
	var scope scopeFlags
	cmd := &cobra.Command{
		Use:   "load",
		Short: "Load this directory's env into the current shell (needs shell integration)",
		Long: `Load this directory's secrets and vars into your CURRENT shell.

This requires the one-time shell integration, because a subprocess cannot change
its parent shell's environment - the shell has to evaluate the values itself.
Enable it with ` + "`shh shell-init`" + `, then run ` + "`shh load`" + ` again.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf(`shh load needs the shell integration to change your current shell.

A subprocess can't modify its parent shell's environment, so the shell has to
eval the values itself. Enable the integration once:

  bash   echo 'eval "$(shh shell-init bash)"' >> ~/.bashrc
  zsh    echo 'eval "$(shh shell-init zsh)"'  >> ~/.zshrc
  fish   echo 'shh shell-init fish | source'  >> ~/.config/fish/config.fish
  pwsh   Add-Content $PROFILE 'Invoke-Expression (& shh shell-init pwsh | Out-String)'
  nu     shh shell-init nu | save --append ($nu.'config-path')

then open a new shell and run 'shh load' again.

To only print the export statements (and eval them yourself), use:
  eval "$(shh pull --shell)"`)
		},
	}
	// Accept the scope flags so `shh load --project/--env ...` parses cleanly
	// even without the integration (the message is the same regardless).
	addScopeFlags(cmd, &scope)
	return cmd
}

// detectInitShell resolves the shell for a bare `shh shell-init` from $SHELL.
func detectInitShell() string {
	shell := filepath.Base(os.Getenv("SHELL"))
	switch {
	case strings.Contains(shell, "fish"):
		return "fish"
	case strings.Contains(shell, "pwsh"), strings.Contains(shell, "powershell"):
		return "pwsh"
	case shell == "nu" || strings.Contains(shell, "nushell"):
		return "nu"
	case strings.Contains(shell, "zsh"):
		return "zsh"
	default:
		return "bash"
	}
}

func shellInitSnippet(shell string) (string, error) {
	switch shell {
	case "bash", "zsh", "sh":
		return posixShellInit, nil
	case "fish":
		return fishShellInit, nil
	case "pwsh", "powershell":
		return pwshShellInit, nil
	case "nu", "nushell":
		return nuShellInit, nil
	default:
		return "", fmt.Errorf("unknown shell %q (expected %s)", shell, strings.Join(shellInitShells, ", "))
	}
}

// The snippets below are raw strings, so they must contain no backtick. Shell
// escapes like \033 (the ANSI CSI) and \n are written literally and expanded by
// the shell's own printf, not by Go.

const posixShellInit = `# itsasecret shell integration (bash/zsh).
# Adds "shh load": pull this directory's env and eval it into the CURRENT shell.
shh() {
  if [ "${1:-}" = load ]; then
    shift
    _shh_out="$(command shh pull --shell=posix "$@")" || { printf '%s\n' "$_shh_out" >&2; return 1; }
    eval "$_shh_out"
    printf '\033[1;33mshh\033[0m loaded secrets into your CURRENT shell - these are live values, readable by every command you run here and by env. Open a new shell to clear them.\n' >&2
    unset _shh_out
  else
    command shh "$@"
  fi
}
`

const fishShellInit = `# itsasecret shell integration (fish).
# Adds "shh load": pull this directory's env and eval it into the CURRENT shell.
function shh
    if test (count $argv) -gt 0; and test "$argv[1]" = load
        set -e argv[1]
        set -l _shh_out (command shh pull --shell=fish $argv)
        if test $status -ne 0
            printf '%s\n' $_shh_out >&2
            return 1
        end
        eval (string join \n -- $_shh_out)
        printf '\033[1;33mshh\033[0m loaded secrets into your CURRENT shell - these are live values, readable by every command you run here and by env. Open a new shell to clear them.\n' >&2
    else
        command shh $argv
    end
end
`

const pwshShellInit = `# itsasecret shell integration (PowerShell).
# Adds "shh load": pull this directory's env and eval it into the CURRENT shell.
function shh {
    if ($args.Count -gt 0 -and $args[0] -eq 'load') {
        $rest = if ($args.Count -gt 1) { $args[1..($args.Count - 1)] } else { @() }
        $out = & (Get-Command shh -CommandType Application) pull --shell=pwsh @rest
        if ($LASTEXITCODE -ne 0) { $out | ForEach-Object { [Console]::Error.WriteLine($_) }; return }
        $out | Invoke-Expression
        Write-Warning 'shh loaded secrets into your CURRENT shell - these are live values, readable by child processes and by Get-ChildItem Env:. Open a new shell to clear them.'
    } else {
        & (Get-Command shh -CommandType Application) @args
    }
}
`

const nuShellInit = `# itsasecret shell integration (nushell).
# Adds "shh load": pull this directory's env and load it into the CURRENT shell.
def --env "shh load" [...rest] {
    let out = (^shh pull --shell nu ...$rest)
    load-env ($out | from json)
    print -e $"(ansi yellow_bold)shh(ansi reset) loaded secrets into your CURRENT shell - these are live values, readable by every command you run here and by env. Open a new shell to clear them."
}
`
