package spellcheck

import (
	"strings"
	"testing"
)

func TestCheckerCheckSkipsNonProseTokens(t *testing.T) {
	checker, err := New(strings.NewReader("SET UTF-8\n"), strings.NewReader("3\nhello\nworld\nthis\n"))
	if err != nil {
		t.Fatal(err)
	}

	text := "hello wrld /helo @wrld https://wrld.test dir/wrld main.go hello_world v2 `wrld` 中文"
	issues := checker.Check(text)
	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want only wrld in prose", issues)
	}
	if issues[0].Word != "wrld" || issues[0].Start != 6 || issues[0].End != 10 {
		t.Fatalf("issue = %#v, want wrld at rune range [6,10)", issues[0])
	}
}

func TestCheckerSuggest(t *testing.T) {
	checker, err := New(strings.NewReader("SET UTF-8\n"), strings.NewReader("2\nhello\nworld\n"))
	if err != nil {
		t.Fatal(err)
	}

	suggestions, err := checker.Suggest("wrld", 5)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(suggestions, "world") {
		t.Fatalf("suggestions = %#v, want world", suggestions)
	}
}

func TestBundledEnglishDictionary(t *testing.T) {
	checker, err := NewEnglish()
	if err != nil {
		t.Fatal(err)
	}
	issues := checker.Check("hello wrld")
	if len(issues) != 1 || issues[0].Word != "wrld" {
		t.Fatalf("bundled dictionary issues = %#v, want wrld", issues)
	}
	suggestions, err := checker.Suggest("wrld", 5)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(suggestions, "world") {
		t.Fatalf("bundled dictionary suggestions = %#v, want world", suggestions)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
