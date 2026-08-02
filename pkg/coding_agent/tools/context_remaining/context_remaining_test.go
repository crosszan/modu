package context_remaining

import (
	"context"
	"strings"
	"testing"

	"github.com/openmodu/modu/pkg/types"
)

type fakeProvider struct {
	remaining int
	ok        bool
}

func (p *fakeProvider) TokensUntilCompaction() (int, bool) { return p.remaining, p.ok }

func mustText(t *testing.T, res types.ToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("result has no content blocks")
	}
	text, ok := res.Content[0].(*types.TextContent)
	if !ok {
		t.Fatalf("content block is %T, want *types.TextContent", res.Content[0])
	}
	return text.Text
}

func TestToolBasics(t *testing.T) {
	tool := New(&fakeProvider{})
	if tool.Name() != "get_context_remaining" {
		t.Errorf("Name() = %q, want get_context_remaining", tool.Name())
	}
}

func TestExecuteReturnsRemainingTokens(t *testing.T) {
	tool := New(&fakeProvider{remaining: 4200, ok: true})
	res, err := tool.Execute(context.Background(), "id", nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "4200") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
	details, ok := res.Details.(map[string]any)
	if !ok || details["tokens_left"] != 4200 {
		t.Errorf("Details[tokens_left] = %v, want 4200", res.Details)
	}
}

func TestExecuteUnknownWhenProviderReportsNotOK(t *testing.T) {
	tool := New(&fakeProvider{ok: false})
	res, err := tool.Execute(context.Background(), "id", nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "unknown") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
	details, ok := res.Details.(map[string]any)
	if !ok || details["tokens_left"] != nil {
		t.Errorf("Details[tokens_left] should be nil when unknown, got %v", res.Details)
	}
}

func TestExecuteUnknownWithNilProvider(t *testing.T) {
	tool := New(nil)
	res, err := tool.Execute(context.Background(), "id", nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "unknown") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}
