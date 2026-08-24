package main

import (
	"context"
	notificationv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/notification/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"testing"
)

func TestNotificationOwnershipAndUnread(t *testing.T) {
	s := &notificationServer{rows: map[string]notificationRow{"n1": {ID: "n1", UserID: "u1", OrderID: "o1"}, "n2": {ID: "n2", UserID: "u2", OrderID: "o2"}}, seen: map[string]bool{}}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-user-id", "u1"))
	r, e := s.ListNotifications(ctx, &notificationv1.ListNotificationsRequest{})
	if e != nil || r.GetTotal() != 1 {
		t.Fatalf("list=%v err=%v", r, e)
	}
	_, e = s.MarkAsRead(ctx, &notificationv1.MarkAsReadRequest{Id: "n2"})
	if status.Code(e) != codes.PermissionDenied {
		t.Fatalf("code=%v", status.Code(e))
	}
	n, _ := s.UnreadCount(ctx, &notificationv1.UnreadCountRequest{})
	if n.GetCount() != 1 {
		t.Fatalf("unread=%d", n.GetCount())
	}
	_, _ = s.MarkAsRead(ctx, &notificationv1.MarkAsReadRequest{Id: "n1"})
	n, _ = s.UnreadCount(ctx, &notificationv1.UnreadCountRequest{})
	if n.GetCount() != 0 {
		t.Fatalf("after read=%d", n.GetCount())
	}
}
