// Package ask provides the tool the model uses to put a question to the
// user and wait for their answer.
package ask

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openmodu/modu/pkg/coding_agent/tools/common"
	"github.com/openmodu/modu/pkg/types"
)

// Asker is the session-side hook the tool blocks on. It mirrors
// PlanModeManager: the tool owns argument shape and result formatting, the
// session owns reaching the host.
type Asker interface {
	// AskUser blocks until the user answers, the host declines to prompt, or
	// ctx is cancelled.
	AskUser(ctx context.Context, request Request) (Result, error)
}

// Request and Result mirror the session-level types. They are redeclared
// here so this package does not import the session package (which imports
// tools, so depending on it the other way would be a cycle).
type Request struct {
	Questions []Question
}

type Question struct {
	ID       string
	Header   string
	Question string
	Options  []Option
}

type Option struct {
	Label       string
	Description string
}

type Result struct {
	Answers   map[string]string
	Cancelled bool
}

type Tool struct {
	asker Asker
}

func NewTool(asker Asker) types.Tool {
	return &Tool{asker: asker}
}

func (t *Tool) Name() string  { return "ask_user_question" }
func (t *Tool) Label() string { return "Ask User" }

func (t *Tool) Description() string {
	return `Ask the user a question and wait for their answer.

Use this when you are blocked on a decision that is genuinely the user's to
make: one you cannot resolve from the request, the code, or sensible
defaults. Reserve it for choices where the answer changes what you do next —
not for decisions with an obvious default, and not for facts you could
verify yourself by reading the code.

Each question needs at least two concrete options. The user can always
decline to answer, in which case you should proceed with your best judgment
and say what you assumed.`
}

func (t *Tool) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"questions": map[string]any{
				"type":        "array",
				"minItems":    1,
				"maxItems":    4,
				"description": "The questions to put to the user.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id": map[string]any{
							"type":        "string",
							"description": "Stable key for this question's answer. Defaults to question1, question2, ...",
						},
						"header": map[string]any{
							"type":        "string",
							"description": "Short label for what is being decided, e.g. \"Auth method\".",
						},
						"question": map[string]any{
							"type":        "string",
							"description": "The question, phrased so the options below answer it.",
						},
						"options": map[string]any{
							"type":        "array",
							"minItems":    2,
							"description": "The choices offered. Put your recommendation first.",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"label": map[string]any{
										"type":        "string",
										"description": "The choice itself, a few words.",
									},
									"description": map[string]any{
										"type":        "string",
										"description": "What picking this means or implies.",
									},
								},
								"required": []string{"label"},
							},
						},
					},
					"required": []string{"question", "options"},
				},
			},
		},
		"required": []string{"questions"},
	}
}

func (t *Tool) Execute(ctx context.Context, toolCallID string, args map[string]any, onUpdate types.ToolUpdateCallback) (types.ToolResult, error) {
	if t.asker == nil {
		return errorResult("this session cannot prompt the user; decide with your best judgment and state your assumption"), nil
	}
	request, err := parseRequest(args)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	result, err := t.asker.AskUser(ctx, request)
	if err != nil {
		// Includes the headless case. Reported as a tool error rather than a
		// Go error so the model can adapt instead of the turn failing.
		return errorResult(err.Error() + "; decide with your best judgment and state your assumption"), nil
	}
	if result.Cancelled {
		return textResult("The user dismissed the question without answering. Proceed with your best judgment and say what you assumed."), nil
	}
	return textResult(formatAnswers(request, result)), nil
}

func parseRequest(args map[string]any) (Request, error) {
	raw, ok := args["questions"].([]any)
	if !ok || len(raw) == 0 {
		return Request{}, fmt.Errorf("questions is required and must be a non-empty array")
	}
	request := Request{Questions: make([]Question, 0, len(raw))}
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		question := Question{
			ID:       stringField(entry, "id"),
			Header:   stringField(entry, "header"),
			Question: stringField(entry, "question"),
		}
		rawOptions, _ := entry["options"].([]any)
		for _, rawOption := range rawOptions {
			optionMap, ok := rawOption.(map[string]any)
			if !ok {
				continue
			}
			label := stringField(optionMap, "label")
			if label == "" {
				continue
			}
			question.Options = append(question.Options, Option{
				Label:       label,
				Description: stringField(optionMap, "description"),
			})
		}
		request.Questions = append(request.Questions, question)
	}
	if len(request.Questions) == 0 {
		return Request{}, fmt.Errorf("no usable questions were provided")
	}
	return request, nil
}

func stringField(m map[string]any, key string) string {
	value, _ := m[key].(string)
	return strings.TrimSpace(value)
}

// formatAnswers renders the answers as readable lines rather than raw JSON,
// echoing each question so the transcript stays understandable on its own,
// and appends a JSON object so a programmatic reader can parse it.
func formatAnswers(request Request, result Result) string {
	var b strings.Builder
	b.WriteString("The user answered:\n")
	for _, question := range request.Questions {
		answer, ok := result.Answers[question.ID]
		if !ok || strings.TrimSpace(answer) == "" {
			answer = "(no answer)"
		}
		label := question.Header
		if label == "" {
			label = question.Question
		}
		fmt.Fprintf(&b, "- %s: %s\n", label, answer)
	}
	if encoded, err := json.Marshal(result.Answers); err == nil {
		b.WriteString("\nanswers: ")
		b.Write(encoded)
	}
	return strings.TrimSpace(b.String())
}

func textResult(text string) types.ToolResult {
	return types.ToolResult{
		Content: []types.ContentBlock{&types.TextContent{Type: "text", Text: text}},
	}
}

func errorResult(msg string) types.ToolResult {
	result := common.ErrorResult(msg)
	result.IsError = true
	return result
}
