package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sessionpkg "github.com/openmodu/modu/pkg/coding_agent/services/session"
)

func TestNormalizeResumeArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "bare --resume at the end",
			args: []string{"--resume"},
			want: []string{"--resume="},
		},
		{
			name: "bare --resume before another flag",
			args: []string{"--resume", "--no-approve"},
			want: []string{"--resume=", "--no-approve"},
		},
		{
			name: "single-dash form",
			args: []string{"-resume"},
			want: []string{"--resume="},
		},
		{
			name: "explicit id is left alone",
			args: []string{"--resume", "abc123"},
			want: []string{"--resume", "abc123"},
		},
		{
			name: "inline id is left alone",
			args: []string{"--resume=abc123"},
			want: []string{"--resume=abc123"},
		},
		{
			name: "arguments after -- are untouched",
			args: []string{"--no-approve", "--", "--resume"},
			want: []string{"--no-approve", "--", "--resume"},
		},
		{
			name: "no resume flag",
			args: []string{"-p", "hello"},
			want: []string{"-p", "hello"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeResumeArgs(tt.args)
			if strings.Join(got, " ") != strings.Join(tt.want, " ") {
				t.Fatalf("normalizeResumeArgs(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestResolveStartupResumeIDPicksLatestSessionOfCwd(t *testing.T) {
	agentDir := t.TempDir()
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	older := writeResumeFlagTestSession(t, agentDir, project, "older")
	newer := writeResumeFlagTestSession(t, agentDir, project, "newer")
	// Both files are written within the same instant, so age the first one
	// rather than relying on filesystem timestamp resolution.
	stale := time.Now().Add(-time.Hour)
	if err := os.Chtimes(older.FilePath(), stale, stale); err != nil {
		t.Fatal(err)
	}

	got, err := resolveStartupResumeID(agentDir, project, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if got != newer.SessionID() {
		t.Fatalf("bare --resume = %q, want newest session %q (older %q)", got, newer.SessionID(), older.SessionID())
	}
}

func TestResolveStartupResumeIDKeepsExplicitIDAndUnsetFlag(t *testing.T) {
	agentDir := t.TempDir()
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	writeResumeFlagTestSession(t, agentDir, project, "saved")

	got, err := resolveStartupResumeID(agentDir, project, "abc123", true)
	if err != nil || got != "abc123" {
		t.Fatalf("explicit id = (%q, %v), want abc123", got, err)
	}
	// Without --resume a fresh session is started, even though this directory
	// has one saved.
	got, err = resolveStartupResumeID(agentDir, project, "", false)
	if err != nil || got != "" {
		t.Fatalf("unset --resume = (%q, %v), want empty", got, err)
	}
}

func TestResolveStartupResumeIDFailsWithoutSavedSession(t *testing.T) {
	agentDir := t.TempDir()
	project := filepath.Join(t.TempDir(), "empty-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := resolveStartupResumeID(agentDir, project, "", true)
	if err == nil || !strings.Contains(err.Error(), project) {
		t.Fatalf("resolveStartupResumeID error = %v, want one naming %q", err, project)
	}
}

// writeResumeFlagTestSession saves one message in a new session for cwd.
// NewFreshManager (not NewManager) is required so each call produces its own
// file instead of appending to the directory's most recent session.
func writeResumeFlagTestSession(t *testing.T, agentDir, cwd, prompt string) *sessionpkg.Manager {
	t.Helper()
	mgr, err := sessionpkg.NewFreshManager(agentDir, cwd)
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Append(sessionpkg.NewEntry(sessionpkg.EntryTypeMessage, "", sessionpkg.MessageData{
		Role:    "user",
		Content: prompt,
	})); err != nil {
		t.Fatal(err)
	}
	return mgr
}
