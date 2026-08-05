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
	// textRequests records the free-text overlays opened after picking
	// "Other"; typed answers them the same way reply answers a card.
	textRequests []modutui.HumanTextRequest
	typed        func(n int, req modutui.HumanTextRequest) string
}

func (h *askPrompterHarness) prompter() *moduTUIPrompter {
	client := modutui.NewClient(func(msg any) {
		switch request := msg.(type) {
		case modutui.RequestHumanPromptMsg:
			n := len(h.requests)
			h.requests = append(h.requests, request.Request)
			go func() { request.Respond <- h.reply(n, request.Request) }()
		case modutui.RequestHumanTextMsg:
			n := len(h.textRequests)
			h.textRequests = append(h.textRequests, request.Request)
			go func() {
				if h.typed == nil {
					request.Respond <- ""
					return
				}
				request.Respond <- h.typed(n, request.Request)
			}()
		}
	})
	return &moduTUIPrompter{ctx: context.Background(), client: client}
}

// pickOther answers a card by selecting the synthetic "Other" entry, which
// is always appended last.
func pickOther(req modutui.HumanPromptRequest) string {
	return req.Options[len(req.Options)-1].Value
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

func TestPrompterAskAlwaysOffersAFreeTextEscapeHatch(t *testing.T) {
	h := &askPrompterHarness{reply: func(_ int, req modutui.HumanPromptRequest) string {
		return req.Options[0].Value
	}}
	if _, err := h.prompter().Ask(context.Background(), askRequest(twoOptionQuestion("auth", "Auth method"))); err != nil {
		t.Fatalf("Ask: %v", err)
	}

	options := h.requests[0].Options
	// The model supplied two; the third is the always-present escape hatch,
	// so the user is never boxed into the model's framing.
	if len(options) != 3 {
		t.Fatalf("expected the model's options plus Other, got %#v", options)
	}
	if options[len(options)-1].Label != askOtherLabel {
		t.Fatalf("last option = %q, want the Other entry", options[len(options)-1].Label)
	}
}

func TestPrompterAskReturnsTypedTextWhenOtherIsChosen(t *testing.T) {
	h := &askPrompterHarness{
		reply: func(_ int, req modutui.HumanPromptRequest) string { return pickOther(req) },
		typed: func(int, modutui.HumanTextRequest) string { return "neither, use mTLS" },
	}
	result, err := h.prompter().Ask(context.Background(), askRequest(twoOptionQuestion("auth", "Auth method")))
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if result.Cancelled {
		t.Fatal("typing an answer is answering, not cancelling")
	}
	if result.Answers["auth"] != "neither, use mTLS" {
		t.Fatalf("answers = %#v, want the typed text", result.Answers)
	}
	if len(h.textRequests) != 1 {
		t.Fatalf("expected one text overlay, got %d", len(h.textRequests))
	}
	// The text overlay has to restate the question; by then the card that
	// asked it is gone from the screen.
	if !strings.Contains(h.textRequests[0].Body, "Which one?") {
		t.Fatalf("text overlay body = %q, want it to restate the question", h.textRequests[0].Body)
	}
}

func TestPrompterAskTreatsAnEmptyOtherAsBackingOut(t *testing.T) {
	h := &askPrompterHarness{
		reply: func(_ int, req modutui.HumanPromptRequest) string { return pickOther(req) },
		typed: func(int, modutui.HumanTextRequest) string { return "   " },
	}
	result, err := h.prompter().Ask(context.Background(), askRequest(twoOptionQuestion("auth", "Auth method")))
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if !result.Cancelled {
		t.Fatal("submitting nothing must read as declining, not as a blank answer")
	}
	if len(result.Answers) != 0 {
		t.Fatalf("answers = %#v, want none", result.Answers)
	}
}

func TestPrompterAskOtherSentinelSurvivesAColludingOptionLabel(t *testing.T) {
	// A model is free to author an option whose label happens to equal the
	// sentinel; the synthetic entry must still be distinguishable from it.
	question := coding_agent.AskQuestion{
		ID:       "auth",
		Header:   "Auth method",
		Question: "Which one?",
		Options: []coding_agent.AskOption{
			{Label: "__modu_ask_other__"},
			{Label: "Bearer token"},
		},
	}
	h := &askPrompterHarness{
		reply: func(_ int, req modutui.HumanPromptRequest) string { return req.Options[0].Value },
		typed: func(int, modutui.HumanTextRequest) string { return "should not be reached" },
	}
	result, err := h.prompter().Ask(context.Background(), askRequest(question))
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if result.Answers["auth"] != "__modu_ask_other__" {
		t.Fatalf("answers = %#v, want the model's own option", result.Answers)
	}
	if len(h.textRequests) != 0 {
		t.Fatal("picking the model's option must not open the free-text overlay")
	}
}

func TestPrompterAskShowsProgressAcrossMultipleQuestions(t *testing.T) {
	h := &askPrompterHarness{reply: func(_ int, req modutui.HumanPromptRequest) string {
		return req.Options[0].Value
	}}
	if _, err := h.prompter().Ask(context.Background(), askRequest(
		twoOptionQuestion("auth", "Auth method"),
		twoOptionQuestion("store", "Storage"),
	)); err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if !strings.Contains(h.requests[0].Title, "1/2") || !strings.Contains(h.requests[1].Title, "2/2") {
		t.Fatalf("titles = %q, %q; want progress so the user knows how many remain", h.requests[0].Title, h.requests[1].Title)
	}
}

func TestPrompterAskOmitsProgressForASingleQuestion(t *testing.T) {
	h := &askPrompterHarness{reply: func(_ int, req modutui.HumanPromptRequest) string {
		return req.Options[0].Value
	}}
	if _, err := h.prompter().Ask(context.Background(), askRequest(twoOptionQuestion("auth", "Auth method"))); err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if strings.Contains(h.requests[0].Title, "1/1") {
		t.Fatalf("title = %q, want no progress counter for a lone question", h.requests[0].Title)
	}
}
