package main

import (
	analyticsv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/analytics/v1"
	"testing"
)

func TestRangeDefaultsAndLimit(t *testing.T) {
	from, to, limit, err := rangeSQL(&analyticsv1.TimeRange{}, 0)
	if err != nil || from == "" || to == "" || limit != 100 {
		t.Fatalf("unexpected defaults: %q %q %d %v", from, to, limit, err)
	}
}
func TestRangeRejectsUnboundedLimit(t *testing.T) {
	if _, _, _, err := rangeSQL(&analyticsv1.TimeRange{}, 501); err == nil {
		t.Fatal("expected limit validation")
	}
}
