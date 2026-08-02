package types

import (
	"errors"
	"testing"
)

func TestAgentEventStreamPushAndEmit(t *testing.T) {
	s := NewAgentEventStream()
	go func() {
		s.Push(Event{Type: EventTypeAgentStart})
		s.Emit(Event{Type: EventTypeAgentEnd})
		s.Resolve(nil, nil)
	}()

	var got []EventType
	for e := range s.Events() {
		got = append(got, e.Type)
		if len(got) == 2 {
			s.Close()
		}
	}
	if len(got) != 2 || got[0] != EventTypeAgentStart || got[1] != EventTypeAgentEnd {
		t.Errorf("events = %#v, want [agent_start agent_end]", got)
	}
}

func TestAgentEventStreamResolveAndResult(t *testing.T) {
	s := NewAgentEventStream()
	msgs := []AgentMessage{UserMessage{Role: RoleUser, Content: "hi"}}
	s.Resolve(msgs, nil)
	got, err := s.Result()
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("Result() = %#v, want the resolved messages", got)
	}
}

func TestAgentEventStreamResolveWithError(t *testing.T) {
	s := NewAgentEventStream()
	wantErr := errors.New("boom")
	s.Resolve(nil, wantErr)
	_, err := s.Result()
	if !errors.Is(err, wantErr) {
		t.Errorf("Result() err = %v, want %v", err, wantErr)
	}
}

// AgentEventStream's Push/Emit/Resolve/Close all explicitly nil-check the
// receiver (unlike EventStreamImpl), so a nil *AgentEventStream must be
// usable as a no-op sink rather than panicking — this is relied on by code
// that emits events unconditionally without always having an active stream.
func TestAgentEventStreamNilReceiverIsSafe(t *testing.T) {
	var s *AgentEventStream
	s.Push(Event{Type: EventTypeAgentStart}) // must not panic
	s.Emit(Event{Type: EventTypeAgentEnd})   // must not panic
	s.Resolve(nil, nil)                      // must not panic
	s.Close()                                // must not panic
}
