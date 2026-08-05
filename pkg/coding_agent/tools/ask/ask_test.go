package ask

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/openmodu/modu/pkg/types"
)

type fakeAsker struct {
	got    Request
	result Result
	err    error
	calls  int
}

func (f *fakeAsker) AskUser(_ context.Context, request Request) (Result, error) {
	f.calls++
	f.got = request
	return f.result, f.err
}

func resultText(t *testing.T, result types.ToolResult) string {
	t.Helper()
	var b strings.Builder
	for _, block := range result.Content {
		if text, ok := block.(*types.TextContent); ok {
			b.WriteString(text.Text)
		}
	}
	return b.String()
}

func questionArgs() map[string]any {
	return map[string]any{
		"questions": []any{
			map[string]any{
				"id":       "auth",
				"header":   "Auth method",
				"question": "Which auth should the endpoint use?",
				"options": []any{
					map[string]any{"label": "Session cookie", "description": "Matches the rest of the app"},
					map[string]any{"label": "Bearer token"},
				},
			},
		},
	}
}

func TestAskToolReturnsTheUsersAnswer(t *testing.T) {
	asker := &fakeAsker{result: Result{Answers: map[string]string{"auth": "Bearer token"}}}
	tool := NewTool(asker)

	result, err := tool.Execute(context.Background(), "call-1", questionArgs(), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", resultText(t, result))
	}

	if len(asker.got.Questions) != 1 {
		t.Fatalf("asker received %#v", asker.got)
	}
	question := asker.got.Questions[0]
	if question.ID != "auth" || question.Header != "Auth method" {
		t.Fatalf("question not parsed: %#v", question)
	}
	if len(question.Options) != 2 || question.Options[0].Description != "Matches the rest of the app" {
		t.Fatalf("options not parsed: %#v", question.Options)
	}

	text := resultText(t, result)
	if !strings.Contains(text, "Bearer token") {
		t.Fatalf("answer missing from result: %s", text)
	}
	if !strings.Contains(text, "Auth method") {
		t.Fatalf("result should echo what was asked so the transcript stands alone: %s", text)
	}
}

func TestAskToolReportsCancellationAsDistinctFromAnAnswer(t *testing.T) {
	asker := &fakeAsker{result: Result{Cancelled: true}}
	tool := NewTool(asker)

	result, err := tool.Execute(context.Background(), "call-1", questionArgs(), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	text := resultText(t, result)
	if !strings.Contains(strings.ToLower(text), "dismissed") {
		t.Fatalf("a dismissal must be reported as such, not as an answer: %s", text)
	}
	// The first option must not leak in as though the user had chosen it.
	if strings.Contains(text, "Session cookie") {
		t.Fatalf("cancellation must not imply the default option: %s", text)
	}
}

func TestAskToolDoesNotHangWithoutAHost(t *testing.T) {
	t.Run("no asker wired", func(t *testing.T) {
		result, err := NewTool(nil).Execute(context.Background(), "call-1", questionArgs(), nil)
		if err != nil {
			t.Fatalf("Execute should report this in the result, not as a Go error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected an error result so the model knows to decide for itself")
		}
	})

	t.Run("host declines to prompt", func(t *testing.T) {
		asker := &fakeAsker{err: errors.New("no host is available to ask the user")}
		result, err := NewTool(asker).Execute(context.Background(), "call-1", questionArgs(), nil)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected an error result")
		}
		if !strings.Contains(resultText(t, result), "best judgment") {
			t.Fatalf("the model needs to be told what to do instead: %s", resultText(t, result))
		}
	})
}

func TestAskToolRejectsUnusableArgs(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "no questions key", args: map[string]any{}},
		{name: "empty questions", args: map[string]any{"questions": []any{}}},
		{name: "wrong type", args: map[string]any{"questions": "a string"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asker := &fakeAsker{}
			result, err := NewTool(asker).Execute(context.Background(), "call-1", tt.args, nil)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !result.IsError {
				t.Fatalf("expected an error result, got %s", resultText(t, result))
			}
			if asker.calls != 0 {
				t.Fatal("the user should not be prompted for an unusable request")
			}
		})
	}
}

func TestAskToolPassesTheCallersContext(t *testing.T) {
	// The tool must hand its own ctx down, so interrupting the turn unblocks
	// a question the user never answered.
	var got context.Context
	asker := &fakeAsker{result: Result{Answers: map[string]string{"auth": "Bearer token"}}}
	tool := &Tool{asker: askerFunc(func(ctx context.Context, request Request) (Result, error) {
		got = ctx
		return asker.AskUser(ctx, request)
	})}

	type ctxKey string
	ctx := context.WithValue(context.Background(), ctxKey("marker"), "present")
	if _, err := tool.Execute(ctx, "call-1", questionArgs(), nil); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got == nil || got.Value(ctxKey("marker")) != "present" {
		t.Fatal("Execute's context should reach the asker unchanged")
	}
}

type askerFunc func(context.Context, Request) (Result, error)

func (f askerFunc) AskUser(ctx context.Context, request Request) (Result, error) {
	return f(ctx, request)
}
