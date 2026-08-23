package main

import (
	"context"
	taskv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/task/v1"
	"google.golang.org/grpc/metadata"
	"testing"
)

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
