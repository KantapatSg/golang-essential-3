package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	orderv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/order/v1"
	"github.com/KantapatSg/golang-essential-3/services/order-service/internal/state"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type orderRow struct {
	ID              string `gorm:"type:uuid;primaryKey"`
	CustomerID      string `gorm:"type:uuid;index"`
	Status          string
	PaymentScenario string
	TotalMinor      int64
	Currency        string
	Reason          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Items           []itemRow `gorm:"foreignKey:OrderID"`
}
type itemRow struct {
	ID             string `gorm:"type:uuid;primaryKey"`
	OrderID        string `gorm:"type:uuid;index"`
	ProductID      string
	Name           string
	Quantity       int32
	UnitPriceMinor int64
	LineTotalMinor int64
	Currency       string
}
type outboxRow struct {
	EventID     string `gorm:"type:uuid;primaryKey"`
	EventType   string
	AggregateID string `gorm:"index"`
	Payload     string
	OccurredAt  time.Time
	PublishedAt *time.Time
}
type idemRow struct {
	Key         string `gorm:"primaryKey"`
	CustomerID  string `gorm:"index"`
	RequestHash string
	OrderID     string
	CreatedAt   time.Time
}

func (orderRow) TableName() string  { return "orders" }
func (itemRow) TableName() string   { return "order_items" }
func (outboxRow) TableName() string { return "order_outbox" }
func (idemRow) TableName() string   { return "order_idempotency" }

type eventPayload struct {
	Order orderPayload `json:"order"`
}
type orderPayload struct {
	ID              string        `json:"id"`
	CustomerID      string        `json:"customer_id"`
	Items           []itemPayload `json:"items"`
	TotalMinor      int64         `json:"total_minor"`
	Currency        string        `json:"currency"`
	PaymentScenario string        `json:"payment_scenario"`
}
type itemPayload struct {
	ProductID      string `json:"product_id"`
	Name           string `json:"name"`
	Quantity       int32  `json:"quantity"`
	UnitPriceMinor int64  `json:"unit_price_minor"`
	Currency       string `json:"currency"`
}

type orderServer struct {
	orderv1.UnimplementedOrderServiceServer
	mu          sync.RWMutex
	orders      map[string]*orderRow
	idempotency map[string]idem
	inventory   inventoryv1.InventoryServiceClient
	db          *gorm.DB
}
type idem struct{ customer, hash, orderID string }
type outcomePayload struct {
	OrderID     string `json:"order_id"`
	CustomerID  string `json:"customer_id"`
	Reason      string `json:"reason"`
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
}

