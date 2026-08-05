package coding_agent

import (
	"context"
	"errors"
	"testing"
)

func validAskRequest() AskRequest {
	return AskRequest{Questions: []AskQuestion{{
		Header:   "Auth method",
		Question: "Which auth should the endpoint use?",
		Options: []AskOption{
			{Label: "Session cookie"},
			{Label: "Bearer token"},
		},
	}}}
}

func TestNormalizeAskRequestFillsIDsAndValidates(t *testing.T) {
	t.Run("assigns positional IDs when omitted", func(t *testing.T) {
		request := AskRequest{Questions: []AskQuestion{
			{Question: "first?", Options: []AskOption{{Label: "a"}, {Label: "b"}}},
			{Question: "second?", Options: []AskOption{{Label: "a"}, {Label: "b"}}},
		}}
		got, err := NormalizeAskRequest(request)
		if err != nil {
			t.Fatalf("NormalizeAskRequest: %v", err)
		}
		if got.Questions[0].ID != "question1" || got.Questions[1].ID != "question2" {
			t.Fatalf("IDs = %q, %q", got.Questions[0].ID, got.Questions[1].ID)
		}
	})

	t.Run("de-duplicates colliding IDs so answers cannot overwrite each other", func(t *testing.T) {
		request := AskRequest{Questions: []AskQuestion{
			{ID: "same", Question: "first?", Options: []AskOption{{Label: "a"}, {Label: "b"}}},
			{ID: "same", Question: "second?", Options: []AskOption{{Label: "a"}, {Label: "b"}}},
		}}
		got, err := NormalizeAskRequest(request)
		if err != nil {
			t.Fatalf("NormalizeAskRequest: %v", err)
		}
		if got.Questions[0].ID == got.Questions[1].ID {
			t.Fatalf("both questions kept ID %q", got.Questions[0].ID)
		}
	})

	t.Run("drops blank options", func(t *testing.T) {
		request := AskRequest{Questions: []AskQuestion{{
			Question: "pick?",
			Options:  []AskOption{{Label: "a"}, {Label: "   "}, {Label: "b"}},
		}}}
		got, err := NormalizeAskRequest(request)
		if err != nil {
			t.Fatalf("NormalizeAskRequest: %v", err)
		}
		if len(got.Questions[0].Options) != 2 {
			t.Fatalf("options = %#v", got.Questions[0].Options)
		}
	})

	t.Run("rejects unusable requests", func(t *testing.T) {
		tests := []struct {
			name    string
			request AskRequest
		}{
			{name: "no questions", request: AskRequest{}},
			{
				name:    "blank question text",
				request: AskRequest{Questions: []AskQuestion{{Options: []AskOption{{Label: "a"}, {Label: "b"}}}}},
			},
			{
				// One option is not a question, it is an announcement.
				name:    "fewer than two options",
				request: AskRequest{Questions: []AskQuestion{{Question: "pick?", Options: []AskOption{{Label: "only"}}}}},
			},
			{
				name: "too many questions",
				request: AskRequest{Questions: []AskQuestion{
					{Question: "1?", Options: []AskOption{{Label: "a"}, {Label: "b"}}},
					{Question: "2?", Options: []AskOption{{Label: "a"}, {Label: "b"}}},
					{Question: "3?", Options: []AskOption{{Label: "a"}, {Label: "b"}}},
					{Question: "4?", Options: []AskOption{{Label: "a"}, {Label: "b"}}},
					{Question: "5?", Options: []AskOption{{Label: "a"}, {Label: "b"}}},
				}},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if _, err := NormalizeAskRequest(tt.request); err == nil {
					t.Fatal("expected an error")
				}
			})
		}
	})
}

func TestAskPromptsWithoutACallbackReportsUnavailable(t *testing.T) {
	var prompts askPrompts
	_, err := prompts.ask(context.Background(), validAskRequest())
	if !errors.Is(err, ErrAskUnavailable) {
		t.Fatalf("err = %v, want ErrAskUnavailable so the tool reports instead of hanging", err)
	}
}

func TestAskPromptsNormalizesBeforeCallingTheHost(t *testing.T) {
	var prompts askPrompts
	var got AskRequest
	prompts.set(func(_ context.Context, request AskRequest) (AskResult, error) {
		got = request
		return AskResult{Answers: map[string]string{"question1": "Bearer token"}}, nil
	})

	result, err := prompts.ask(context.Background(), validAskRequest())
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if got.Questions[0].ID != "question1" {
		t.Fatalf("host should receive a normalized request, got ID %q", got.Questions[0].ID)
	}
	if result.Answers["question1"] != "Bearer token" {
		t.Fatalf("answers = %#v", result.Answers)
	}
}

func TestAskPromptsRejectsBadRequestsBeforePromptingTheUser(t *testing.T) {
	var prompts askPrompts
	called := false
	prompts.set(func(context.Context, AskRequest) (AskResult, error) {
		called = true
		return AskResult{}, nil
	})

	if _, err := prompts.ask(context.Background(), AskRequest{}); err == nil {
		t.Fatal("expected a validation error")
	}
	if called {
		t.Fatal("the user should not be interrupted by a malformed request")
	}
}

func TestAskToolIsRegisteredOnTheSession(t *testing.T) {
	session := newTestSession(t, newTestModel())
	defer session.Close("test")

	found := false
	for _, tool := range session.agent.GetState().Tools {
		if tool.Name() == "ask_user_question" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("ask_user_question should be available to the model")
	}
}

func TestSessionAskUserReachesTheRegisteredCallback(t *testing.T) {
	session := newTestSession(t, newTestModel())
	defer session.Close("test")

	session.SetAskCallback(func(_ context.Context, request AskRequest) (AskResult, error) {
		return AskResult{Answers: map[string]string{request.Questions[0].ID: "Bearer token"}}, nil
	})

	result, err := session.AskUser(context.Background(), validAskRequest())
	if err != nil {
		t.Fatalf("AskUser: %v", err)
	}
	if result.Answers["question1"] != "Bearer token" {
		t.Fatalf("answers = %#v", result.Answers)
	}
}
