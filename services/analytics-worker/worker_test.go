package main

import "testing"

func TestValidateRejectsPoisonEvent(t *testing.T) {
	w := &worker{}
	if _, err := w.validate([]byte(`{"event_type":"task.created"}`)); err == nil {
		t.Fatal("expected required event fields")
	}
}
func TestValidateAcceptsSupportedEvent(t *testing.T) {
	w := &worker{}
	if _, err := w.validate([]byte(`{"event_id":"e1","event_type":"task.created","Task":{"ID":"t1","OwnerID":"u1"},"occurred_at":"2026-01-01T00:00:00Z"}`)); err != nil {
		t.Fatal(err)
	}
}
