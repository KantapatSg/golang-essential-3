package main

import (
	"context"
	"encoding/json"
	taskv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/task/v1"
	"google.golang.org/grpc/metadata"
	"strings"
	"testing"
)

func TestOutboxEventUsesVersionedSnakeCaseEnvelope(t *testing.T) {
	event := outboxEvent{SchemaVersion: 1, EventID: "e1", EventType: "task.created", Task: task{ID: "t1", OwnerID: "u1", Status: "todo"}}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"schema_version":1`, `"event_id":"e1"`, `"event_type":"task.created"`, `"task":{"id":"t1"`, `"owner_id":"u1"`} {
		if !strings.Contains(string(raw), field) {
			t.Fatalf("missing %s in %s", field, raw)
		}
	}
}

func TestTaskOwnerRule(t *testing.T) {
	s := &taskServer{data: map[string]task{}, cache: map[string][]task{}, outbox: make(chan outboxEvent, 2)}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-user-id", "u1", "x-user-role", "member"))
	x, e := s.CreateTask(ctx, &taskv1.CreateTaskRequest{Title: "study"})
	if e != nil {
		t.Fatal(e)
	}
	ctx2 := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-user-id", "u2", "x-user-role", "member"))
	if _, e = s.GetTask(ctx2, &taskv1.GetTaskRequest{Id: x.Id}); e == nil {
		t.Fatal("expected ownership denial")
	}
}
