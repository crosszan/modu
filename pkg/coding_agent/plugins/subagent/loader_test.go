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

// A repo that already carries Claude Code agent profiles should work without
// moving files, and modu's own project dir stays authoritative on a clash.
func TestDiscoverReadsClaudeAgentDirs(t *testing.T) {
	agentDir := t.TempDir()
	cwd := t.TempDir()

	writeProfile(t, filepath.Join(cwd, ".claude", "agents"), "explorer", "from claude dir")
	writeProfile(t, filepath.Join(cwd, ".claude", "agents"), "reviewer", "claude reviewer")
	writeProfile(t, filepath.Join(cwd, ".coding_agent", "agents"), "reviewer", "modu reviewer")

	l := NewLoader()
	l.Discover(agentDir, cwd)

	explorer, ok := l.Get("explorer")
	if !ok {
		t.Fatal("profile from .claude/agents not discovered")
	}
	if explorer.Source != "project" {
		t.Fatalf("explorer source = %q, want project", explorer.Source)
	}
	reviewer, ok := l.Get("reviewer")
	if !ok {
		t.Fatal("reviewer not discovered")
	}
	if reviewer.Description != "modu reviewer" {
		t.Fatalf("reviewer description = %q; .coding_agent/agents must win the name clash", reviewer.Description)
	}
}
