package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"itsasecret.dev/cli/internal/api"
	"itsasecret.dev/cli/internal/localcfg"

	"github.com/spf13/cobra"
)

// shellDialects are the --shell output formats. "auto" resolves at delivery
// time via detectShellDialect.
var shellDialects = []string{"posix", "fish", "nu", "pwsh"}

// envKeyPattern matches a POSIX environment-variable / shell identifier. Values
// are always quoted before emission, but variable NAMES are interpolated raw
// into `export NAME=...` / `set -gx NAME ...` / `$env:NAME = ...`, and the
// documented usage feeds that output straight to `eval`/`source`. A name is
// therefore an unquotable injection point: a hostile or compromised server (the
// CLI's threat model already assumes the URL can be attacker-controlled - see
// requireSecureURL) returning a key like `X=1;curl evil` would otherwise run
// arbitrary shell. Validate every key and fail closed rather than emit it.
var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// shellQuote single-quotes a value so it survives `source`/`eval` and direnv's
// dotenv parser intact, including spaces, $, and quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// fishQuote single-quotes for fish, where only \' and \\ escape inside
// single quotes.
func fishQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return "'" + strings.ReplaceAll(s, "'", `\'`) + "'"
}

// pwshQuote single-quotes for PowerShell, where ' is escaped by doubling.
func pwshQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func newPullCmd() *cobra.Command {
	var (
		scope        scopeFlags
		shellDialect string
		outFile      string
	)
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull env vars & secrets into a file or shell",
		Long: `Pull env vars & secrets into a file (default .env, or --out) or, with
--shell, print them for your shell:

  eval "$(shh pull --shell)"                     # bash/zsh, direnv
  eval (shh pull --shell)                        # fish
  load-env (shh pull --shell | from json)        # nushell

Bare --shell picks the dialect from $SHELL (POSIX inside direnv, where .envrc
is always bash); force one with --shell=posix|fish|nu|pwsh.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pc := localcfg.PullConfig{Mode: localcfg.PullModeFile, Out: outFile}
			if pc.Out == "" {
				pc.Out = ".env"
			}
			if cmd.Flags().Changed("shell") {
				pc = localcfg.PullConfig{Mode: localcfg.PullModeShell}
				if shellDialect != "auto" {
					if err := validateShellDialect(shellDialect); err != nil {
						return err
					}
					pc.Shell = shellDialect
				}
			}

			rs, client, err := scope.resolveClient(cmd)
			if err != nil {
				return err
			}
			if err := runPull(cmd.Context(), client, rs.project, rs.env, pc, cmd.OutOrStdout()); err != nil {
				return err
			}
			recordPull(cmd.ErrOrStderr(), rs, pc)
			return nil
		},
	}
	addScopeFlags(cmd, &scope)
	cmd.Flags().StringVar(&shellDialect, "shell", "", "emit for a shell: --shell[=posix|fish|nu|pwsh] (default: from $SHELL)")
	cmd.Flags().Lookup("shell").NoOptDefVal = "auto"
	cmd.Flags().StringVar(&outFile, "out", "", "output file (default: .env)")
	return cmd
}

func validateShellDialect(dialect string) error {
	for _, d := range shellDialects {
		if dialect == d {
			return nil
		}
	}
	return fmt.Errorf("unknown shell dialect %q (expected %s)", dialect, strings.Join(shellDialects, ", "))
}

// detectShellDialect resolves a bare --shell. Inside direnv POSIX is forced -
// .envrc is always evaluated by bash, and $SHELL still names the login shell
// there, which would lie (e.g. a nushell user's direnv would get nu JSON fed
// to bash). Otherwise the login shell decides.
func detectShellDialect() string {
	if os.Getenv("DIRENV_IN_ENVRC") != "" {
		return "posix"
	}
	shell := filepath.Base(os.Getenv("SHELL"))
	switch {
	case strings.Contains(shell, "fish"):
		return "fish"
	case strings.Contains(shell, "pwsh"), strings.Contains(shell, "powershell"):
		return "pwsh"
	case shell == "nu" || strings.Contains(shell, "nushell"):
		return "nu"
	default:
		return "posix"
	}
}

// runPull fetches the environment's values and delivers them per pc: shell
// output on out for PullModeShell (dialect resolved here when pc.Shell is
// empty/auto), written to pc.Out for PullModeFile.
func runPull(ctx context.Context, client *api.Client, project, env string, pc localcfg.PullConfig, out io.Writer) error {
	vars, err := client.Pull(ctx, project, env)
	if err != nil {
		return err
	}

	if pc.Mode == localcfg.PullModeShell {
		dialect := pc.Shell
		if dialect == "" {
			dialect = detectShellDialect()
		}
		if err := validateShellDialect(dialect); err != nil {
			return err
		}
		return writeExports(out, vars, dialect)
	}

	// 0600: the output holds decrypted secrets. os.Create would use 0666&umask
	// (typically 0644, world-readable). Truncate on open to match os.Create's
	// overwrite semantics. If the file already exists with looser perms, tighten
	// them - OpenFile's mode only applies on creation.
	f, err := os.OpenFile(pc.Out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	// The .env file stays POSIX - it's meant for `source` and direnv.
	if err := writeExports(f, vars, "posix"); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	say(out, "Wrote %s\n", pc.Out)
	return nil
}

func writeExports(w io.Writer, vars map[string]string, dialect string) error {
	// Reject unsafe variable names before emitting anything - fail closed so a
	// single hostile key can't slip arbitrary shell into an `eval`/`source`.
	for k := range vars {
		if !envKeyPattern.MatchString(k) {
			return fmt.Errorf("refusing to emit variable with unsafe name %q", k)
		}
	}

	if dialect == "nu" {
		// Nushell has no eval; the injection path is structured data:
		//   load-env (shh pull --shell | from json)
		data, err := json.Marshal(vars)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, "%s\n", data)
		return err
	}

	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		var line string
		switch dialect {
		case "fish":
			line = fmt.Sprintf("set -gx %s %s", k, fishQuote(vars[k]))
		case "pwsh":
			line = fmt.Sprintf("$env:%s = %s", k, pwshQuote(vars[k]))
		default: // posix
			line = fmt.Sprintf("export %s=%s", k, shellQuote(vars[k]))
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

// recordPull remembers the delivery mode in the resolved .shh.project so
// `shh reload` can repeat it. Only a pull of the linked scope is recorded -
// a one-off --project/--env override describes a different pull than the one
// `shh reload` (which always targets the linked project and env) would
// repeat. Best-effort: a pull that worked shouldn't fail because the marker
// file can't be written.
func recordPull(errOut io.Writer, rs *resolvedScope, pc localcfg.PullConfig) {
	if rs.files.ProjectPath == "" || rs.project != rs.files.Project {
		return
	}
	linkedEnv := rs.files.Env
	if linkedEnv == "" {
		linkedEnv = "production"
	}
	if rs.env != linkedEnv {
		return
	}
	if pc.Mode == localcfg.PullModeFile {
		pc.Out = pathRelativeToMarker(rs.files.ProjectPath, pc.Out)
	}
	if err := localcfg.SavePull(rs.files.ProjectPath, pc); err != nil {
		say(errOut, "warning: could not record pull mode in %s: %v\n", rs.files.ProjectPath, err)
	}
}

// pathRelativeToMarker rewrites out relative to the .shh.project directory,
// so the recorded path means the same thing wherever reload later runs.
// Absolute paths (given or as fallback) are kept as-is.
func pathRelativeToMarker(markerPath, out string) string {
	abs, err := filepath.Abs(out)
	if err != nil {
		return out
	}
	if filepath.IsAbs(out) {
		return out
	}
	rel, err := filepath.Rel(filepath.Dir(markerPath), abs)
	if err != nil {
		return abs
	}
	return rel
}
