package env

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLine(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantKey   string
		wantValue string
		wantErr   bool
	}{
		{"simple", "KEY=value", "KEY", "value", false},
		{"with export prefix", "export KEY=value", "KEY", "value", false},
		{"whitespace around key and value", "  KEY  =  value  ", "KEY", "value", false},
		{"double-quoted value", `KEY="hello world"`, "KEY", "hello world", false},
		{"single-quoted value", `KEY='hello world'`, "KEY", "hello world", false},
		{"value contains equals sign", "KEY=a=b=c", "KEY", "a=b=c", false},
		{"double-quoted escapes newline and tab", `KEY="line1\nline2\ttabbed"`, "KEY", "line1\nline2\ttabbed", false},
		{"double-quoted escapes backslash and quote", `KEY="a\\b\"c"`, "KEY", `a\b"c`, false},
		{"single-quoted backslash sequences stay literal", `KEY='C:\new\temp'`, "KEY", `C:\new\temp`, false},
		{"unquoted backslash sequences stay literal", `KEY=C:\new\temp`, "KEY", `C:\new\temp`, false},
		{"empty value", "KEY=", "KEY", "", false},
		{"no equals sign", "INVALID", "", "", true},
		{"empty key", "=value", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, value, err := parseLine(tt.line)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseLine(%q) error = %v, wantErr %v", tt.line, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if key != tt.wantKey || value != tt.wantValue {
				t.Errorf("parseLine(%q) = (%q, %q), want (%q, %q)", tt.line, key, value, tt.wantKey, tt.wantValue)
			}
		})
	}
}

func TestLoadReadsFileIntoEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "FOO=bar\n# a comment\n\nBAZ=qux\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FOO", "")
	t.Setenv("BAZ", "")
	_ = os.Unsetenv("FOO")
	_ = os.Unsetenv("BAZ")

	if err := Load(WithDir(dir)); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if Get("FOO") != "bar" {
		t.Errorf("FOO = %q, want %q", Get("FOO"), "bar")
	}
	if Get("BAZ") != "qux" {
		t.Errorf("BAZ = %q, want %q", Get("BAZ"), "qux")
	}
}

func TestLoadMissingFileIsNotAnErrorByDefault(t *testing.T) {
	dir := t.TempDir()
	if err := Load(WithDir(dir), WithFile("does-not-exist.env")); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

func TestLoadMissingFileWithRequiredErrors(t *testing.T) {
	dir := t.TempDir()
	err := Load(WithDir(dir), WithFile("does-not-exist.env"), WithRequired())
	if err == nil {
		t.Fatal("expected an error for a required file that does not exist")
	}
}

func TestLoadDoesNotOverrideExistingByDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("KEY=from-file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KEY", "from-process")

	if err := Load(WithDir(dir)); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if Get("KEY") != "from-process" {
		t.Errorf("KEY = %q, want the pre-existing value preserved (%q)", Get("KEY"), "from-process")
	}
}

func TestLoadWithOverrideReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("KEY=from-file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KEY", "from-process")

	if err := Load(WithDir(dir), WithOverride()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if Get("KEY") != "from-file" {
		t.Errorf("KEY = %q, want overridden to %q", Get("KEY"), "from-file")
	}
}

func TestLoadInvalidLineReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("NOT_VALID_LINE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Load(WithDir(dir)); err == nil {
		t.Fatal("expected an error for a malformed line")
	}
}

func TestGetDefault(t *testing.T) {
	_ = os.Unsetenv("MODU_TEST_MISSING_KEY")
	if got := GetDefault("MODU_TEST_MISSING_KEY", "fallback"); got != "fallback" {
		t.Errorf("GetDefault() = %q, want %q", got, "fallback")
	}
	t.Setenv("MODU_TEST_PRESENT_KEY", "actual")
	if got := GetDefault("MODU_TEST_PRESENT_KEY", "fallback"); got != "actual" {
		t.Errorf("GetDefault() = %q, want %q", got, "actual")
	}
}

func TestGetRequired(t *testing.T) {
	_ = os.Unsetenv("MODU_TEST_MISSING_KEY")
	if _, err := GetRequired("MODU_TEST_MISSING_KEY"); err == nil {
		t.Error("expected an error for a missing required key")
	}
	t.Setenv("MODU_TEST_PRESENT_KEY", "actual")
	got, err := GetRequired("MODU_TEST_PRESENT_KEY")
	if err != nil || got != "actual" {
		t.Errorf("GetRequired() = (%q, %v), want (%q, nil)", got, err, "actual")
	}
}

func TestMustGetPanicsWhenMissing(t *testing.T) {
	_ = os.Unsetenv("MODU_TEST_MISSING_KEY")
	defer func() {
		if recover() == nil {
			t.Error("expected MustGet to panic for a missing key")
		}
	}()
	MustGet("MODU_TEST_MISSING_KEY")
}

func TestMustLoadPanicsOnRequiredMissingFile(t *testing.T) {
	dir := t.TempDir()
	defer func() {
		if recover() == nil {
			t.Error("expected MustLoad to panic when the required file is missing")
		}
	}()
	MustLoad(WithDir(dir), WithFile("missing.env"), WithRequired())
}

func TestSetAndGet(t *testing.T) {
	if err := Set("MODU_TEST_SET_KEY", "value"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := Get("MODU_TEST_SET_KEY"); got != "value" {
		t.Errorf("Get() = %q, want %q", got, "value")
	}
}
