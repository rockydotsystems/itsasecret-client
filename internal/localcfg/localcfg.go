// Package localcfg reads and writes the per-directory .shh.* marker files
// that pin a directory tree to a project and environment, so commands don't
// need --project/--env flags on every run.
//
// .shh.project is meant to be committed and holds `key = value` lines:
//
//	project = heyq1dpc
//	pull = file:.env
//
// `project` is the project ID; `pull` records how the last `shh pull`
// delivered values ("shell", or "file:<path>" with the path relative to the
// .shh.project directory unless absolute) so `shh reload` can repeat it. A
// legacy file holding just a bare project ID line still parses. Unknown keys
// are ignored for forward compatibility.
//
// .shh.env holds the environment name as a single line and stays local
// (per-developer), so it belongs in .gitignore.
package localcfg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	ProjectFile = ".shh.project"
	EnvFile     = ".shh.env"
)

// PullMode says where `shh pull` delivered values.
type PullMode string

const (
	PullModeFile  PullMode = "file"
	PullModeShell PullMode = "shell"
)

// PullConfig is the delivery of the last `shh pull`, recorded in
// .shh.project so `shh reload` can repeat it.
type PullConfig struct {
	Mode PullMode
	// Out is the destination path for PullModeFile, relative to the
	// .shh.project directory unless absolute.
	Out string
}

func (p PullConfig) encode() string {
	if p.Mode == PullModeFile {
		return "file:" + p.Out
	}
	return string(p.Mode)
}

// Scope is the project/environment resolved from .shh.* files. A zero field
// means the corresponding file was not found (or was empty). The *Path fields
// record which file each value came from, for user-facing messages. Pull is
// the pull config recorded in the same .shh.project the project came from,
// nil if none was recorded.
type Scope struct {
	Project     string
	ProjectPath string
	Pull        *PullConfig
	Env         string
	EnvPath     string
}

// Find resolves .shh.project and .shh.env starting at dir and walking up
// parent directories to the filesystem root. Each file is resolved
// independently — the closest one wins — so a nested project can override
// just one of the two.
func Find(dir string) (*Scope, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	scope := &Scope{}
	for {
		if scope.Project == "" {
			p := filepath.Join(dir, ProjectFile)
			project, pull, err := parseProjectFile(p)
			if err != nil {
				return nil, err
			}
			if project != "" {
				scope.Project, scope.ProjectPath, scope.Pull = project, p, pull
			}
		}
		if scope.Env == "" {
			p := filepath.Join(dir, EnvFile)
			v, err := readValue(p)
			if err != nil {
				return nil, err
			}
			if v != "" {
				scope.Env, scope.EnvPath = v, p
			}
		}
		if scope.Project != "" && scope.Env != "" {
			return scope, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return scope, nil
		}
		dir = parent
	}
}

// parseProjectFile reads a .shh.project file. A missing file returns zero
// values without an error.
func parseProjectFile(path string) (project string, pull *PullConfig, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, nil
		}
		return "", nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, isKV := strings.Cut(line, "=")
		if !isKV {
			// Legacy format: a bare line is the project ID.
			if project != "" {
				return "", nil, fmt.Errorf("%s: multiple project values", path)
			}
			project = line
			continue
		}
		switch key, value = strings.TrimSpace(key), strings.TrimSpace(value); key {
		case "project":
			project = value
		case "pull":
			pull, err = parsePullValue(path, value)
			if err != nil {
				return "", nil, err
			}
		default:
			// Ignore unknown keys so older CLIs tolerate newer files.
		}
	}
	return project, pull, nil
}

func parsePullValue(path, value string) (*PullConfig, error) {
	if value == string(PullModeShell) {
		return &PullConfig{Mode: PullModeShell}, nil
	}
	if out, ok := strings.CutPrefix(value, "file:"); ok && out != "" {
		return &PullConfig{Mode: PullModeFile, Out: out}, nil
	}
	return nil, fmt.Errorf(`%s: invalid pull setting %q (expected "shell" or "file:<path>")`, path, value)
}

func formatProjectFile(project string, pull *PullConfig) string {
	s := "project = " + project + "\n"
	if pull != nil {
		s += "pull = " + pull.encode() + "\n"
	}
	return s
}

// readValue returns the trimmed content of a single-value marker file, or ""
// if the file does not exist or is empty.
func readValue(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	v := strings.TrimSpace(string(data))
	if strings.ContainsAny(v, "\n\r") {
		return "", fmt.Errorf("%s: expected a single value, got multiple lines", path)
	}
	return v, nil
}

// WriteProject writes .shh.project in dir and returns its path, preserving a
// recorded pull config if the file already exists.
func WriteProject(dir, project string) (string, error) {
	path := filepath.Join(dir, ProjectFile)
	_, pull, err := parseProjectFile(path)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(formatProjectFile(project, pull)), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// SavePull records the pull config in an existing .shh.project file
// (identified by its full path, as returned in Scope.ProjectPath). The write
// is skipped when the file already says exactly this, so repeated pulls (e.g.
// `pull --shell` from an .envrc on every cd) don't churn a tracked file.
func SavePull(path string, pull PullConfig) error {
	project, _, err := parseProjectFile(path)
	if err != nil {
		return err
	}
	if project == "" {
		return fmt.Errorf("%s: no project recorded", path)
	}
	content := formatProjectFile(project, &pull)
	if current, err := os.ReadFile(path); err == nil && string(current) == content {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// WriteEnv writes .shh.env in dir and returns its path.
func WriteEnv(dir, env string) (string, error) {
	path := filepath.Join(dir, EnvFile)
	if err := os.WriteFile(path, []byte(env+"\n"), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// EnsureGitignored makes sure entry is listed in dir's .gitignore, creating
// the file if needed. Returns true if the entry was added, false if it was
// already present.
func EnsureGitignored(dir, entry string) (bool, error) {
	path := filepath.Join(dir, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == entry || line == "/"+entry {
			return false, nil
		}
	}
	out := string(data)
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	out += entry + "\n"
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return false, err
	}
	return true, nil
}
