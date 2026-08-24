package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	inventoryv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/inventory/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type product struct {
	id, name, currency string
	price              int64
	available          int32
}
type inventoryServer struct {
	inventoryv1.UnimplementedInventoryServiceServer
	mu       sync.RWMutex
	products map[string]product
}

func (s *inventoryServer) ListProducts(_ context.Context, req *inventoryv1.ListProductsRequest) (*inventoryv1.ListProductsResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	page, size := int(req.GetPage()), int(req.GetPageSize())
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	all := make([]product, 0, len(s.products))
	for _, p := range s.products {
		all = append(all, p)
	}
	start := (page - 1) * size
	if start > len(all) {
		start = len(all)
	}
	end := start + size
	if end > len(all) {
		end = len(all)
	}
	out := make([]*inventoryv1.Product, 0, end-start)
	for _, p := range all[start:end] {
		out = append(out, toProto(p))
	}
	return &inventoryv1.ListProductsResponse{Products: out, Total: int32(len(all))}, nil
}
func (s *inventoryServer) QuoteProducts(_ context.Context, req *inventoryv1.QuoteProductsRequest) (*inventoryv1.QuoteProductsResponse, error) {
	if req == nil || len(req.GetItems()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one item is required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*inventoryv1.Product, 0, len(req.GetItems()))
	var total int64
	currency := "USD"
	for _, item := range req.GetItems() {
		if item.GetQuantity() < 1 || item.GetQuantity() > 10000 {
			return nil, status.Error(codes.InvalidArgument, "quantity must be between 1 and 10000")
		}
		p, ok := s.products[item.GetProductId()]
		if !ok {
			return nil, status.Error(codes.NotFound, "product not found")
		}
		currency = p.currency
		total += p.price * int64(item.GetQuantity())
		out = append(out, toProto(p))
	}
	return &inventoryv1.QuoteProductsResponse{Products: out, TotalMinor: total, Currency: currency}, nil
}
func (s *inventoryServer) ListReservations(context.Context, *inventoryv1.ListReservationsRequest) (*inventoryv1.ListReservationsResponse, error) {
	return &inventoryv1.ListReservationsResponse{Reservations: []*inventoryv1.Reservation{}, Total: 0}, nil
}
func toProto(p product) *inventoryv1.Product {
	return &inventoryv1.Product{Id: p.id, Name: p.name, UnitPriceMinor: p.price, Currency: p.currency, Available: p.available}
}
func health(ctx context.Context, addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	srv := &http.Server{Addr: addr, Handler: mux}
	go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("health: %v", err)
		}
	}()
}
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &inventoryServer{products: map[string]product{
		"prod-coffee": {"prod-coffee", "House Coffee", "USD", 1299, 100},
		"prod-mug":    {"prod-mug", "Portfolio Mug", "USD", 1899, 25},
		"prod-shirt":  {"prod-shirt", "Demo T-Shirt", "USD", 2499, 0},
	}}
	lis, err := net.Listen("tcp", env("INVENTORY_ADDR", ":50055"))
	if err != nil {
		log.Fatal(err)
	}
	health(ctx, env("INVENTORY_HTTP_ADDR", ":9106"))
	grpcServer := grpc.NewServer()
	inventoryv1.RegisterInventoryServiceServer(grpcServer, s)
	log.Printf("inventory-service listening on %s", lis.Addr())
	if err := grpcServer.Serve(lis); err != nil && !strings.Contains(err.Error(), "closed") {
		log.Fatal(err)
	}
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
