package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/KantapatSg/golang-essential-3/contracts"
	inventoryv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/inventory/v1"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
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
	mu           sync.RWMutex
	products     map[string]product
	reservations map[string]reservation
	processed    map[string]bool
}
type reservation struct {
	ID, OrderID, ProductID, Status, Reason string
	Quantity                               int32
	CreatedAt                              time.Time
}
type createdPayload struct {
	Order struct {
		ID              string `json:"id"`
		CustomerID      string `json:"customer_id"`
		TotalMinor      int64  `json:"total_minor"`
		Currency        string `json:"currency"`
		PaymentScenario string `json:"payment_scenario"`
		Items           []struct {
			ProductID string `json:"product_id"`
			Quantity  int32  `json:"quantity"`
		} `json:"items"`
	} `json:"order"`
}
type compensationPayload struct {
	OrderID         string `json:"order_id"`
	CustomerID      string `json:"customer_id"`
	Reason          string `json:"reason"`
	AmountMinor     int64  `json:"amount_minor,omitempty"`
	Currency        string `json:"currency,omitempty"`
	PaymentScenario string `json:"payment_scenario,omitempty"`
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*inventoryv1.Reservation, 0, len(s.reservations))
	for _, r := range s.reservations {
		out = append(out, &inventoryv1.Reservation{Id: r.ID, OrderId: r.OrderID, ProductId: r.ProductID, Quantity: r.Quantity, Status: r.Status, Reason: r.Reason, CreatedAt: r.CreatedAt.Format(time.RFC3339)})
	}
	return &inventoryv1.ListReservationsResponse{Reservations: out, Total: int32(len(out))}, nil
}
func (s *inventoryServer) reserve(e contracts.Envelope) (contracts.Envelope, error) {
	var p createdPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return contracts.Envelope{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.processed[e.EventID] {
		return contracts.Envelope{}, nil
	}
	for _, item := range p.Order.Items {
		pr, ok := s.products[item.ProductID]
		if !ok || pr.available < item.Quantity {
			s.processed[e.EventID] = true
			return s.outcome(e, p.Order.CustomerID, "InventoryRejected", "OUT_OF_STOCK", 0, p.Order.Currency, p.Order.PaymentScenario), nil
		}
	}
	for _, item := range p.Order.Items {
		pr := s.products[item.ProductID]
		pr.available -= item.Quantity
		s.products[item.ProductID] = pr
		id := uuid.NewString()
		s.reservations[id] = reservation{ID: id, OrderID: p.Order.ID, ProductID: item.ProductID, Quantity: item.Quantity, Status: "RESERVED", CreatedAt: time.Now().UTC()}
	}
	s.processed[e.EventID] = true
	return s.outcome(e, p.Order.CustomerID, "InventoryReserved", "", p.Order.TotalMinor, p.Order.Currency, p.Order.PaymentScenario), nil
}
func (s *inventoryServer) release(e contracts.Envelope) (contracts.Envelope, error) {
	var p compensationPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return contracts.Envelope{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.processed[e.EventID] {
		return contracts.Envelope{}, nil
	}
	for id, r := range s.reservations {
		if r.OrderID == p.OrderID && r.Status == "RESERVED" {
			pr := s.products[r.ProductID]
			pr.available += r.Quantity
			s.products[r.ProductID] = pr
			r.Status = "RELEASED"
			s.reservations[id] = r
		}
	}
	s.processed[e.EventID] = true
	return s.outcome(e, p.CustomerID, "InventoryReleased", "", 0, "", ""), nil
}
func (s *inventoryServer) outcome(e contracts.Envelope, customer, eventType, reason string, amount int64, currency, scenario string) contracts.Envelope {
	b, _ := json.Marshal(compensationPayload{OrderID: e.OrderID, CustomerID: customer, Reason: reason, AmountMinor: amount, Currency: currency, PaymentScenario: scenario})
	return contracts.Envelope{SchemaVersion: 1, EventID: uuid.NewString(), EventType: eventType, CorrelationID: e.CorrelationID, CausationID: e.EventID, OccurredAt: time.Now().UTC(), CustomerID: customer, OrderID: e.OrderID, Payload: b}
}
func (s *inventoryServer) consume(ctx context.Context, brokers string) {
	if brokers == "" {
		return
	}
	reader := kafka.NewReader(kafka.ReaderConfig{Brokers: strings.Split(brokers, ","), Topic: contracts.OrderEventsTopic, GroupID: "inventory-reservation-v1", MinBytes: 1, MaxBytes: 1 << 20})
	defer reader.Close()
	writer := &kafka.Writer{Addr: kafka.TCP(strings.Split(brokers, ",")...), Topic: contracts.OrderEventsTopic}
	defer writer.Close()
	for {
		m, err := reader.ReadMessage(ctx)
		if err != nil {
			return
		}
		var e contracts.Envelope
		if json.Unmarshal(m.Value, &e) != nil || e.Validate() != nil {
			continue
		}
		var out contracts.Envelope
		if e.EventType == "OrderCreated" {
			out, err = s.reserve(e)
		} else if e.EventType == "InventoryReleaseRequested" {
			out, err = s.release(e)
		}
		if err == nil && out.EventID != "" {
			b, _ := json.Marshal(out)
			_ = writer.WriteMessages(ctx, kafka.Message{Key: []byte(out.EventID), Value: b})
		}
	}
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
	}, reservations: map[string]reservation{}, processed: map[string]bool{}}
	lis, err := net.Listen("tcp", env("INVENTORY_ADDR", ":50055"))
	if err != nil {
		log.Fatal(err)
	}
	health(ctx, env("INVENTORY_HTTP_ADDR", ":9106"))
	go s.consume(ctx, os.Getenv("KAFKA_BROKERS"))
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