func actor(ctx context.Context) (string, string) {
	md, _ := metadata.FromIncomingContext(ctx)
	id, role := "", "member"
	if x := md.Get("x-user-id"); len(x) > 0 {
		id = x[0]
	}
	if x := md.Get("x-user-role"); len(x) > 0 {
		role = x[0]
	}
	return id, role
}
func (s *orderServer) CreateOrder(ctx context.Context, req *orderv1.CreateOrderRequest) (*orderv1.Order, error) {
	if req == nil || len(req.GetItems()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one item is required")
	}
	customer, role := actor(ctx)
	if role == "admin" && req.GetCustomerId() != "" {
		customer = req.GetCustomerId()
	}
	if customer == "" {
		customer = req.GetCustomerId()
	}
	if customer == "" {
		return nil, status.Error(codes.Unauthenticated, "customer identity required")
	}
	// Gateway ส่ง identity ผ่าน metadata; member ไม่สามารถกำหนด customer อื่นใน body เพื่อข้าม ownership ได้
	if strings.TrimSpace(req.GetIdempotencyKey()) == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency-key is required")
	}
	for _, i := range req.GetItems() {
		if i.GetQuantity() < 1 || i.GetQuantity() > 10000 {
			return nil, status.Error(codes.InvalidArgument, "quantity must be between 1 and 10000")
		}
	}
	hash := requestHash(req.GetItems(), req.GetPaymentScenario())
	key := customer + ":" + req.GetIdempotencyKey()
	s.mu.RLock()
	previous, ok := s.idempotency[key]
	s.mu.RUnlock()
	if ok {
		if previous.hash != hash {
			return nil, status.Error(codes.AlreadyExists, "idempotency key payload conflict")
		}
		return s.getByID(ctx, previous.orderID, customer, role)
	}
	if s.db != nil {
		var row idemRow
		if e := s.db.WithContext(ctx).First(&row, "key = ?", key).Error; e == nil {
			if row.RequestHash != hash {
				return nil, status.Error(codes.AlreadyExists, "idempotency key payload conflict")
			}
			return s.getByID(ctx, row.OrderID, customer, role)
		}
	}
	if s.inventory == nil {
		return nil, status.Error(codes.Unavailable, "inventory quote unavailable")
	}
	// Quote เป็น synchronous dependency เพื่อ snapshot ราคา authoritative; การ reserve stock จะเริ่มหลัง OrderCreated ผ่าน Kafka
	quoteItems := make([]*inventoryv1.ProductRequestItem, 0, len(req.GetItems()))
	for _, i := range req.GetItems() {
		quoteItems = append(quoteItems, &inventoryv1.ProductRequestItem{ProductId: i.GetProductId(), Quantity: i.GetQuantity()})
	}
	quote, err := s.inventory.QuoteProducts(ctx, &inventoryv1.QuoteProductsRequest{Items: quoteItems})
	if err != nil {
		return nil, status.Error(codes.Unavailable, "inventory quote unavailable")
	}
	if len(quote.GetProducts()) != len(req.GetItems()) {
		return nil, status.Error(codes.FailedPrecondition, "catalog quote mismatch")
	}
	now := time.Now().UTC()
	row := &orderRow{ID: uuid.NewString(), CustomerID: customer, Status: "PENDING", PaymentScenario: normalizeScenario(req.GetPaymentScenario()), TotalMinor: quote.GetTotalMinor(), Currency: quote.GetCurrency(), CreatedAt: now, UpdatedAt: now}
	for i, item := range req.GetItems() {
		p := quote.GetProducts()[i]
		row.Items = append(row.Items, itemRow{ID: uuid.NewString(), OrderID: row.ID, ProductID: item.GetProductId(), Name: p.GetName(), Quantity: item.GetQuantity(), UnitPriceMinor: p.GetUnitPriceMinor(), LineTotalMinor: p.GetUnitPriceMinor() * int64(item.GetQuantity()), Currency: p.GetCurrency()})
	}
	payload := eventPayload{Order: orderPayload{ID: row.ID, CustomerID: row.CustomerID, Items: make([]itemPayload, 0, len(row.Items)), TotalMinor: row.TotalMinor, Currency: row.Currency, PaymentScenario: row.PaymentScenario}}
	for _, i := range row.Items {
		payload.Order.Items = append(payload.Order.Items, itemPayload{ProductID: i.ProductID, Name: i.Name, Quantity: i.Quantity, UnitPriceMinor: i.UnitPriceMinor, Currency: i.Currency})
	}
	b, _ := json.Marshal(payload)
	envelope := contracts.Envelope{SchemaVersion: 1, EventID: uuid.NewString(), EventType: "OrderCreated", CorrelationID: row.ID, OrderID: row.ID, CustomerID: row.CustomerID, OccurredAt: now, Payload: b}
	envelopeBytes, _ := json.Marshal(envelope)
	if s.db != nil {
		// Aggregate และ outbox ต้อง commit transaction เดียวกัน; Kafka ล่มหลังจากนี้ให้ publisher retry จากแถวที่ยังค้าง
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(row).Error; err != nil {
				return err
			}
			if err := tx.Create(&outboxRow{EventID: envelope.EventID, EventType: envelope.EventType, AggregateID: row.ID, Payload: string(envelopeBytes), OccurredAt: now}).Error; err != nil {
				return err
			}
			return tx.Create(&idemRow{Key: key, CustomerID: customer, RequestHash: hash, OrderID: row.ID, CreatedAt: now}).Error
		}); err != nil {
			return nil, status.Error(codes.Internal, "order transaction failed")
		}
	}
	s.mu.Lock()
	s.orders[row.ID] = row
	s.idempotency[key] = idem{customer: customer, hash: hash, orderID: row.ID}
	s.mu.Unlock()
	return toProto(row), nil
}
func (s *orderServer) ListOrders(ctx context.Context, req *orderv1.ListOrdersRequest) (*orderv1.ListOrdersResponse, error) {
	customer, role := actor(ctx)
	if customer == "" {
		return nil, status.Error(codes.Unauthenticated, "customer identity required")
	}
	page, size := int(req.GetPage()), int(req.GetPageSize())
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	if s.db != nil {
		var rows []orderRow
		q := s.db.WithContext(ctx).Preload("Items")
		if role != "admin" {
			q = q.Where("customer_id = ?", customer)
		}
		var total int64
		if e := q.Model(&orderRow{}).Count(&total).Error; e != nil {
			return nil, status.Error(codes.Internal, "order query failed")
		}
		if e := q.Order("created_at desc").Offset((page - 1) * size).Limit(size).Find(&rows).Error; e != nil {
			return nil, status.Error(codes.Internal, "order query failed")
		}
		out := make([]*orderv1.Order, 0, len(rows))
		for i := range rows {
			out = append(out, toProto(&rows[i]))
		}
		return &orderv1.ListOrdersResponse{Orders: out, Total: int32(total)}, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*orderv1.Order{}
	for _, r := range s.orders {
		if role == "admin" || r.CustomerID == customer {
			out = append(out, toProto(r))
		}
	}
	return &orderv1.ListOrdersResponse{Orders: out, Total: int32(len(out))}, nil
}
func (s *orderServer) GetOrder(ctx context.Context, req *orderv1.GetOrderRequest) (*orderv1.Order, error) {
	customer, role := actor(ctx)
	if req == nil || !validID(req.GetId()) {
		return nil, status.Error(codes.InvalidArgument, "id must be UUID")
	}
	return s.getByID(ctx, req.GetId(), customer, role)
}
func (s *orderServer) getByID(ctx context.Context, id, customer, role string) (*orderv1.Order, error) {
	var row orderRow
	var ok bool
	if s.db != nil {
		e := s.db.WithContext(ctx).Preload("Items").First(&row, "id = ?", id).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		if e != nil {
			return nil, status.Error(codes.Internal, "order query failed")
		}
		ok = true
	} else {
		s.mu.RLock()
		rowPtr := s.orders[id]
		if rowPtr != nil {
			row = *rowPtr
			ok = true
		}
		s.mu.RUnlock()
	}
	if !ok {
		return nil, status.Error(codes.NotFound, "order not found")
	}
	if role != "admin" && row.CustomerID != customer {
		return nil, status.Error(codes.PermissionDenied, "order ownership required")
	}
	return toProto(&row), nil
}
func toProto(r *orderRow) *orderv1.Order {
	items := make([]*orderv1.OrderItem, 0, len(r.Items))
	for _, i := range r.Items {
		items = append(items, &orderv1.OrderItem{ProductId: i.ProductID, Name: i.Name, Quantity: i.Quantity, UnitPrice: &orderv1.Money{AmountMinor: i.UnitPriceMinor, Currency: i.Currency}, LineTotal: &orderv1.Money{AmountMinor: i.LineTotalMinor, Currency: i.Currency}})
	}
	return &orderv1.Order{Id: r.ID, CustomerId: r.CustomerID, Items: items, Total: &orderv1.Money{AmountMinor: r.TotalMinor, Currency: r.Currency}, Status: r.Status, PaymentScenario: r.PaymentScenario, Reason: r.Reason, CreatedAt: r.CreatedAt.Format(time.RFC3339), UpdatedAt: r.UpdatedAt.Format(time.RFC3339)}
}
func validID(v string) bool { _, e := uuid.Parse(v); return e == nil }
func requestHash(items []*orderv1.CreateOrderItem, scenario string) string {
	b, _ := json.Marshal(struct {
		I []*orderv1.CreateOrderItem
		S string
	}{items, normalizeScenario(scenario)})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func normalizeScenario(v string) string {
	if strings.EqualFold(v, "decline") {
		return "decline"
	}
	return "success"
}
func (s *orderServer) publishOutbox(ctx context.Context, brokers string) {
	if s.db == nil || brokers == "" {
		return
	}
	writer := &kafka.Writer{Addr: kafka.TCP(strings.Split(brokers, ",")...), Topic: contracts.OrderEventsTopic}
	defer writer.Close()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var rows []outboxRow
			if e := s.db.WithContext(ctx).Where("published_at IS NULL").Order("occurred_at").Limit(50).Find(&rows).Error; e != nil {
				continue
			}
			for _, row := range rows {
				if e := writer.WriteMessages(ctx, kafka.Message{Key: []byte(row.EventID), Value: []byte(row.Payload)}); e != nil {
					continue
				}
				now := time.Now().UTC()
				_ = s.db.WithContext(ctx).Model(&outboxRow{}).Where("event_id = ? AND published_at IS NULL", row.EventID).Update("published_at", now).Error
			}
		}
	}
}
func (s *orderServer) applyOutcome(ctx context.Context, e contracts.Envelope) (contracts.Envelope, error) {
	var p outcomePayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return contracts.Envelope{}, err
	}
	if p.OrderID == "" {
		p.OrderID = e.OrderID
	}
	if s.db != nil {
		var row orderRow
		if err := s.db.WithContext(ctx).First(&row, "id = ?", p.OrderID).Error; err != nil {
			return contracts.Envelope{}, err
		}
		next, err := state.Transition(row.Status, e.EventType)
		if err != nil || (next == row.Status && e.EventType != "PaymentFailed") {
			return contracts.Envelope{}, err
		}
		if next != row.Status {
			if err := s.db.WithContext(ctx).Model(&orderRow{}).Where("id = ?", p.OrderID).Updates(map[string]interface{}{"status": next, "reason": p.Reason, "updated_at": time.Now().UTC()}).Error; err != nil {
				return contracts.Envelope{}, err
			}
		}
	} else {
		s.mu.Lock()
		row := s.orders[p.OrderID]
		if row == nil {
			s.mu.Unlock()
			return contracts.Envelope{}, nil
		}
		next, err := state.Transition(row.Status, e.EventType)
		if err != nil || (next == row.Status && e.EventType != "PaymentFailed") {
			s.mu.Unlock()
			return contracts.Envelope{}, err
		}
		if next != row.Status {
			row.Status, row.Reason, row.UpdatedAt = next, p.Reason, time.Now().UTC()
		}
		s.mu.Unlock()
	}
	if e.EventType == "PaymentFailed" {
		// Payment decline เป็น terminal state; คำสั่ง release เป็น compensation ที่ส่งซ้ำได้
		payload, _ := json.Marshal(outcomePayload{OrderID: p.OrderID, CustomerID: p.CustomerID, Reason: p.Reason})
		return contracts.Envelope{SchemaVersion: 1, EventID: uuid.NewString(), EventType: "InventoryReleaseRequested", CorrelationID: e.CorrelationID, CausationID: e.EventID, OccurredAt: time.Now().UTC(), CustomerID: p.CustomerID, OrderID: p.OrderID, Payload: payload}, nil
	}
	return contracts.Envelope{}, nil
}
func (s *orderServer) consumeOutcomes(ctx context.Context, brokers string) {
	if brokers == "" {
		return
	}
	reader := kafka.NewReader(kafka.ReaderConfig{Brokers: strings.Split(brokers, ","), Topic: contracts.OrderEventsTopic, GroupID: "order-state-v1", MinBytes: 1, MaxBytes: 1 << 20})
	defer reader.Close()
	writer := &kafka.Writer{Addr: kafka.TCP(strings.Split(brokers, ",")...), Topic: contracts.OrderEventsTopic}
	defer writer.Close()
	processed := map[string]bool{}
	for {
		m, err := reader.ReadMessage(ctx)
		if err != nil {
			return
		}
		var e contracts.Envelope
		if json.Unmarshal(m.Value, &e) != nil || e.Validate() != nil {
			continue
		}
		if e.EventType != "InventoryReserved" && e.EventType != "InventoryRejected" && e.EventType != "PaymentCompleted" && e.EventType != "PaymentFailed" {
			continue
		}
		if processed[e.EventID] {
			continue
		}
		out, err := s.applyOutcome(ctx, e)
		if err != nil {
			continue
		}
		if out.EventID != "" {
			b, _ := json.Marshal(out)
			if err := writer.WriteMessages(ctx, kafka.Message{Key: []byte(out.EventID), Value: b}); err != nil {
				log.Printf("release publish retry event=%s: %v", out.EventID, err)
				continue
			}
		}
		processed[e.EventID] = true
	}
}
func openDB(ctx context.Context, dsn string) (*gorm.DB, error) {
	for i := 0; i < 10; i++ {
		db, e := gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if e == nil {
			sqlDB, _ := db.DB()
			if e = sqlDB.PingContext(ctx); e == nil {
				return db, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return nil, errors.New("database unavailable")
}
func health(ctx context.Context, addr string, db *gorm.DB) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		if db != nil {
			if sqlDB, e := db.DB(); e != nil || sqlDB.PingContext(r.Context()) != nil {
				w.WriteHeader(503)
				return
			}
		}
		w.WriteHeader(200)
	})
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
	s := &orderServer{orders: map[string]*orderRow{}, idempotency: map[string]idem{}}
	if dsn := os.Getenv("ORDER_DB_DSN"); dsn != "" {
		db, e := openDB(ctx, dsn)
		if e != nil {
			log.Fatal(e)
		}
		s.db = db
		if e = db.AutoMigrate(&orderRow{}, &itemRow{}, &outboxRow{}, &idemRow{}); e != nil {
			log.Fatal(e)
		}
	}
	invAddr := os.Getenv("INVENTORY_ADDR")
	if invAddr == "" {
		invAddr = ":50055"
	}
	conn, e := grpc.Dial(invAddr, grpc.WithInsecure())
	if e != nil {
		log.Fatal(e)
	}
	defer conn.Close()
	s.inventory = inventoryv1.NewInventoryServiceClient(conn)
	lis, e := net.Listen("tcp", env("ORDER_ADDR", ":50052"))
	if e != nil {
		log.Fatal(e)
	}
	health(ctx, env("ORDER_HTTP_ADDR", ":9107"), s.db)
	go s.publishOutbox(ctx, os.Getenv("KAFKA_BROKERS"))
	go s.consumeOutcomes(ctx, os.Getenv("KAFKA_BROKERS"))
	g := grpc.NewServer(grpc.UnaryInterceptor(deadlineInterceptor))
	orderv1.RegisterOrderServiceServer(g, s)
	log.Printf("order-service listening on %s", lis.Addr())
	if e = g.Serve(lis); e != nil && !strings.Contains(e.Error(), "closed") {
		log.Fatal(e)
	}
}
func deadlineInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}
	return handler(ctx, req)
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
