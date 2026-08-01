package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeHost is a minimal Host implementation for testing the Controller
// without a real kernel.
type fakeHost struct {
	cwd                 string
	agentDir            string
	worktreeModeEnabled bool

	switchedTo      []string
	createdEvents   []string
	removedEvents   []string
	runtimeStateHit int
}

func (h *fakeHost) Cwd() string      { return h.cwd }
func (h *fakeHost) AgentDir() string { return h.agentDir }
func (h *fakeHost) SwitchCwd(newCwd string) {
	h.cwd = newCwd
	h.switchedTo = append(h.switchedTo, newCwd)
}
func (h *fakeHost) EmitWorktreeCreated(path string) { h.createdEvents = append(h.createdEvents, path) }
func (h *fakeHost) EmitWorktreeRemoved(path string) { h.removedEvents = append(h.removedEvents, path) }
func (h *fakeHost) WriteRuntimeState()              { h.runtimeStateHit++ }
func (h *fakeHost) WorktreeModeEnabled() bool       { return h.worktreeModeEnabled }

func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// initTestRepo creates a git repository with one commit, so `git worktree
// add` (which requires a valid HEAD) works against it.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, dir, "add", ".")
	runGitCmd(t, dir, "commit", "-q", "-m", "initial")
	return dir
}

func TestNewControllerActiveWorktreeEmptyByDefault(t *testing.T) {
	host := &fakeHost{}
	c := New(host)
	if got := c.ActiveWorktree(); got != "" {
		t.Errorf("ActiveWorktree() = %q, want empty", got)
	}
}

func TestActiveWorktreeNilSafety(t *testing.T) {
	var c *Controller
	if got := c.ActiveWorktree(); got != "" {
		t.Errorf("nil Controller.ActiveWorktree() = %q, want empty", got)
	}
}

