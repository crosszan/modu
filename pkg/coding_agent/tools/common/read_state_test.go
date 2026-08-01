package common

import (
	"sync"
	"testing"
)

func TestFileReadStateRecordAndGet(t *testing.T) {
	s := NewFileReadState()
	s.Record("/a.txt", "content", 123, false)

	got, ok := s.Get("/a.txt")
	if !ok {
		t.Fatal("expected a record for /a.txt")
	}
	if got.Content != "content" || got.ModTimeNanos != 123 || got.Partial {
		t.Errorf("Get(/a.txt) = %+v, want Content=content ModTimeNanos=123 Partial=false", got)
	}

	if _, ok := s.Get("/missing.txt"); ok {
		t.Error("expected no record for an unrecorded path")
	}
}

func TestFileReadStateRecordOverwritesPriorEntry(t *testing.T) {
	s := NewFileReadState()
	s.Record("/a.txt", "v1", 1, false)
	s.Record("/a.txt", "v2", 2, true)

	got, ok := s.Get("/a.txt")
	if !ok {
		t.Fatal("expected a record")
	}
	if got.Content != "v2" || got.ModTimeNanos != 2 || !got.Partial {
		t.Errorf("Get(/a.txt) = %+v, want the second Record's values", got)
	}
}

func TestFileReadStateNilReceiverIsSafe(t *testing.T) {
	var s *FileReadState
	s.Record("/a.txt", "content", 1, false) // must not panic
	if _, ok := s.Get("/a.txt"); ok {
		t.Error("a nil FileReadState should report no records")
	}
}

func TestFileReadStateZeroValueWorks(t *testing.T) {
	// FileReadState is used as an embedded/plain value in a few places, not
	// only via NewFileReadState; its files map must lazily initialize.
	var s FileReadState
	s.Record("/a.txt", "content", 1, false)
	got, ok := s.Get("/a.txt")
	if !ok || got.Content != "content" {
		t.Errorf("zero-value FileReadState.Record/Get failed: got=%+v ok=%v", got, ok)
	}
}

func TestFileReadStateConcurrentAccess(t *testing.T) {
	s := NewFileReadState()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			s.Record("/a.txt", "content", int64(i), false)
		}(i)
		go func() {
			defer wg.Done()
			s.Get("/a.txt")
		}()
	}
	wg.Wait()
}
