package stream

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestPushAndEvents(t *testing.T) {
	s := New[string, int]()
	go func() {
		s.Push("a")
		s.Push("b")
		s.Resolve(42, nil)
	}()

	var got []string
	for e := range s.Events() {
		got = append(got, e)
		if len(got) == 2 {
			s.Close()
		}
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("events = %#v, want [a b]", got)
	}
}

func TestResolveReturnsResult(t *testing.T) {
	s := New[string, int]()
	s.Resolve(42, nil)
	res, err := s.Result()
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	if res != 42 {
		t.Errorf("Result() = %d, want 42", res)
	}
}

func TestResolveOnlyAppliesFirstCall(t *testing.T) {
	s := New[string, int]()
	s.Resolve(1, nil)
	s.Resolve(2, nil) // should be ignored
	res, err := s.Result()
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	if res != 1 {
		t.Errorf("Result() = %d, want 1 (first Resolve wins)", res)
	}
}

func TestResolveWithError(t *testing.T) {
	s := New[string, int]()
	wantErr := errors.New("boom")
	s.Resolve(0, wantErr)
	_, err := s.Result()
	if !errors.Is(err, wantErr) {
		t.Errorf("Result() err = %v, want %v", err, wantErr)
	}
}

func TestCloseWithoutResolveReturnsDefaultError(t *testing.T) {
	s := New[string, int]()
	s.Close()
	res, err := s.Result()
	if err == nil {
		t.Fatal("expected an error when Close is called without Resolve")
	}
	if res != 0 {
		t.Errorf("Result() = %d, want the zero value", res)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	s := New[string, int]()
	s.Close()
	s.Close() // must not panic (double close of channels)
}

func TestPushAfterCloseDoesNotBlockOrPanic(t *testing.T) {
	s := New[string, int]()
	s.Close()
	done := make(chan struct{})
	go func() {
		s.Push("ignored")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Push after Close should not block")
	}
}

func TestConcurrentPushAndClose(t *testing.T) {
	s := New[int, struct{}]()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.Push(i)
		}(i)
	}
	go func() {
		for range s.Events() {
			// drain
		}
	}()
	wg.Wait()
	s.Close()
}

// Regression test for a real panic: Push raced Close via a bare select on
// ch/done, but Go's select picks randomly among ready cases, and a send on
// an already-closed ch is "ready" too — it panics the instant it's chosen.
// Pushers must never observe ch in a closed state while still attempting to
// send on it. Run with -race and a high iteration count to catch the window.
func TestConcurrentPushDuringCloseNeverPanics(t *testing.T) {
	for iter := 0; iter < 200; iter++ {
		s := New[int, struct{}]()
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				s.Push(i)
			}(i)
		}
		go func() {
			for range s.Events() {
				// drain
			}
		}()
		s.Close()
		wg.Wait()
	}
}
