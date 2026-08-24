package main

import (
	"context"
	inventoryv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/inventory/v1"
	orderv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/order/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"testing"
)

type inventoryStub struct {
	inventoryv1.InventoryServiceClient
	calls int
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