func TestEnterWorktreeRejectedWhenModeDisabled(t *testing.T) {
	repo := initTestRepo(t)
	host := &fakeHost{cwd: repo, agentDir: t.TempDir(), worktreeModeEnabled: false}
	c := New(host)
	_, err := c.EnterWorktree()
	if err == nil {
		t.Fatal("expected an error when worktree mode is disabled")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEnterWorktreeRejectedOutsideGitRepo(t *testing.T) {
	nonRepo := t.TempDir()
	host := &fakeHost{cwd: nonRepo, agentDir: t.TempDir(), worktreeModeEnabled: true}
	c := New(host)
	_, err := c.EnterWorktree()
	if err == nil {
		t.Fatal("expected an error outside a git repository")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEnterAndExitWorktreeLifecycle(t *testing.T) {
	repo := initTestRepo(t)
	agentDir := t.TempDir()
	host := &fakeHost{cwd: repo, agentDir: agentDir, worktreeModeEnabled: true}
	c := New(host)

	path, err := c.EnterWorktree()
	if err != nil {
		t.Fatalf("EnterWorktree: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("worktree directory should exist: %v", err)
	}
	if c.ActiveWorktree() != path {
		t.Errorf("ActiveWorktree() = %q, want %q", c.ActiveWorktree(), path)
	}
	if host.cwd != path {
		t.Errorf("host.cwd = %q, want %q (SwitchCwd should move into the worktree)", host.cwd, path)
	}
	if len(host.createdEvents) != 1 || host.createdEvents[0] != path {
		t.Errorf("expected one EmitWorktreeCreated(%q), got %v", path, host.createdEvents)
	}
	if host.runtimeStateHit == 0 {
		t.Error("expected WriteRuntimeState to be called")
	}

	status := c.Status()
	if !status.Active || status.Path != path || !status.Exists {
		t.Errorf("unexpected status after entering: %+v", status)
	}
	if status.OriginalCwd != repo {
		t.Errorf("Status.OriginalCwd = %q, want %q", status.OriginalCwd, repo)
	}

	if err := c.ExitWorktree(); err != nil {
		t.Fatalf("ExitWorktree: %v", err)
	}
	if c.ActiveWorktree() != "" {
		t.Errorf("ActiveWorktree() after exit = %q, want empty", c.ActiveWorktree())
	}
	if host.cwd != repo {
		t.Errorf("host.cwd after exit = %q, want restored to %q", host.cwd, repo)
	}
	if len(host.removedEvents) != 1 || host.removedEvents[0] != path {
		t.Errorf("expected one EmitWorktreeRemoved(%q), got %v", path, host.removedEvents)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("worktree directory should be removed after exit, stat err=%v", err)
	}
}

func TestEnterWorktreeIsIdempotentWhileActive(t *testing.T) {
	repo := initTestRepo(t)
	host := &fakeHost{cwd: repo, agentDir: t.TempDir(), worktreeModeEnabled: true}
	c := New(host)

	first, err := c.EnterWorktree()
	if err != nil {
		t.Fatalf("EnterWorktree: %v", err)
	}
	second, err := c.EnterWorktree()
	if err != nil {
		t.Fatalf("EnterWorktree (second call): %v", err)
	}
	if first != second {
		t.Errorf("calling EnterWorktree twice should return the same active path, got %q then %q", first, second)
	}
	if len(host.createdEvents) != 1 {
		t.Errorf("expected exactly one create event across two EnterWorktree calls, got %d", len(host.createdEvents))
	}
}

func TestExitWorktreeWithoutActiveWorktreeIsNoop(t *testing.T) {
	host := &fakeHost{cwd: "/somewhere", agentDir: t.TempDir()}
	c := New(host)
	if err := c.ExitWorktree(); err != nil {
		t.Fatalf("ExitWorktree with nothing active should not error: %v", err)
	}
	if len(host.removedEvents) != 0 {
		t.Error("no worktree was active, should not emit a removed event")
	}
}

func TestActiveDiffReflectsWorkingTreeChanges(t *testing.T) {
	repo := initTestRepo(t)
	host := &fakeHost{cwd: repo, agentDir: t.TempDir(), worktreeModeEnabled: true}
	c := New(host)
	path, err := c.EnterWorktree()
	if err != nil {
		t.Fatalf("EnterWorktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "README.md"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}

	diff, err := c.ActiveDiff()
	if err != nil {
		t.Fatalf("ActiveDiff: %v", err)
	}
	if !strings.Contains(diff.NameStatus, "README.md") {
		t.Errorf("NameStatus should mention the changed file, got %q", diff.NameStatus)
	}
	if !strings.Contains(diff.Patch, "changed") {
		t.Errorf("Patch should contain the change, got %q", diff.Patch)
	}
}

func TestActiveDiffErrorsWithoutActiveWorktree(t *testing.T) {
	host := &fakeHost{cwd: t.TempDir(), agentDir: t.TempDir()}
	c := New(host)
	if _, err := c.ActiveDiff(); err == nil {
		t.Fatal("expected an error when no worktree is active")
	}
}

func TestListManagedIncludesActiveEvenWhenUndiscovered(t *testing.T) {
	// The active worktree lives outside AgentDir()/worktrees in this test
	// (fakeHost doesn't create it there), so ListManaged must still surface
	// it via the active-path fallback rather than silently dropping it.
	repo := initTestRepo(t)
	agentDir := t.TempDir()
	host := &fakeHost{cwd: repo, agentDir: agentDir, worktreeModeEnabled: true}
	c := New(host)
	path, err := c.EnterWorktree()
	if err != nil {
		t.Fatalf("EnterWorktree: %v", err)
	}

	managed := c.ListManaged()
	found := false
	for _, info := range managed {
		if info.Path == path {
			found = true
			if !info.Active || !info.Exists {
				t.Errorf("active worktree entry should be Active=true Exists=true, got %+v", info)
			}
		}
	}
	if !found {
		t.Errorf("ListManaged() should include the active worktree, got %+v", managed)
	}
}

func TestIsManagedPath(t *testing.T) {
	agentDir := t.TempDir()
	host := &fakeHost{agentDir: agentDir}
	c := New(host)
	base := filepath.Join(agentDir, "worktrees")

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"empty path", "", false},
		{"inside managed base", filepath.Join(base, "abc", "repo"), true},
		{"managed base itself", base, false},
		{"outside managed base", "/etc/passwd", false},
		{"sibling directory that shares a prefix", base + "-evil", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.isManagedPath(tt.path); got != tt.want {
				t.Errorf("isManagedPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestNewManagedWorktreeBranchSanitizesRepoName(t *testing.T) {
	// "My Repo!!" sanitizes to "My-Repo-", then Trim(".-") drops the
	// trailing dash before the uuid separator is appended back.
	branch := newManagedWorktreeBranch("/path/to/My Repo!!")
	if !strings.HasPrefix(branch, "modu-code/My-Repo-") {
		t.Errorf("branch = %q, want it to start with a sanitized repo name", branch)
	}
	if strings.ContainsAny(branch, " !") {
		t.Errorf("branch name should not contain spaces or special characters: %q", branch)
	}
}

func TestNewManagedWorktreeBranchHandlesRootPath(t *testing.T) {
	branch := newManagedWorktreeBranch("/")
	if !strings.HasPrefix(branch, "modu-code/repo-") {
		t.Errorf("branch = %q, want it to fall back to \"repo\" for a root path", branch)
	}
}

func TestPathExists(t *testing.T) {
	dir := t.TempDir()
	if !pathExists(dir) {
		t.Error("pathExists should be true for an existing directory")
	}
	if pathExists(filepath.Join(dir, "missing")) {
		t.Error("pathExists should be false for a missing path")
	}
	if pathExists("") {
		t.Error("pathExists should be false for an empty path")
	}
}

func TestRemoveEmptyManagedParents(t *testing.T) {
	base := t.TempDir()
	nested := filepath.Join(base, "abc", "repo")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	// nested itself represents the worktree dir, already removed by the
	// caller in real usage; simulate that and confirm the now-empty parent
	// ("abc") is cleaned up but base itself is left alone.
	if err := os.Remove(nested); err != nil {
		t.Fatal(err)
	}
	removeEmptyManagedParents(nested, base)
	if _, err := os.Stat(filepath.Join(base, "abc")); !os.IsNotExist(err) {
		t.Errorf("expected the empty parent directory to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(base); err != nil {
		t.Errorf("the base directory itself should be left alone: %v", err)
	}
}

func TestRemoveEmptyManagedParentsStopsAtNonEmptyDir(t *testing.T) {
	base := t.TempDir()
	nested := filepath.Join(base, "abc", "def", "repo")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	// Leave a sibling file under "abc" so it's non-empty once "def" is gone.
	if err := os.WriteFile(filepath.Join(base, "abc", "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(base, "abc", "def")); err != nil {
		t.Fatal(err)
	}
	removeEmptyManagedParents(filepath.Join(base, "abc", "def", "repo"), base)
	if _, err := os.Stat(filepath.Join(base, "abc")); err != nil {
		t.Errorf("non-empty parent should survive, stat err=%v", err)
	}
}
