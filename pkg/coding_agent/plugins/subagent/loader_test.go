package subagent

import (
	"os"
	"path/filepath"
	"testing"
)

func writeProfile(t *testing.T, dir, name, description string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	body := "---\nname: " + name + "\ndescription: " + description + "\n---\n\nBe useful.\n"
	if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}
}

// Discovery stays inside modu's own directories. Another tool's profiles are
// not modu's to run: picking them up would put agents the user never wrote
// for modu into the subagent tool description and its context budget.
func TestDiscoverIgnoresForeignAgentDirs(t *testing.T) {
	agentDir := t.TempDir()
	cwd := t.TempDir()

	writeProfile(t, filepath.Join(agentDir, "agents"), "user-agent", "modu user profile")
	writeProfile(t, filepath.Join(cwd, ".coding_agent", "agents"), "project-agent", "modu project profile")
	writeProfile(t, filepath.Join(cwd, ".claude", "agents"), "claude-agent", "another tool's profile")

	l := NewLoader()
	l.Discover(agentDir, cwd)

	if _, ok := l.Get("claude-agent"); ok {
		t.Error("a profile from .claude/agents must not be discovered")
	}
	user, ok := l.Get("user-agent")
	if !ok {
		t.Fatal("profile from {agentDir}/agents not discovered")
	}
	if user.Source != "user" {
		t.Errorf("user-agent source = %q, want user", user.Source)
	}
	project, ok := l.Get("project-agent")
	if !ok {
		t.Fatal("profile from .coding_agent/agents not discovered")
	}
	if project.Source != "project" {
		t.Errorf("project-agent source = %q, want project", project.Source)
	}
}

// Project profiles override user profiles of the same name.
func TestDiscoverProjectOverridesUser(t *testing.T) {
	agentDir := t.TempDir()
	cwd := t.TempDir()
	writeProfile(t, filepath.Join(agentDir, "agents"), "reviewer", "user reviewer")
	writeProfile(t, filepath.Join(cwd, ".coding_agent", "agents"), "reviewer", "project reviewer")

	l := NewLoader()
	l.Discover(agentDir, cwd)

	reviewer, ok := l.Get("reviewer")
	if !ok {
		t.Fatal("reviewer not discovered")
	}
	if reviewer.Description != "project reviewer" {
		t.Fatalf("reviewer description = %q; the project profile must win", reviewer.Description)
	}
}
