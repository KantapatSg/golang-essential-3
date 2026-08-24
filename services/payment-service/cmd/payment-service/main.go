package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/KantapatSg/golang-essential-3/contracts"
	paymentv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/payment/v1"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type payment struct {
	ID, OrderID, Status, Currency, Reason string
	AmountMinor                           int64
	CreatedAt                             time.Time
}
type paymentServer struct {
	paymentv1.UnimplementedPaymentServiceServer
	mu        sync.RWMutex
	payments  map[string]payment
	processed map[string]bool
}
type paymentEvent struct {
	OrderID         string `json:"order_id"`
	CustomerID      string `json:"customer_id"`
	Reason          string `json:"reason"`
	AmountMinor     int64  `json:"amount_minor"`
	Currency        string `json:"currency"`
	PaymentScenario string `json:"payment_scenario"`
}

func (s *paymentServer) ListPayments(context.Context, *paymentv1.ListPaymentsRequest) (*paymentv1.ListPaymentsResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*paymentv1.Payment, 0, len(s.payments))
	for _, p := range s.payments {
		out = append(out, &paymentv1.Payment{Id: p.ID, OrderId: p.OrderID, Status: p.Status, AmountMinor: p.AmountMinor, Currency: p.Currency, Reason: p.Reason, CreatedAt: p.CreatedAt.Format(time.RFC3339)})
	}
	return &paymentv1.ListPaymentsResponse{Payments: out, Total: int32(len(out))}, nil
}
func (s *paymentServer) GetPayment(_ context.Context, req *paymentv1.GetPaymentRequest) (*paymentv1.Payment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.payments {
		if p.OrderID == req.GetOrderId() {
			return &paymentv1.Payment{Id: p.ID, OrderId: p.OrderID, Status: p.Status, AmountMinor: p.AmountMinor, Currency: p.Currency, Reason: p.Reason, CreatedAt: p.CreatedAt.Format(time.RFC3339)}, nil
		}
	}
	return nil, errors.New("payment not found")
}
func (s *paymentServer) consume(ctx context.Context, brokers string) {
	if brokers == "" {
		return
	}
	r := kafka.NewReader(kafka.ReaderConfig{Brokers: strings.Split(brokers, ","), Topic: contracts.OrderEventsTopic, GroupID: "payment-v1", MinBytes: 1, MaxBytes: 1 << 20})
	defer r.Close()
	w := &kafka.Writer{Addr: kafka.TCP(strings.Split(brokers, ",")...), Topic: contracts.OrderEventsTopic}
	defer w.Close()
	for {
		m, e := r.ReadMessage(ctx)
		if e != nil {
			return
		}
		var env contracts.Envelope
		if json.Unmarshal(m.Value, &env) != nil || env.Validate() != nil || env.EventType != "InventoryReserved" {
			continue
		}
		var in paymentEvent
		if json.Unmarshal(env.Payload, &in) != nil {
			continue
		}
		s.mu.Lock()
		if s.processed[env.EventID] {
			s.mu.Unlock()
			continue
		}
		scenario := strings.ToLower(in.PaymentScenario)
		if scenario == "" {
			scenario = "success"
		}
		status, reason, typ := "COMPLETED", "", "PaymentCompleted"
		if scenario == "decline" {
			status, reason, typ = "FAILED", "PAYMENT_DECLINED", "PaymentFailed"
		}
		p := payment{ID: uuid.NewString(), OrderID: env.OrderID, Status: status, Currency: in.Currency, Reason: reason, AmountMinor: in.AmountMinor, CreatedAt: time.Now().UTC()}
		s.payments[p.ID] = p
		s.processed[env.EventID] = true
		s.mu.Unlock()
		payload, _ := json.Marshal(paymentEvent{OrderID: env.OrderID, CustomerID: env.CustomerID, Reason: reason, AmountMinor: in.AmountMinor, Currency: in.Currency})
		out := contracts.Envelope{SchemaVersion: 1, EventID: uuid.NewString(), EventType: typ, CorrelationID: env.CorrelationID, CausationID: env.EventID, OccurredAt: time.Now().UTC(), CustomerID: env.CustomerID, OrderID: env.OrderID, Payload: payload}
		b, _ := json.Marshal(out)
		if e = w.WriteMessages(ctx, kafka.Message{Key: []byte(out.EventID), Value: b}); e != nil {
			log.Printf("payment publish retry event=%s: %v", out.EventID, e)
		}
	}
}
func health(ctx context.Context, addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	srv := &http.Server{Addr: addr, Handler: mux}
	go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
	go func() {
		if e := srv.ListenAndServe(); e != nil && !errors.Is(e, http.ErrServerClosed) {
			log.Printf("health: %v", e)
		}
	}()
}
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &paymentServer{payments: map[string]payment{}, processed: map[string]bool{}}
	lis, e := net.Listen("tcp", env("PAYMENT_ADDR", ":50057"))
	if e != nil {
		log.Fatal(e)
	}
	health(ctx, env("PAYMENT_HTTP_ADDR", ":9108"))
	go s.consume(ctx, os.Getenv("KAFKA_BROKERS"))
	g := grpc.NewServer()
	paymentv1.RegisterPaymentServiceServer(g, s)
	log.Printf("payment-service listening on %s", lis.Addr())
	if e = g.Serve(lis); e != nil && !strings.Contains(e.Error(), "closed") {
		log.Fatal(e)
	}
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
