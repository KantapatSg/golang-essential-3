package main

import (
	"context"
	"encoding/json"
	"github.com/KantapatSg/golang-essential-3/contracts"
	inventoryv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/inventory/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"testing"
	"time"
)

func TestQuoteUsesAuthoritativeCatalog(t *testing.T) {
	s := &inventoryServer{products: map[string]product{"p": {"p", "Catalog Name", "USD", 250, 7}}}
	r, err := s.QuoteProducts(context.Background(), &inventoryv1.QuoteProductsRequest{Items: []*inventoryv1.ProductRequestItem{{ProductId: "p", Quantity: 2}}})
	if err != nil || r.GetTotalMinor() != 500 || r.GetProducts()[0].GetName() != "Catalog Name" {
		t.Fatalf("quote=%v err=%v", r, err)
	}
}

func TestReserveIsAtomicAndIdempotent(t *testing.T) {
	s := &inventoryServer{products: map[string]product{"p": {"p", "P", "USD", 10, 2}}, reservations: map[string]reservation{}, processed: map[string]bool{}}
	b, _ := json.Marshal(map[string]interface{}{"order": map[string]interface{}{"id": "o1", "customer_id": "u1", "items": []map[string]interface{}{{"product_id": "p", "quantity": 1}}}})
	e := contracts.Envelope{SchemaVersion: 1, EventID: "e1", EventType: "OrderCreated", CorrelationID: "o1", OrderID: "o1", OccurredAt: time.Now().UTC(), Payload: b}
	if _, err := s.reserve(e); err != nil {
		t.Fatal(err)
	}
	if s.products["p"].available != 1 {
		t.Fatalf("available=%d", s.products["p"].available)
	}
	if out, err := s.reserve(e); err != nil || out.EventID != "" {
		t.Fatalf("duplicate out=%v err=%v", out, err)
	}
	if s.products["p"].available != 1 {
		t.Fatal("duplicate changed stock")
	}
}
func TestQuoteRejectsUnknownProduct(t *testing.T) {
	s := &inventoryServer{products: map[string]product{}}
	_, err := s.QuoteProducts(context.Background(), &inventoryv1.QuoteProductsRequest{Items: []*inventoryv1.ProductRequestItem{{ProductId: "missing", Quantity: 1}}})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code=%v", status.Code(err))
	}
}

func TestConfirmedConsumesReservationIdempotently(t *testing.T) {
	s := &inventoryServer{products: map[string]product{"p": {"p", "P", "USD", 10, 1}}, reservations: map[string]reservation{}, processed: map[string]bool{}}
	b, _ := json.Marshal(map[string]interface{}{"order": map[string]interface{}{"id": "o1", "customer_id": "u1", "items": []map[string]interface{}{{"product_id": "p", "quantity": 1}}}})
	created := contracts.Envelope{SchemaVersion: 1, EventID: "created", EventType: contracts.EventOrderCreated, CorrelationID: "o1", OrderID: "o1", CustomerID: "u1", OccurredAt: time.Now().UTC(), Payload: b}
	if _, err := s.reserve(created); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(compensationPayload{OrderID: "o1", CustomerID: "u1"})
	confirmed := contracts.Envelope{SchemaVersion: 1, EventID: "confirmed", EventType: contracts.EventOrderConfirmed, CorrelationID: "o1", OrderID: "o1", CustomerID: "u1", OccurredAt: time.Now().UTC(), Payload: payload}
	out, err := s.consumeReservation(confirmed)
	if err != nil || out.EventType != contracts.EventInventoryConsumed {
		t.Fatalf("out=%v err=%v", out, err)
	}
	for _, r := range s.reservations {
		if r.Status != "CONSUMED" {
			t.Fatalf("reservation=%+v", r)
		}
	}
	if duplicate, err := s.consumeReservation(confirmed); err != nil || duplicate.EventID != "" {
		t.Fatalf("duplicate=%v err=%v", duplicate, err)
	}
}

func TestAdjustStockSignedAndIdempotent(t *testing.T) {
	s := &inventoryServer{products: map[string]product{"p": {"p", "P", "USD", 10, 2}}}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-user-role", "admin"))
	first, err := s.AdjustStock(ctx, &inventoryv1.AdjustStockRequest{ProductId: "p", Delta: 3, Reason: "restock", IdempotencyKey: "stock-1"})
	if err != nil || first.GetProduct().GetAvailable() != 5 || first.GetMovement().GetBalanceAfter() != 5 {
		t.Fatalf("first=%v err=%v", first, err)
	}
	retry, err := s.AdjustStock(ctx, &inventoryv1.AdjustStockRequest{ProductId: "p", Delta: 3, Reason: "restock", IdempotencyKey: "stock-1"})
	if err != nil || !retry.GetReplayed() || s.products["p"].available != 5 {
		t.Fatalf("retry=%v err=%v available=%d", retry, err, s.products["p"].available)
	}
	if _, err := s.AdjustStock(ctx, &inventoryv1.AdjustStockRequest{ProductId: "p", Delta: 2, Reason: "different", IdempotencyKey: "stock-1"}); status.Code(err) != codes.AlreadyExists {
		t.Fatalf("mismatch err=%v", err)
	}
	if _, err := s.AdjustStock(ctx, &inventoryv1.AdjustStockRequest{ProductId: "p", Delta: -6, Reason: "bad", IdempotencyKey: "stock-2"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("negative err=%v", err)
	}
}
