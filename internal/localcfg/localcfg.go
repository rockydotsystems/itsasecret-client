// Package localcfg reads and writes the per-directory .shh.* marker files
// that pin a directory tree to a project and environment, so commands don't
// need --project/--env flags on every run.
//
// .shh.project holds the project ID and is meant to be committed; .shh.env
// holds the environment name and stays local (per-developer), so it belongs
// in .gitignore.
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

// Scope is the project/environment resolved from .shh.* files. A zero field
// means the corresponding file was not found (or was empty). The *Path fields
// record which file each value came from, for user-facing messages.
type Scope struct {
	Project     string
	ProjectPath string
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
			v, err := readValue(p)
			if err != nil {
				return nil, err
			}
			if v != "" {
				scope.Project, scope.ProjectPath = v, p
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

// WriteProject writes .shh.project in dir and returns its path.
func WriteProject(dir, project string) (string, error) {
	return writeValue(filepath.Join(dir, ProjectFile), project)
}

// WriteEnv writes .shh.env in dir and returns its path.
func WriteEnv(dir, env string) (string, error) {
	return writeValue(filepath.Join(dir, EnvFile), env)
}

func writeValue(path, value string) (string, error) {
	if err := os.WriteFile(path, []byte(value+"\n"), 0o644); err != nil {
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
