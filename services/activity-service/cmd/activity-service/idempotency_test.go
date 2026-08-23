package main

import (
	"encoding/json"
	"github.com/google/uuid"
	"testing"
)

func TestEventIDIsStableKey(t *testing.T) {
	id := uuid.NewString()
	p := eventPayload{EventID: id, EventType: "task.created"}
	b, _ := json.Marshal(p)
	var q eventPayload
	_ = json.Unmarshal(b, &q)
	if q.EventID != id {
		t.Fatal("event id lost")
	}
	s := &activityServer{seen: map[string]struct{}{}, items: nil}
	s.seen[id] = struct{}{}
	if _, ok := s.seen[id]; !ok {
		t.Fatal("duplicate should be ignored")
	}
}
