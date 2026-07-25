package bgtask

import "testing"

func TestSteerDeliversOnlyToLiveTasks(t *testing.T) {
	m := New()
	id := m.Create("subagent", "explorer: map auth")

	if m.Steer(id, "also check the refresh path") {
		t.Fatal("steer succeeded before a delivery function was registered")
	}

	var got []string
	m.RegisterSteer(id, func(text string) { got = append(got, text) })
	if !m.Steer(id, "also check the refresh path") {
		t.Fatal("steer failed for a running task with a registered delivery function")
	}
	if len(got) != 1 || got[0] != "also check the refresh path" {
		t.Fatalf("delivered = %v, want the message once", got)
	}

	if m.Steer("task-does-not-exist", "hi") {
		t.Fatal("steer succeeded for an unknown task")
	}

	// A finished task must not accept messages even while its delivery
	// function is still registered — the child's loop has already exited.
	m.Complete(id, "done")
	if m.Steer(id, "too late") {
		t.Fatal("steer succeeded for a completed task")
	}
	if len(got) != 1 {
		t.Fatalf("delivered %d messages, want 1", len(got))
	}
}

func TestUnregisterCancelDropsSteer(t *testing.T) {
	m := New()
	id := m.Create("subagent", "explorer: map auth")
	m.RegisterSteer(id, func(string) {})
	m.UnregisterCancel(id)
	if m.Steer(id, "hello") {
		t.Fatal("steer succeeded after the run unregistered itself")
	}
}
