package main

import (
	"context"
	"encoding/json"
	"github.com/KantapatSg/golang-essential-3/contracts"
	inventoryv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/inventory/v1"
	"google.golang.org/grpc/codes"
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
