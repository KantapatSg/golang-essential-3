package state

import "testing"

func TestLegalTransitions(t *testing.T) {
	for _, tc := range []struct{ from, event, to string }{{Pending, "InventoryReserved", StockReserved}, {Pending, "InventoryRejected", Rejected}, {StockReserved, "PaymentCompleted", Confirmed}, {StockReserved, "PaymentFailed", Cancelled}} {
		got, e := Transition(tc.from, tc.event)
		if e != nil || got != tc.to {
			t.Fatalf("%s + %s = %s/%v", tc.from, tc.event, got, e)
		}
	}
}
func TestTerminalStateDoesNotRegress(t *testing.T) {
	got, e := Transition(Confirmed, "PaymentFailed")
	if e != nil || got != Confirmed {
		t.Fatalf("got %s err %v", got, e)
	}
}
func TestPaymentBeforeReservationRejected(t *testing.T) {
	if _, e := Transition(Pending, "PaymentCompleted"); e == nil {
		t.Fatal("expected illegal transition")
	}
}
