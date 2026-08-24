package contracts

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const OrderEventsTopic = "order.events.v1"

const (
	EventOrderCreated              = "OrderCreated"
	EventInventoryReserved         = "InventoryReserved"
	EventInventoryRejected         = "InventoryRejected"
	EventPaymentCompleted          = "PaymentCompleted"
	EventPaymentFailed             = "PaymentFailed"
	EventInventoryReleaseRequested = "InventoryReleaseRequested"
	EventInventoryReleased         = "InventoryReleased"
	EventOrderConfirmed            = "OrderConfirmed"
	EventOrderRejected             = "OrderRejected"
	EventOrderCancelled            = "OrderCancelled"
	EventInventoryConsumed         = "InventoryConsumed"
)

// Envelope เป็น boundary เดียวของ event เพื่อให้ทุก consumer ตรวจ identity/correlation
// ก่อนทำ side effect และรองรับการ retry แบบ at-least-once ได้อย่างปลอดภัย
type Envelope struct {
	SchemaVersion int             `json:"schema_version"`
	EventID       string          `json:"event_id"`
	EventType     string          `json:"event_type"`
	CorrelationID string          `json:"correlation_id"`
	CausationID   string          `json:"causation_id,omitempty"`
	OccurredAt    time.Time       `json:"occurred_at"`
	CustomerID    string          `json:"customer_id"`
	OrderID       string          `json:"order_id"`
	Payload       json.RawMessage `json:"payload"`
}

func (e Envelope) Validate() error {
	if e.SchemaVersion < 1 || strings.TrimSpace(e.EventID) == "" || strings.TrimSpace(e.EventType) == "" || strings.TrimSpace(e.CorrelationID) == "" || strings.TrimSpace(e.OrderID) == "" || e.OccurredAt.IsZero() || len(e.Payload) == 0 {
		return errors.New("invalid event envelope")
	}
	return nil
}
