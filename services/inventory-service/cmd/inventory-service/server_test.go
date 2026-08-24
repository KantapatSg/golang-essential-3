package main

import (
	"context"
	inventoryv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/inventory/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
)

func TestQuoteUsesAuthoritativeCatalog(t *testing.T) {
	s := &inventoryServer{products: map[string]product{"p": {"p", "Catalog Name", "USD", 250, 7}}}
	r, err := s.QuoteProducts(context.Background(), &inventoryv1.QuoteProductsRequest{Items: []*inventoryv1.ProductRequestItem{{ProductId: "p", Quantity: 2}}})
	if err != nil || r.GetTotalMinor() != 500 || r.GetProducts()[0].GetName() != "Catalog Name" {
		t.Fatalf("quote=%v err=%v", r, err)
	}
}
func TestQuoteRejectsUnknownProduct(t *testing.T) {
	s := &inventoryServer{products: map[string]product{}}
	_, err := s.QuoteProducts(context.Background(), &inventoryv1.QuoteProductsRequest{Items: []*inventoryv1.ProductRequestItem{{ProductId: "missing", Quantity: 1}}})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code=%v", status.Code(err))
	}
}
