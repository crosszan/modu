package types

import (
	"errors"
	"testing"
)

func TestEventStreamImplPushAndEvents(t *testing.T) {
	s := NewEventStream()
	go func() {
		s.Push(StreamEvent{Type: EventTextDelta, Delta: "hi"})
		s.Resolve(&AssistantMessage{Role: RoleAssistant}, nil)
	}()

	var got StreamEvent
	for e := range s.Events() {
		got = e
		s.Close()
	}
	if got.Type != EventTextDelta || got.Delta != "hi" {
		t.Errorf("unexpected event: %+v", got)
	}
}

func TestEventStreamImplResolveAndResult(t *testing.T) {
	s := NewEventStream()
	msg := &AssistantMessage{Role: RoleAssistant}
	s.Resolve(msg, nil)
	got, err := s.Result()
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	if got != msg {
		t.Errorf("Result() = %v, want the resolved message", got)
	}
}

func TestEventStreamImplResolveWithError(t *testing.T) {
	s := NewEventStream()
	wantErr := errors.New("boom")
	s.Resolve(nil, wantErr)
	_, err := s.Result()
	if !errors.Is(err, wantErr) {
		t.Errorf("Result() err = %v, want %v", err, wantErr)
	}
}

func TestEventStreamImplCloseWithoutResolveReturnsDefaultError(t *testing.T) {
	s := NewEventStream()
	s.Close()
	_, err := s.Result()
	if err == nil {
		t.Fatal("expected an error when Close is called without Resolve")
	}
}
