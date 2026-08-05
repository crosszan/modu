package coding_agent

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/openmodu/modu/pkg/coding_agent/tools/ask"
)

// MaxAskQuestions bounds how many questions one ask may carry. A card the
// user has to page through defeats the point of asking; more than a handful
// means the model should be making some of those calls itself.
const MaxAskQuestions = 4

// ErrAskUnavailable is returned when nothing can prompt the user — headless
// runs (print/rpc/acp), cron ticks, or any host that never registered an ask
// callback. Callers must surface it rather than block, so the agent keeps
// making progress instead of hanging on an answer that will never arrive.
var ErrAskUnavailable = errors.New("no host is available to ask the user")

// AskOption is one selectable answer.
type AskOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// AskQuestion is a single question with its selectable answers.
type AskQuestion struct {
	// ID keys this question's entry in AskResult.Answers. Callers that omit
	// it get a positional fallback ("question1", ...).
	ID string `json:"id,omitempty"`
	// Header is a short label (a few words) identifying what is being
	// decided, shown as the card's title.
	Header   string      `json:"header,omitempty"`
	Question string      `json:"question"`
	Options  []AskOption `json:"options"`
}

// AskRequest is one round of questions for the user.
type AskRequest struct {
	Questions []AskQuestion `json:"questions"`
}

// AskResult carries the user's answers, keyed by AskQuestion.ID.
type AskResult struct {
	Answers map[string]string `json:"answers"`
	// Cancelled reports that the user dismissed the prompt instead of
	// answering. It is deliberately distinct from an empty answer: the model
	// must be able to tell "declined to answer" from "picked something
	// blank", and must not read a dismissal as consent to the first option.
	Cancelled bool `json:"cancelled"`
}

// askPrompts holds the host's ask callback, mirroring extensionPrompts: the
// host wires a callback in, tools ask through it, and a missing callback is
// an explicit error rather than a silent default.
type askPrompts struct {
	mu sync.RWMutex
	cb func(context.Context, AskRequest) (AskResult, error)
}

func (a *askPrompts) set(fn func(context.Context, AskRequest) (AskResult, error)) {
	a.mu.Lock()
	a.cb = fn
	a.mu.Unlock()
}

func (a *askPrompts) ask(ctx context.Context, request AskRequest) (AskResult, error) {
	a.mu.RLock()
	cb := a.cb
	a.mu.RUnlock()
	if cb == nil {
		return AskResult{}, ErrAskUnavailable
	}
	normalized, err := NormalizeAskRequest(request)
	if err != nil {
		return AskResult{}, err
	}
	return cb(ctx, normalized)
}

// NormalizeAskRequest validates a request and fills in the defaults the rest
// of the pipeline relies on: every question gets a non-empty ID (so answers
// can be keyed) and options are trimmed of blanks.
func NormalizeAskRequest(request AskRequest) (AskRequest, error) {
	if len(request.Questions) == 0 {
		return AskRequest{}, errors.New("at least one question is required")
	}
	if len(request.Questions) > MaxAskQuestions {
		return AskRequest{}, errors.New("too many questions")
	}
	out := AskRequest{Questions: make([]AskQuestion, 0, len(request.Questions))}
	seen := make(map[string]bool, len(request.Questions))
	for i, question := range request.Questions {
		question.ID = strings.TrimSpace(question.ID)
		question.Header = strings.TrimSpace(question.Header)
		question.Question = strings.TrimSpace(question.Question)
		if question.Question == "" {
			return AskRequest{}, errors.New("every question needs question text")
		}
		if question.ID == "" || seen[question.ID] {
			question.ID = "question" + strconv.Itoa(i+1)
		}
		seen[question.ID] = true

		options := make([]AskOption, 0, len(question.Options))
		for _, option := range question.Options {
			option.Label = strings.TrimSpace(option.Label)
			option.Description = strings.TrimSpace(option.Description)
			if option.Label == "" {
				continue
			}
			options = append(options, option)
		}
		if len(options) < 2 {
			return AskRequest{}, errors.New("every question needs at least two options")
		}
		question.Options = options
		out.Questions = append(out.Questions, question)
	}
	return out, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// --- engine delegates (preserve the public API surface) ---

// SetAskCallback wires the host prompt used by the ask_user_question tool.
// Without one the tool reports ErrAskUnavailable instead of blocking.
func (s *engine) SetAskCallback(fn func(context.Context, AskRequest) (AskResult, error)) {
	s.askPrompts.set(fn)
}

// AskUser puts a round of questions to the user and blocks until they
// answer, the host declines, or ctx is cancelled.
func (s *engine) AskUser(ctx context.Context, request AskRequest) (AskResult, error) {
	return s.askPrompts.ask(ctx, request)
}

// replaceAskTool installs the ask_user_question tool. The tool is always
// registered: whether a user can actually be reached depends on the host at
// call time (a TUI can, a cron tick cannot), and askPrompts reports that as
// an error the model can act on.
func (s *engine) replaceAskTool() {
	tool := ask.NewTool(askAdapter{engine: s})
	s.activeTools = replaceTool(s.activeTools, tool)
	s.agent.SetTools(replaceTool(s.agent.GetState().Tools, tool))
}

// askAdapter converts between the session's Ask types and the tool package's
// copies. The tool package cannot import this one (this package imports
// tools, so the reverse would be an import cycle), so the two type sets are
// declared separately and translated here.
type askAdapter struct{ engine *engine }

func (a askAdapter) AskUser(ctx context.Context, request ask.Request) (ask.Result, error) {
	questions := make([]AskQuestion, 0, len(request.Questions))
	for _, question := range request.Questions {
		options := make([]AskOption, 0, len(question.Options))
		for _, option := range question.Options {
			options = append(options, AskOption{Label: option.Label, Description: option.Description})
		}
		questions = append(questions, AskQuestion{
			ID:       question.ID,
			Header:   question.Header,
			Question: question.Question,
			Options:  options,
		})
	}
	result, err := a.engine.AskUser(ctx, AskRequest{Questions: questions})
	if err != nil {
		return ask.Result{}, err
	}
	return ask.Result{Answers: result.Answers, Cancelled: result.Cancelled}, nil
}
