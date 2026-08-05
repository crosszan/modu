package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	coding_agent "github.com/openmodu/modu/pkg/coding_agent"
	modutui "github.com/openmodu/modu/pkg/modu-tui"
)

// askPrompterHarness answers HumanPrompt requests the way the TUI would,
// letting the prompter's own logic run against real modutui messages.
type askPrompterHarness struct {
	requests []modutui.HumanPromptRequest
	// reply picks what the "user" does for the nth request; returning ""
	// stands for dismissing the card with Esc.
	reply func(n int, req modutui.HumanPromptRequest) string
}

func (h *askPrompterHarness) prompter() *moduTUIPrompter {
	client := modutui.NewClient(func(msg any) {
		request, ok := msg.(modutui.RequestHumanPromptMsg)
		if !ok {
			return
		}
		n := len(h.requests)
		h.requests = append(h.requests, request.Request)
		go func() { request.Respond <- h.reply(n, request.Request) }()
	})
	return &moduTUIPrompter{ctx: context.Background(), client: client}
}

func askRequest(questions ...coding_agent.AskQuestion) coding_agent.AskRequest {
	return coding_agent.AskRequest{Questions: questions}
}

func twoOptionQuestion(id, header string) coding_agent.AskQuestion {
	return coding_agent.AskQuestion{
		ID:       id,
		Header:   header,
		Question: "Which one?",
		Options: []coding_agent.AskOption{
			{Label: "First", Description: "the recommended one"},
			{Label: "Second"},
		},
	}
}

func TestPrompterAskCollectsAnswersByQuestionID(t *testing.T) {
	h := &askPrompterHarness{reply: func(n int, req modutui.HumanPromptRequest) string {
		return req.Options[1].Value
	}}
	result, err := h.prompter().Ask(context.Background(), askRequest(
		twoOptionQuestion("auth", "Auth method"),
		twoOptionQuestion("store", "Storage"),
	))
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if result.Cancelled {
		t.Fatal("answering every question should not report a cancellation")
	}
	if result.Answers["auth"] != "Second" || result.Answers["store"] != "Second" {
		t.Fatalf("answers = %#v", result.Answers)
	}
	if len(h.requests) != 2 {
		t.Fatalf("expected one card per question, got %d", len(h.requests))
	}
}

func TestPrompterAskCardsCannotBeEscapedIntoADefaultAnswer(t *testing.T) {
	h := &askPrompterHarness{reply: func(int, modutui.HumanPromptRequest) string { return "" }}
	result, err := h.prompter().Ask(context.Background(), askRequest(twoOptionQuestion("auth", "Auth method")))
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if !result.Cancelled {
		t.Fatal("dismissing the card must report a cancellation, not an answer")
	}
	if len(result.Answers) != 0 {
		t.Fatalf("a dismissed round should carry no answers, got %#v", result.Answers)
	}

	// DefaultIndex -1 is what makes Esc resolve to "" instead of silently
	// selecting an option the user never chose.
	if got := h.requests[0].DefaultIndex; got != -1 {
		t.Fatalf("DefaultIndex = %d, want -1 so Esc cannot imply consent", got)
	}
}

func TestPrompterAskStopsAtTheFirstDismissal(t *testing.T) {
	h := &askPrompterHarness{reply: func(n int, req modutui.HumanPromptRequest) string {
		if n == 0 {
			return ""
		}
		return req.Options[0].Value
	}}
	result, err := h.prompter().Ask(context.Background(), askRequest(
		twoOptionQuestion("auth", "Auth method"),
		twoOptionQuestion("store", "Storage"),
	))
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if !result.Cancelled {
		t.Fatal("expected the round to be cancelled")
	}
	if len(h.requests) != 1 {
		t.Fatalf("opting out of one question should abandon the round, got %d cards", len(h.requests))
	}
}

func TestPrompterAskShowsDescriptionsButAnswersWithTheBareLabel(t *testing.T) {
	h := &askPrompterHarness{reply: func(_ int, req modutui.HumanPromptRequest) string {
		return req.Options[0].Value
	}}
	result, err := h.prompter().Ask(context.Background(), askRequest(twoOptionQuestion("auth", "Auth method")))
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if !strings.Contains(h.requests[0].Options[0].Label, "the recommended one") {
		t.Fatalf("the description should be visible on the card: %q", h.requests[0].Options[0].Label)
	}
	if result.Answers["auth"] != "First" {
		t.Fatalf("the model should read back the bare label, got %q", result.Answers["auth"])
	}
}

func TestPrompterAskWithoutAClientReportsUnavailable(t *testing.T) {
	p := &moduTUIPrompter{ctx: context.Background()}
	_, err := p.Ask(context.Background(), askRequest(twoOptionQuestion("auth", "Auth method")))
	if !errors.Is(err, coding_agent.ErrAskUnavailable) {
		t.Fatalf("err = %v, want ErrAskUnavailable", err)
	}
}

func TestPrompterAskPropagatesContextCancellation(t *testing.T) {
	// A card nobody answers must not pin the turn: cancelling the context
	// has to unblock Ask.
	client := modutui.NewClient(func(any) {})
	p := &moduTUIPrompter{ctx: context.Background(), client: client}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Ask(ctx, askRequest(twoOptionQuestion("auth", "Auth method"))); err == nil {
		t.Fatal("expected the cancelled context to surface as an error")
	}
}
