package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ResolveProjectID derives the canonical project identity from cwd. It uses
// the shared repository root for linked worktrees so a worktree name does not
// become part of the project identity.
func ResolveProjectID(cwd string) string {
	anchor := resolveProjectAnchor(cwd)
	if anchor == "" {
		return ""
	}

	parent := filepath.Base(filepath.Dir(anchor))
	current := filepath.Base(anchor)
	if parent == "." || current == "." || parent == string(filepath.Separator) || current == string(filepath.Separator) {
		return ""
	}
	return parent + "_" + current
}

func resolveProjectAnchor(cwd string) string {
	cwd = canonicalDirectory(cwd)
	if cwd == "" {
		return ""
	}

	if strings.TrimSpace(gitRevParse(cwd, "--is-inside-work-tree")) != "true" {
		return cwd
	}

	topLevel := canonicalGitPath(cwd, gitRevParse(cwd, "--path-format=absolute", "--show-toplevel"))
	gitDir := canonicalGitPath(cwd, gitRevParse(cwd, "--path-format=absolute", "--git-dir"))
	commonDir := canonicalGitPath(cwd, gitRevParse(cwd, "--path-format=absolute", "--git-common-dir"))
	if topLevel == "" || gitDir == "" || commonDir == "" {
		return firstNonEmpty(topLevel, cwd)
	}

	if gitDir != commonDir {
		switch filepath.Base(commonDir) {
		case ".git", ".bare":
			return filepath.Dir(commonDir)
		}
	}
	return topLevel
}

func canonicalDirectory(path string) string {
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return ""
		}
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(absolute)
}

func canonicalGitPath(cwd, path string) string {
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	return canonicalDirectory(path)
}

func gitRevParse(cwd string, args ...string) string {
	commandArgs := append([]string{"-C", cwd, "rev-parse"}, args...)
	output, err := exec.Command("git", commandArgs...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
