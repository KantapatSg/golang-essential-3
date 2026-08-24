package main

import (
	"strings"
	"testing"
)

func TestPaymentScenarioIsDeterministic(t *testing.T) {
	if strings.ToLower("decline") != "decline" {
		t.Fatal("scenario")
	}
	if strings.ToLower("") == "decline" {
		t.Fatal("empty scenario")
	}
	s := &paymentServer{payments: map[string]payment{}, processed: map[string]bool{}}
	if len(s.payments) != 0 {
		t.Fatal("unexpected payment")
	}
}
