package common

import (
	"testing"

	"github.com/openmodu/modu/pkg/types"
)

func TestErrorResult(t *testing.T) {
	res := ErrorResult("boom")
	if len(res.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(res.Content))
	}
	text, ok := res.Content[0].(*types.TextContent)
	if !ok {
		t.Fatalf("expected *types.TextContent, got %T", res.Content[0])
	}
	if text.Text != "boom" {
		t.Errorf("Text = %q, want %q", text.Text, "boom")
	}
}

func TestToInt(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want int
	}{
		{"int", 5, 5},
		{"negative int", -3, -3},
		{"int64", int64(42), 42},
		{"float64", 3.9, 3}, // truncates, does not round
		{"float32", float32(2.9), 2},
		{"string is unsupported", "5", 0},
		{"nil is unsupported", nil, 0},
		{"bool is unsupported", true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToInt(tt.in); got != tt.want {
				t.Errorf("ToInt(%#v) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestToSemanticInt(t *testing.T) {
	tests := []struct {
		name     string
		in       any
		want     int
		wantOK   bool
	}{
		{"int", 5, 5, true},
		{"int64", int64(7), 7, true},
		{"float64", 3.9, 3, true},
		{"float32", float32(2.0), 2, true},
		{"numeric string", "42", 42, true},
		{"negative numeric string", "-42", -42, true},
		{"decimal string", "3.9", 3, true},
		{"empty string", "", 0, false},
		{"non-numeric string", "abc", 0, false},
		{"numeric with trailing text", "5s", 0, false},
		{"numeric with leading whitespace", " 5", 0, false},
		{"nil", nil, 0, false},
		{"bool", true, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ToSemanticInt(tt.in)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("ToSemanticInt(%#v) = (%d, %v), want (%d, %v)", tt.in, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestToSemanticBool(t *testing.T) {
	tests := []struct {
		name   string
		in     any
		want   bool
		wantOK bool
	}{
		{"true bool", true, true, true},
		{"false bool", false, false, true},
		{"true string", "true", true, true},
		{"false string", "false", false, true},
		{"True capitalized is not accepted", "True", false, false},
		{"1 string is not accepted", "1", false, false},
		{"empty string", "", false, false},
		{"nil", nil, false, false},
		{"int", 1, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ToSemanticBool(tt.in)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("ToSemanticBool(%#v) = (%v, %v), want (%v, %v)", tt.in, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
