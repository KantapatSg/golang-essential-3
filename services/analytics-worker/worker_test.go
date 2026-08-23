package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
func TestValidateAcceptsLegacyGoEnvelope(t *testing.T) {
	w := &worker{}
	raw := `{"EventID":"e2","EventType":"task.updated","Task":{"ID":"t2","OwnerID":"u2","Status":"done"},"OccurredAt":"2026-01-01T00:00:00Z"}`
	e, err := w.validate([]byte(raw))
	if err != nil || e.EventID != "e2" || e.Task.OwnerID != "u2" {
		t.Fatalf("legacy payload not decoded: %#v %v", e, err)
	}
}
func TestInsertBatchAndFailure(t *testing.T) {
	seen := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen = string(body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	w := &worker{endpoint: server.URL, http: server.Client()}
	e, err := w.validate([]byte(`{"event_id":"e3","event_type":"task.created","task":{"id":"t3","owner_id":"u3"},"occurred_at":"2026-01-01T00:00:00Z"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = w.insert(context.Background(), []taskEvent{e, e}); err != nil {
		t.Fatal(err)
	}
	if w.processed.Load() != 2 || strings.Count(seen, `"actor_id"`) != 2 {
		t.Fatalf("batch insert not recorded: processed=%d body=%s", w.processed.Load(), seen)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", http.StatusServiceUnavailable) }))
	defer bad.Close()
	w.endpoint = bad.URL
	if err = w.retryInsert(context.Background(), []taskEvent{e}); err == nil || w.retries.Load() != 3 {
		t.Fatalf("expected bounded retry, err=%v retries=%d", err, w.retries.Load())
	}
}
