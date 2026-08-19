package tui

import (
	"context"
	"sync"
	"testing"
	"time"

	modutui "github.com/openmodu/modu/pkg/modu-tui"
	"github.com/openmodu/modu/pkg/types"
)

type runtimeSessionStub struct {
	mu sync.Mutex

	promptStarted   chan struct{}
	promptRelease   chan struct{}
	promptOnce      sync.Once
	prompts         []string
	queued          bool
	continueCalls   int
	followUps       []string
	followUpImages  [][]types.ImageContent
	steers          []string
	abortCalls      int
	abortBashCalls  int
	promptCancelled bool
}

func newRuntimeSessionStub() *runtimeSessionStub {
	return &runtimeSessionStub{
		promptStarted: make(chan struct{}),
		promptRelease: make(chan struct{}),
	}
}

func (s *runtimeSessionStub) PromptWithImages(ctx context.Context, text string, _ []types.ImageContent) error {
	s.mu.Lock()
	s.prompts = append(s.prompts, text)
	promptNumber := len(s.prompts)
	s.mu.Unlock()
	s.promptOnce.Do(func() { close(s.promptStarted) })
	if promptNumber > 1 {
		return nil
	}
	select {
	case <-s.promptRelease:
		return nil
	case <-ctx.Done():
		s.mu.Lock()
		s.promptCancelled = true
		s.mu.Unlock()
		return ctx.Err()
	}
}

func (s *runtimeSessionStub) Continue(context.Context) error {
	s.mu.Lock()
	s.continueCalls++
	s.queued = false
	s.mu.Unlock()
	return nil
}

func (s *runtimeSessionStub) FollowUpWithImages(text string, images []types.ImageContent) error {
	s.mu.Lock()
	s.followUps = append(s.followUps, text)
	s.followUpImages = append(s.followUpImages, append([]types.ImageContent(nil), images...))
	s.queued = true
	s.mu.Unlock()
	return nil
}

func (s *runtimeSessionStub) SteerWithImages(text string, _ []types.ImageContent) error {
	s.mu.Lock()
	s.steers = append(s.steers, text)
	s.queued = true
	s.mu.Unlock()
	return nil
}

func (s *runtimeSessionStub) HasQueuedMessages() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queued
}

func (s *runtimeSessionStub) Abort() {
	s.mu.Lock()
	s.abortCalls++
	s.mu.Unlock()
}

func (s *runtimeSessionStub) AbortBash() {
	s.mu.Lock()
	s.abortBashCalls++
	s.mu.Unlock()
}

