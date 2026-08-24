package main

import (
	"context"
	"encoding/json"
	"github.com/KantapatSg/golang-essential-3/contracts"
	inventoryv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/inventory/v1"
	orderv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/order/v1"
	"github.com/KantapatSg/golang-essential-3/services/order-service/internal/state"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"testing"
	"time"
)

type inventoryStub struct {
	inventoryv1.InventoryServiceClient
	calls int
}

func TestCanonicalTerminalEvents(t *testing.T) {
	s := &orderServer{orders: map[string]*orderRow{
		"reserved": {ID: "reserved", CustomerID: "u1", Status: state.StockReserved},
		"pending":  {ID: "pending", CustomerID: "u1", Status: state.Pending},
	}, idempotency: map[string]idem{}}
	for _, tc := range []struct {
		order, event, wantState, wantEvent string
	}{{"reserved", contracts.EventPaymentCompleted, state.Confirmed, contracts.EventOrderConfirmed}, {"pending", contracts.EventInventoryRejected, state.Rejected, contracts.EventOrderRejected}} {
		b, _ := json.Marshal(outcomePayload{OrderID: tc.order, CustomerID: "u1"})
		out, err := s.applyOutcome(context.Background(), contracts.Envelope{SchemaVersion: 1, EventID: "evt-" + tc.order, EventType: tc.event, CorrelationID: tc.order, OrderID: tc.order, CustomerID: "u1", OccurredAt: time.Now().UTC(), Payload: b})
		if err != nil || out.EventType != tc.wantEvent || s.orders[tc.order].Status != tc.wantState {
			t.Fatalf("event=%s out=%v err=%v state=%s", tc.event, out, err, s.orders[tc.order].Status)
		}
	}
}

func TestDuplicateOutcomeIsNoOp(t *testing.T) {
	s := &orderServer{orders: map[string]*orderRow{"o1": {ID: "o1", CustomerID: "u1", Status: state.StockReserved}}, idempotency: map[string]idem{}}
	b, _ := json.Marshal(outcomePayload{OrderID: "o1", CustomerID: "u1"})
	e := contracts.Envelope{SchemaVersion: 1, EventID: "duplicate", EventType: contracts.EventPaymentCompleted, CorrelationID: "o1", OrderID: "o1", CustomerID: "u1", OccurredAt: time.Now().UTC(), Payload: b}
	first, err := s.applyOutcome(context.Background(), e)
	second, err2 := s.applyOutcome(context.Background(), e)
	if err != nil || err2 != nil || first.EventType != contracts.EventOrderConfirmed || second.EventID != "" {
		t.Fatalf("first=%v second=%v err=%v/%v", first, second, err, err2)
	}
}

func (s *inventoryStub) QuoteProducts(context.Context, *inventoryv1.QuoteProductsRequest, ...grpc.CallOption) (*inventoryv1.QuoteProductsResponse, error) {
	s.calls++
	return &inventoryv1.QuoteProductsResponse{Currency: "USD", TotalMinor: 1299, Products: []*inventoryv1.Product{{Id: "p", Name: "Authoritative", UnitPriceMinor: 1299, Currency: "USD"}}}, nil
}

func TestOrderValidationRequiresIdempotency(t *testing.T) {
	s := &orderServer{orders: map[string]*orderRow{}, idempotency: map[string]idem{}}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-user-id", "u1"))
	_, e := s.CreateOrder(ctx, &orderv1.CreateOrderRequest{Items: []*orderv1.CreateOrderItem{{ProductId: "p", Quantity: 1}}})
	if status.Code(e) != codes.InvalidArgument {
		t.Fatalf("code=%v", status.Code(e))
	}
}
func TestRequestHashScenarioIsDeterministic(t *testing.T) {
	a := requestHash([]*orderv1.CreateOrderItem{{ProductId: "p", Quantity: 1}}, "success")
	b := requestHash([]*orderv1.CreateOrderItem{{ProductId: "p", Quantity: 1}}, "success")
	if a != b {
		t.Fatal("hash changed")
	}
	if normalizeScenario("decline") != "decline" {
		t.Fatal("decline not normalized")
	}
}

func TestCreateOrderIsPendingAndIdempotent(t *testing.T) {
	inv := &inventoryStub{}
	s := &orderServer{orders: map[string]*orderRow{}, idempotency: map[string]idem{}, inventory: inv}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-user-id", "u1"))
	req := &orderv1.CreateOrderRequest{Items: []*orderv1.CreateOrderItem{{ProductId: "p", Quantity: 1}}, IdempotencyKey: "k", PaymentScenario: "success"}
	a, e := s.CreateOrder(ctx, req)
	if e != nil || a.GetStatus() != "PENDING" || a.GetItems()[0].GetName() != "Authoritative" {
		t.Fatalf("order=%v err=%v", a, e)
	}
	b, e := s.CreateOrder(ctx, req)
	if e != nil || b.GetId() != a.GetId() || inv.calls != 1 {
		t.Fatalf("retry=%v err=%v calls=%d", b, e, inv.calls)
	}
}

func TestPaymentFailureTransitionsAndRequestsRelease(t *testing.T) {
	s := &orderServer{orders: map[string]*orderRow{"o1": {ID: "o1", CustomerID: "u1", Status: "STOCK_RESERVED"}}, idempotency: map[string]idem{}}
	b, _ := json.Marshal(outcomePayload{OrderID: "o1", CustomerID: "u1", Reason: "PAYMENT_DECLINED"})
	e := contracts.Envelope{SchemaVersion: 1, EventID: "evt", EventType: "PaymentFailed", CorrelationID: "o1", OrderID: "o1", OccurredAt: time.Now().UTC(), Payload: b}
	out, err := s.applyOutcome(context.Background(), e)
	if err != nil || out.EventType != contracts.EventInventoryReleaseRequested || s.orders["o1"].Status != state.Cancelling {
		t.Fatalf("out=%v err=%v order=%+v", out, err, s.orders["o1"])
	}
}
