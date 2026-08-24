package contracts

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEnvelopeValidate(t *testing.T) {
	e := Envelope{SchemaVersion: 1, EventID: "evt-1", EventType: "OrderCreated", CorrelationID: "ord-1", OrderID: "ord-1", OccurredAt: time.Now().UTC(), Payload: json.RawMessage(`{}`)}
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
	e.EventID = ""
	if err := e.Validate(); err == nil {
		t.Fatal("expected missing identity to fail")
	}
}
