package main

import (
	"context"
	"errors"
	"strings"

	coding_agent "github.com/openmodu/modu/pkg/coding_agent"
	modutui "github.com/openmodu/modu/pkg/modu-tui"
	"github.com/openmodu/modu/pkg/types"
)

type moduTUIPrompter struct {
	ctx    context.Context
	client modutui.Client
}

func (p *moduTUIPrompter) Confirm(title, body string, defaultYes bool) bool {
	defaultIndex := 1
	if defaultYes {
		defaultIndex = 0
	}
	choice := p.requestHumanPrompt(modutui.HumanPromptRequest{
		Title: title,
		Body:  body,
		Options: []modutui.HumanPromptOption{
			{Label: "Yes", Value: "yes"},
			{Label: "No", Value: "no"},
		},
		DefaultIndex: defaultIndex,
	})
	if choice == "" {
		return defaultYes
	}
	return choice == "yes"
}

func (p *moduTUIPrompter) Select(title string, options []string) string {
	if len(options) == 0 {
		return ""
	}
	promptOptions := make([]modutui.HumanPromptOption, 0, len(options))
	for _, option := range options {
		promptOptions = append(promptOptions, modutui.HumanPromptOption{Label: option, Value: option})
	}
	choice := p.requestHumanPrompt(modutui.HumanPromptRequest{
		Title:        title,
		Options:      promptOptions,
		DefaultIndex: 0,
	})
	if choice == "" {
		return options[0]
	}
	return choice
}

func (p *moduTUIPrompter) ApprovePlan(plan string, steps []string) string {
	body := strings.TrimSpace(plan)
	if len(steps) > 0 {
		body += "\n\n" + strings.Join(steps, "\n")
	}
	choice := p.requestHumanPrompt(modutui.HumanPromptRequest{
		Title: "Plan approval required",
		Body:  body,
		Options: []modutui.HumanPromptOption{
			{Label: "Approve", Value: "approve"},
			{Label: "Approve + auto", Value: "approve_auto"},
			{Label: "Reject", Value: "reject"},
		},
		DefaultIndex: 2,
	})
	switch choice {
	case "approve", "approve_auto":
		return choice
	default:
		return "reject: rejected in modu-tui"
	}
}

func (p *moduTUIPrompter) ApproveTool(toolName, toolCallID string, args map[string]any) (types.ToolApprovalDecision, error) {
	if p == nil {
		return types.ToolApprovalDeny, nil
	}
	ctx := p.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	decision, err := p.client.AskToolApproval(ctx, modutui.ToolApprovalRequest{
		ID:       toolCallID,
		ToolName: toolName,
		Summary:  "approval required: " + toolName,
		Detail:   toolInputFromArgs(toolName, args),
	})
	if errors.Is(err, modutui.ErrClientUnavailable) {
		return types.ToolApprovalDeny, nil
	}
	if err != nil {
		return types.ToolApprovalDeny, err
	}
	return toolApprovalDecisionToTypes(decision), nil
}

// Ask puts the model's questions to the user, one card at a time, and
// collects the answers by question ID.
//
// Each card is opened with DefaultIndex -1 so Esc resolves to "" rather than
// silently selecting an option: a dismissed question must reach the model as
// "declined to answer", not as consent to whatever happened to be first.
// (The host wizards keep their own explicit DefaultIndex, so their
// Esc-takes-the-default behavior is unchanged.)
func (p *moduTUIPrompter) Ask(ctx context.Context, request coding_agent.AskRequest) (coding_agent.AskResult, error) {
	if p == nil {
		return coding_agent.AskResult{}, coding_agent.ErrAskUnavailable
	}
	if ctx == nil {
		ctx = p.ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}

	answers := make(map[string]string, len(request.Questions))
	for _, question := range request.Questions {
		options := make([]modutui.HumanPromptOption, 0, len(question.Options))
		for _, option := range question.Options {
			label := option.Label
			if option.Description != "" {
				label += " — " + option.Description
			}
			// Value stays the bare label: it is what the model reads back,
			// so it should not carry the description text used for display.
			options = append(options, modutui.HumanPromptOption{Label: label, Value: option.Label})
		}
		title := question.Header
		if title == "" {
			title = "Question"
		}
		choice, err := p.client.AskChoice(ctx, modutui.HumanPromptRequest{
			ID:           question.ID,
			Title:        title,
			Body:         question.Question,
			Options:      options,
			DefaultIndex: -1,
		})
		if errors.Is(err, modutui.ErrClientUnavailable) {
			return coding_agent.AskResult{}, coding_agent.ErrAskUnavailable
		}
		if err != nil {
			return coding_agent.AskResult{}, err
		}
		if strings.TrimSpace(choice) == "" {
			// Dismissing one question abandons the round: answering the rest
			// would attribute a decision to a user who just opted out.
			return coding_agent.AskResult{Cancelled: true}, nil
		}
		answers[question.ID] = choice
	}
	return coding_agent.AskResult{Answers: answers}, nil
}

func (p *moduTUIPrompter) requestHumanPrompt(req modutui.HumanPromptRequest) string {
	if p == nil {
		return ""
	}
	ctx := p.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	choice, err := p.client.AskChoice(ctx, req)
	if err != nil {
		return ""
	}
	return choice
}

func toolApprovalDecisionToTypes(decision modutui.ToolApprovalDecision) types.ToolApprovalDecision {
	switch decision {
	case modutui.ToolApprovalAllow:
		return types.ToolApprovalAllow
	case modutui.ToolApprovalAllowAlways:
		return types.ToolApprovalAllowAlways
	case modutui.ToolApprovalDenyAlways:
		return types.ToolApprovalDenyAlways
	default:
		return types.ToolApprovalDeny
	}
}
