package prompts

import (
	"reflect"
	"testing"
)

func TestSplitArgsUsesShellQuoting(t *testing.T) {
	got, err := SplitArgs(`Button "click handler" 'aria label' ""`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Button", "click handler", "aria label", ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SplitArgs() = %#v, want %#v", got, want)
	}
}

func TestSplitArgsRejectsUnterminatedQuote(t *testing.T) {
	if _, err := SplitArgs(`Button "click handler`); err == nil {
		t.Fatal("expected unterminated quote to fail")
	}
}

func TestExpandTemplateDoesNotExpandDollarInDefault(t *testing.T) {
	if got := ExpandTemplate("${1:-pay $5}", nil); got != "pay $5" {
		t.Fatalf("default was recursively expanded: %q", got)
	}
}

func TestExpandTemplateInvalidSlicesAreEmpty(t *testing.T) {
	for _, template := range []string{"${@:0}", "${@:9}", "${@:2:-1}", "${1:2}"} {
		if got := ExpandTemplate(template, []string{"a", "b"}); got != "" {
			t.Fatalf("ExpandTemplate(%q) = %q, want empty", template, got)
		}
	}
}

func TestExpandTemplateAppendsTokenizedArgumentsWhenNoPlaceholder(t *testing.T) {
	if got := (&Template{Content: "Base prompt"}).Expand(`one "two three"`); got != "Base prompt\n\none two three" {
		t.Fatalf("bare template expansion = %q", got)
	}
}
