package common

import (
	"strings"
	"testing"
)

func TestTruncateHeadUnderLimitsReturnsUnchanged(t *testing.T) {
	content := "line1\nline2\nline3"
	got := TruncateHead(content, TruncateOptions{MaxLines: 10, MaxBytes: 1000})
	if got.WasTruncated {
		t.Fatalf("expected no truncation, got %+v", got)
	}
	if got.Content != content {
		t.Errorf("Content = %q, want unchanged %q", got.Content, content)
	}
	if got.OrigLines != 3 || got.KeptLines != 3 {
		t.Errorf("OrigLines/KeptLines = %d/%d, want 3/3", got.OrigLines, got.KeptLines)
	}
}

func TestTruncateHeadByLineCount(t *testing.T) {
	content := "1\n2\n3\n4\n5"
	got := TruncateHead(content, TruncateOptions{MaxLines: 2, MaxBytes: 1000})
	if !got.WasTruncated {
		t.Fatal("expected truncation")
	}
	if got.Content != "1\n2" {
		t.Errorf("Content = %q, want %q", got.Content, "1\n2")
	}
	if got.KeptLines != 2 || got.OrigLines != 5 {
		t.Errorf("OrigLines/KeptLines = %d/%d, want 5/2", got.OrigLines, got.KeptLines)
	}
	if !strings.Contains(got.Message, "truncated") {
		t.Errorf("Message should mention truncation, got %q", got.Message)
	}
}

// Regression test: a single line with no newlines that exceeds MaxBytes must
// still be truncated. The line-count-based fallback used to miss this case
// entirely, silently returning the whole untruncated content because cutting
// bytes out of one line doesn't change the line count.
func TestTruncateHeadEnforcesMaxBytesOnSingleLine(t *testing.T) {
	content := strings.Repeat("a", 1000)
	got := TruncateHead(content, TruncateOptions{MaxLines: 1000, MaxBytes: 100})
	if !got.WasTruncated {
		t.Fatal("expected truncation for a single line exceeding MaxBytes")
	}
	if len(got.Content) > 100 {
		t.Errorf("Content is %d bytes, want <= 100", len(got.Content))
	}
}

func TestTruncateHeadByByteLimitAcrossMultipleLines(t *testing.T) {
	// Each line is 11 bytes with its newline ("0123456789\n"); MaxBytes=25
	// fits two complete lines (22 bytes) but not three (33 bytes), so the
	// partial third line is dropped and the first two are kept whole.
	content := "0123456789\n0123456789\n0123456789"
	got := TruncateHead(content, TruncateOptions{MaxLines: 1000, MaxBytes: 25})
	if !got.WasTruncated {
		t.Fatal("expected truncation")
	}
	want := "0123456789\n0123456789"
	if got.Content != want {
		t.Errorf("Content = %q, want %q", got.Content, want)
	}
}

func TestTruncateTailUnderLimitsReturnsUnchanged(t *testing.T) {
	content := "line1\nline2\nline3"
	got := TruncateTail(content, TruncateOptions{MaxLines: 10, MaxBytes: 1000})
	if got.WasTruncated {
		t.Fatalf("expected no truncation, got %+v", got)
	}
	if got.Content != content {
		t.Errorf("Content = %q, want unchanged %q", got.Content, content)
	}
}

func TestTruncateTailByLineCountKeepsLastLines(t *testing.T) {
	content := "1\n2\n3\n4\n5"
	got := TruncateTail(content, TruncateOptions{MaxLines: 2, MaxBytes: 1000})
	if !got.WasTruncated {
		t.Fatal("expected truncation")
	}
	if got.Content != "4\n5" {
		t.Errorf("Content = %q, want %q", got.Content, "4\n5")
	}
	if got.KeptLines != 2 || got.OrigLines != 5 {
		t.Errorf("OrigLines/KeptLines = %d/%d, want 5/2", got.OrigLines, got.KeptLines)
	}
}

// Regression test: same bug as TruncateHead, mirrored for the tail strategy
// bash output uses.
func TestTruncateTailEnforcesMaxBytesOnSingleLine(t *testing.T) {
	content := strings.Repeat("b", 1000)
	got := TruncateTail(content, TruncateOptions{MaxLines: 1000, MaxBytes: 100})
	if !got.WasTruncated {
		t.Fatal("expected truncation for a single line exceeding MaxBytes")
	}
	if len(got.Content) > 100 {
		t.Errorf("Content is %d bytes, want <= 100", len(got.Content))
	}
}

func TestTruncateTailByByteLimitAcrossMultipleLines(t *testing.T) {
	// Symmetric to the head case: the last two complete lines fit in 25
	// bytes, so the partial (cut-off) first line is dropped.
	content := "0123456789\n0123456789\n0123456789"
	got := TruncateTail(content, TruncateOptions{MaxLines: 1000, MaxBytes: 25})
	if !got.WasTruncated {
		t.Fatal("expected truncation")
	}
	want := "0123456789\n0123456789"
	if got.Content != want {
		t.Errorf("Content = %q, want %q", got.Content, want)
	}
}

func TestTruncateHeadAndTailDefaultOptionsOnZeroValues(t *testing.T) {
	// Zero MaxLines/MaxBytes should fall back to the package defaults rather
	// than truncating everything to nothing.
	content := "hello\nworld"
	head := TruncateHead(content, TruncateOptions{})
	if head.WasTruncated || head.Content != content {
		t.Errorf("TruncateHead with zero options should use defaults and not truncate short content, got %+v", head)
	}
	tail := TruncateTail(content, TruncateOptions{})
	if tail.WasTruncated || tail.Content != content {
		t.Errorf("TruncateTail with zero options should use defaults and not truncate short content, got %+v", tail)
	}
}

func TestTruncateLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		maxChars int
		want     string
	}{
		{"under limit unchanged", "short", 10, "short"},
		{"exact limit unchanged", "1234567890", 10, "1234567890"},
		{"over limit truncated with ellipsis", "12345678901234", 10, "1234567890..."},
		{"zero maxChars uses default", strings.Repeat("x", 600), 0, strings.Repeat("x", DefaultMaxLineLen) + "..."},
		{"multi-byte runes counted as runes not bytes", strings.Repeat("中", 5), 3, "中中中..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TruncateLine(tt.line, tt.maxChars); got != tt.want {
				t.Errorf("TruncateLine(%q, %d) = %q, want %q", tt.line, tt.maxChars, got, tt.want)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0B"},
		{500, "500B"},
		{1024, "1.0KB"},
		{1536, "1.5KB"},
		{1024 * 1024, "1.0MB"},
		{1024 * 1024 * 1024, "1.0GB"},
		{int64(2.5 * 1024 * 1024 * 1024), "2.5GB"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := FormatSize(tt.bytes); got != tt.want {
				t.Errorf("FormatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}