func TestRuntimeFollowUpStartsDistinctTurnAfterActivePrompt(t *testing.T) {
	session := newRuntimeSessionStub()
	var messagesMu sync.Mutex
	var messages []any
	runtime, err := NewRuntime(RuntimeOptions{
		Context: context.Background(),
		Session: session,
		Client: modutui.NewClient(func(message any) {
			messagesMu.Lock()
			messages = append(messages, message)
			messagesMu.Unlock()
		}),
		TerminalStatusTTL: time.Second,
		FormatDuration: func(time.Duration) string {
			return "1s"
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	runtime.RunPrompt("first", nil)
	waitRuntimeSignal(t, session.promptStarted)
	images := []types.ImageContent{{Type: "image", Data: "before", MimeType: "image/png"}}
	runtime.QueueFollowUp("second", images, true)
	images[0].Data = "after"
	waitRuntimeCondition(t, func() bool {
		return containsRuntimeStatusSnapshot(&messagesMu, &messages, modutui.StatusQueued)
	})
	session.mu.Lock()
	if len(session.followUps) != 0 || session.continueCalls != 0 {
		t.Fatalf("follow-up entered active agent loop before it completed: followUps=%#v continueCalls=%d", session.followUps, session.continueCalls)
	}
	session.mu.Unlock()
	close(session.promptRelease)
	waitRuntimeCondition(t, func() bool {
		session.mu.Lock()
		defer session.mu.Unlock()
		return session.continueCalls == 1 && !runtime.IsForegroundRunActive()
	})

	session.mu.Lock()
	if len(session.followUps) != 1 || session.followUps[0] != "second" {
		t.Fatalf("followUps = %#v", session.followUps)
	}
	if len(session.followUpImages) != 1 || len(session.followUpImages[0]) != 1 || session.followUpImages[0][0].Data != "before" {
		t.Fatalf("followUpImages = %#v", session.followUpImages)
	}
	if session.continueCalls != 1 {
		t.Fatalf("follow-up did not start its distinct continuation: %d", session.continueCalls)
	}
	if len(session.prompts) != 1 || session.prompts[0] != "first" {
		t.Fatalf("prompts = %#v", session.prompts)
	}
	session.mu.Unlock()
	messagesMu.Lock()
	defer messagesMu.Unlock()
	if !containsRuntimeStatus(messages, modutui.StatusQueued) || !containsRuntimeStatus(messages, "✓ Completed 1s") {
		t.Fatalf("messages = %#v", messages)
	}
}

func containsRuntimeStatusSnapshot(mu *sync.Mutex, messages *[]any, want string) bool {
	mu.Lock()
	defer mu.Unlock()
	return containsRuntimeStatus(*messages, want)
}

func TestRuntimeSteerDoesNotAbortTheRunningTurn(t *testing.T) {
	// Steering must not throw away in-flight work. The agent loop already
	// collects steering after each tool batch and skips the calls queued
	// behind it (pkg/agent/tools.go), so the message joins the running turn
	// at its next boundary. Cancelling here would kill the in-flight tool
	// and LLM request and bypass that mechanism entirely.
	session := newRuntimeSessionStub()
	runtime, err := NewRuntime(RuntimeOptions{
		Context: context.Background(),
		Session: session,
		Client:  modutui.NewClient(func(any) {}),
	})
	if err != nil {
		t.Fatal(err)
	}

	runtime.RunPrompt("first", nil)
	waitRuntimeSignal(t, session.promptStarted)
	runtime.QueueSteer("change direction", nil, true)
	waitRuntimeCondition(t, func() bool {
		session.mu.Lock()
		defer session.mu.Unlock()
		return len(session.steers) == 1
	})

	session.mu.Lock()
	defer session.mu.Unlock()
	if session.steers[0] != "change direction" {
		t.Fatalf("steers = %#v", session.steers)
	}
	if session.abortCalls != 0 || session.abortBashCalls != 0 {
		t.Fatalf("steering must not abort: abort = %d, bash = %d", session.abortCalls, session.abortBashCalls)
	}
	if session.promptCancelled {
		t.Fatal("steering must not cancel the running turn's context")
	}
}

func TestRuntimeKeepsQueuedInputInSubmissionOrder(t *testing.T) {
	session := newRuntimeSessionStub()
	runtime, err := NewRuntime(RuntimeOptions{
		Context: context.Background(),
		Session: session,
		Client:  modutui.NewClient(func(any) {}),
	})
	if err != nil {
		t.Fatal(err)
	}

	runtime.RunPrompt("first", nil)
	waitRuntimeSignal(t, session.promptStarted)
	runtime.QueueFollowUp("follow-up one", nil, true)
	runtime.QueueFollowUp("follow-up two", nil, true)
	runtime.QueueSteer("steer one", nil, true)
	runtime.QueueSteer("steer two", nil, true)

	session.mu.Lock()
	if len(session.followUps) != 0 {
		t.Fatalf("follow-ups entered the active turn early: %#v", session.followUps)
	}
	if got := append([]string(nil), session.steers...); len(got) != 2 || got[0] != "steer one" || got[1] != "steer two" {
		t.Fatalf("steers = %#v", got)
	}
	// Simulate the active agent loop consuming both steers at its next tool
	// boundary before the original prompt returns.
	session.queued = false
	session.mu.Unlock()

	close(session.promptRelease)
	waitRuntimeCondition(t, func() bool {
		session.mu.Lock()
		defer session.mu.Unlock()
		return session.continueCalls == 2 && !runtime.IsForegroundRunActive()
	})

	session.mu.Lock()
	defer session.mu.Unlock()
	if len(session.followUps) != 2 || session.followUps[0] != "follow-up one" || session.followUps[1] != "follow-up two" {
		t.Fatalf("followUps = %#v", session.followUps)
	}
}

func TestRuntimeInterruptCancelsWithoutContinuation(t *testing.T) {
	session := newRuntimeSessionStub()
	statuses := make(chan string, 8)
	runtime, err := NewRuntime(RuntimeOptions{
		Context: context.Background(),
		Session: session,
		Client: modutui.NewClient(func(message any) {
			if status, ok := runtimeStatusUpdate(message); ok {
				statuses <- status.Status
			}
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	runtime.RunPrompt("first", nil)
	waitRuntimeSignal(t, session.promptStarted)
	runtime.Interrupt()
	waitRuntimeCondition(t, func() bool {
		session.mu.Lock()
		defer session.mu.Unlock()
		return session.abortCalls == 1 && !runtime.IsForegroundRunActive()
	})

	session.mu.Lock()
	defer session.mu.Unlock()
	if session.continueCalls != 0 {
		t.Fatalf("continueCalls = %d", session.continueCalls)
	}
	if session.abortBashCalls != 1 {
		t.Fatalf("abortBashCalls = %d", session.abortBashCalls)
	}
}

func TestRuntimeRejectsRequiredQueueWhenIdle(t *testing.T) {
	session := newRuntimeSessionStub()
	statuses := make(chan string, 2)
	runtime, err := NewRuntime(RuntimeOptions{
		Session: session,
		Client: modutui.NewClient(func(message any) {
			if status, ok := runtimeStatusUpdate(message); ok {
				statuses <- status.Status
			}
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	runtime.QueueFollowUp("later", nil, true)
	select {
	case status := <-statuses:
		if status != "no active task to followup" {
			t.Fatalf("status = %q", status)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for idle status")
	}
}

func TestRuntimeRejectsRequiredSteerWhenIdle(t *testing.T) {
	session := newRuntimeSessionStub()
	statuses := make(chan string, 2)
	runtime, err := NewRuntime(RuntimeOptions{
		Session: session,
		Client: modutui.NewClient(func(message any) {
			if status, ok := runtimeStatusUpdate(message); ok {
				statuses <- status.Status
			}
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	runtime.QueueSteer("late", nil, true)
	select {
	case status := <-statuses:
		if status != "no active task to steer" {
			t.Fatalf("status = %q", status)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for idle steer status")
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	if len(session.steers) != 0 || len(session.prompts) != 0 {
		t.Fatalf("idle required steer mutated session: steers=%#v prompts=%#v", session.steers, session.prompts)
	}
}

func TestRuntimeRunPanicClearsPromptState(t *testing.T) {
	session := newRuntimeSessionStub()
	runtime, err := NewRuntime(RuntimeOptions{
		Context:           context.Background(),
		Session:           session,
		Client:            modutui.NewClient(func(any) {}),
		TerminalStatusTTL: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	runtime.Run(func(context.Context) error {
		panic("boom")
	})

	waitRuntimeCondition(t, func() bool {
		return !runtime.IsForegroundRunActive()
	})
	if runtime.IsPromptActive() {
		t.Fatal("IsPromptActive stuck true after panicking run")
	}
}

func TestRuntimeRunWithCompletionReleasesPromptBeforeCallback(t *testing.T) {
	session := newRuntimeSessionStub()
	runtime, err := NewRuntime(RuntimeOptions{
		Context: context.Background(),
		Session: session,
		Client:  modutui.NewClient(func(any) {}),
	})
	if err != nil {
		t.Fatal(err)
	}

	type callbackState struct {
		promptActive     bool
		foregroundActive bool
	}
	completed := make(chan callbackState, 1)
	runtime.RunWithCompletion(func(context.Context) error {
		return nil
	}, func(error) {
		completed <- callbackState{
			promptActive:     runtime.IsPromptActive(),
			foregroundActive: runtime.IsForegroundRunActive(),
		}
	})

	select {
	case state := <-completed:
		if state.promptActive {
			t.Fatal("prompt was still active when completion callback ran")
		}
		if !state.foregroundActive {
			t.Fatal("foreground run ended before completion callback returned")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for completion callback")
	}
	waitRuntimeCondition(t, func() bool {
		return !runtime.IsForegroundRunActive()
	})
}

func TestRuntimeRunOnceWithCompletionDoesNotConsumeMainQueue(t *testing.T) {
	session := newRuntimeSessionStub()
	session.queued = true
	runtime, err := NewRuntime(RuntimeOptions{
		Context: context.Background(),
		Session: session,
		Client:  modutui.NewClient(func(any) {}),
	})
	if err != nil {
		t.Fatal(err)
	}

	completed := make(chan struct{}, 1)
	runtime.RunOnceWithCompletion(func(context.Context) error {
		return nil
	}, func(error) {
		completed <- struct{}{}
	})
	waitRuntimeSignal(t, completed)

	session.mu.Lock()
	defer session.mu.Unlock()
	if session.continueCalls != 0 || !session.queued {
		t.Fatalf("isolated run consumed main queue: continueCalls=%d queued=%v", session.continueCalls, session.queued)
	}
}

func waitRuntimeSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runtime signal")
	}
}

func waitRuntimeCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for runtime condition")
}

func containsRuntimeStatus(messages []any, want string) bool {
	for _, message := range messages {
		if status, ok := runtimeStatusUpdate(message); ok && status.Status == want {
			return true
		}
	}
	return false
}

func runtimeStatusUpdate(message any) (modutui.SetStatusUpdate, bool) {
	envelope, ok := message.(modutui.UpdateMsg)
	if !ok {
		return modutui.SetStatusUpdate{}, false
	}
	status, ok := envelope.Update.(modutui.SetStatusUpdate)
	return status, ok
}
