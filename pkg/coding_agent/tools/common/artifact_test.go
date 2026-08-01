package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewArtifactStoreRejectsEmptyDir(t *testing.T) {
	if s := NewArtifactStore(""); s != nil {
		t.Errorf("NewArtifactStore(\"\") = %+v, want nil", s)
	}
	if s := NewArtifactStore("   "); s != nil {
		t.Errorf("NewArtifactStore(whitespace) = %+v, want nil", s)
	}
}

func TestArtifactStorePutAndFind(t *testing.T) {
	dir := t.TempDir()
	store := NewArtifactStore(dir)
	if store == nil {
		t.Fatal("NewArtifactStore returned nil for a valid dir")
	}

	ref, err := store.Put("call-1", "output", []byte("hello world"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if ref.Bytes != len("hello world") {
		t.Errorf("Bytes = %d, want %d", ref.Bytes, len("hello world"))
	}
	data, err := os.ReadFile(ref.Path)
	if err != nil {
		t.Fatalf("reading artifact file: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("file content = %q, want %q", string(data), "hello world")
	}

	found, err := store.Find("call-1")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found.Path != ref.Path {
		t.Errorf("Find().Path = %q, want %q", found.Path, ref.Path)
	}
}

func TestArtifactStoreFindMissingReturnsError(t *testing.T) {
	dir := t.TempDir()
	store := NewArtifactStore(dir)
	if _, err := store.Find("does-not-exist"); err == nil {
		t.Error("expected an error for a call_id with no artifact")
	}
}

func TestArtifactStoreNilReceiverIsSafe(t *testing.T) {
	var store *ArtifactStore
	if _, err := store.Put("id", "name", []byte("x")); err == nil {
		t.Error("expected an error from a nil ArtifactStore.Put")
	}
	if _, err := store.Find("id"); err == nil {
		t.Error("expected an error from a nil ArtifactStore.Find")
	}
}

func TestArtifactStorePutSanitizesUnsafeIDAndName(t *testing.T) {
	dir := t.TempDir()
	store := NewArtifactStore(dir)
	ref, err := store.Put("../../etc/passwd", "../secrets", []byte("x"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	// The resulting path must stay inside the artifact directory — a
	// traversal-looking id/name must not escape it.
	absDir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	absPath, err := filepath.Abs(ref.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(absPath, absDir+string(os.PathSeparator)) {
		t.Errorf("artifact path %q escaped the store directory %q", absPath, absDir)
	}
}

func TestPreviewTextUntruncatedHasNoArtifact(t *testing.T) {
	store := NewArtifactStore(t.TempDir())
	preview := PreviewText("short output", TextPreviewOptions{
		ToolCallID:    "call-1",
		ArtifactStore: store,
		MaxLines:      100,
		MaxBytes:      1000,
	})
	if preview.Text != "short output" {
		t.Errorf("Text = %q, want unchanged", preview.Text)
	}
	output, ok := preview.Details["output"].(map[string]any)
	if !ok {
		t.Fatal("Details[\"output\"] missing or wrong type")
	}
	if output["truncated"] != false {
		t.Errorf("truncated = %v, want false", output["truncated"])
	}
	if _, hasArtifact := output["artifactId"]; hasArtifact {
		t.Error("untruncated output should not write an artifact")
	}
}

func TestPreviewTextTruncatedWritesArtifact(t *testing.T) {
	dir := t.TempDir()
	store := NewArtifactStore(dir)
	raw := strings.Repeat("line\n", 100)
	preview := PreviewText(raw, TextPreviewOptions{
		ToolCallID:    "call-1",
		ArtifactName:  "output",
		ArtifactStore: store,
		Strategy:      PreviewTail,
		MaxLines:      5,
		MaxBytes:      100000,
	})
	output, ok := preview.Details["output"].(map[string]any)
	if !ok {
		t.Fatal("Details[\"output\"] missing or wrong type")
	}
	if output["truncated"] != true {
		t.Fatalf("truncated = %v, want true", output["truncated"])
	}
	artifactPath, ok := output["artifactPath"].(string)
	if !ok || artifactPath == "" {
		t.Fatal("expected an artifactPath to be recorded")
	}
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("reading artifact: %v", err)
	}
	if string(data) != raw {
		t.Error("artifact should contain the full untruncated raw text")
	}
}

func TestPreviewTextWithoutArtifactStoreStillTruncates(t *testing.T) {
	raw := strings.Repeat("line\n", 100)
	preview := PreviewText(raw, TextPreviewOptions{
		ToolCallID: "call-1",
		Strategy:   PreviewTail,
		MaxLines:   5,
		MaxBytes:   100000,
	})
	output := preview.Details["output"].(map[string]any)
	if output["truncated"] != true {
		t.Fatal("expected truncation even without an artifact store")
	}
	if _, hasErr := output["artifactError"]; hasErr {
		t.Error("no artifact store configured should not report an artifactError")
	}
	if _, hasID := output["artifactId"]; hasID {
		t.Error("no artifact store configured should not report an artifactId")
	}
}

func TestPreviewTextFromPartialVisibleIsMarkedTruncated(t *testing.T) {
	// PreviewTextFrom lets a caller show less than the raw content (e.g. a
	// redacted view) even when the visible portion itself fits comfortably —
	// that should still be reported as "truncated" since raw != visible.
	preview := PreviewTextFrom("full raw content", "partial", TextPreviewOptions{
		ToolCallID: "call-1",
		MaxLines:   100,
		MaxBytes:   1000,
	})
	output := preview.Details["output"].(map[string]any)
	if output["truncated"] != true {
		t.Error("raw != visible should be reported as truncated even if the visible text itself fits")
	}
	if output["rawBytes"] != len("full raw content") {
		t.Errorf("rawBytes = %v, want %d", output["rawBytes"], len("full raw content"))
	}
}
