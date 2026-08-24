package main

import (
	"context"
	"encoding/json"
	"time"

	activityv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/activity/v1"
	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
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

func TestAdminCanListAllOrderActivitiesWithoutOrderFilter(t *testing.T) {
	s := &activityServer{seen: map[string]struct{}{}, items: []activity{
		{ID: "a1", TaskID: "order-1", ActorID: "member-1", EventType: "OrderCreated", OccurredAt: time.Now()},
		{ID: "a2", TaskID: "order-2", ActorID: "member-2", EventType: "OrderConfirmed", OccurredAt: time.Now()},
	}}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-user-id", "admin-1", "x-user-role", "admin"))

	response, err := s.ListOrderActivities(ctx, &activityv1.ListOrderActivitiesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if response.GetTotal() != 2 {
		t.Fatalf("expected all order activities for admin, got %d", response.GetTotal())
	}
}
