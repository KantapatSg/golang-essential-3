package main

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
)

func TestUnavailableMapsTo503(t *testing.T) {
	if got := statusToHTTP(status.Error(codes.Unavailable, "down")); got != 503 {
		t.Fatalf("got %d", got)
	}
}
func statusToHTTP(err error) int { return map[codes.Code]int{codes.Unavailable: 503}[status.Code(err)] }
