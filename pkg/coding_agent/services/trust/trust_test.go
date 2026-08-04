package trust

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPersistentTrustUsesNearestAncestor(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child", "project")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "trust.json")
	manager, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetPersistent(root, Trusted); err != nil {
		t.Fatal(err)
	}
	if !manager.IsTrusted(child) {
		t.Fatal("trusted ancestor should apply to child")
	}
	if err := manager.SetPersistent(filepath.Join(root, "child"), Untrusted); err != nil {
		t.Fatal(err)
	}
	status := manager.Status(child)
	if status.Decision != Untrusted || status.Path != canonicalPath(filepath.Join(root, "child")) {
		t.Fatalf("nearest status = %#v", status)
	}

	reloaded, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Status(child).Decision; got != Untrusted {
		t.Fatalf("reloaded decision = %v", got)
	}
}

func TestSessionTrustIsNotPersisted(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(t.TempDir(), "trust.json")
	manager, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetSession(root); err != nil {
		t.Fatal(err)
	}
	status := manager.Status(root)
	if status.Decision != Trusted || !status.Session {
		t.Fatalf("session status = %#v", status)
	}
	reloaded, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Status(root).Decision; got != Undecided {
		t.Fatalf("session trust persisted unexpectedly: %v", got)
	}
}

func TestMalformedTrustStoreFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trust.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(path); err == nil {
		t.Fatal("expected malformed trust store error")
	}
}

func TestCurrentDirectoryCanBeTrustedByRelativePath(t *testing.T) {
	manager, err := New(filepath.Join(t.TempDir(), "trust.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetPersistent(".", Trusted); err != nil {
		t.Fatal(err)
	}
	if !manager.IsTrusted(".") {
		t.Fatal("relative current directory should resolve to a trusted absolute path")
	}
}
