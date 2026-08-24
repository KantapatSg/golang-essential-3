package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/KantapatSg/golang-essential-3/contracts"
	notificationv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/notification/v1"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type notificationRow struct {
	ID            string `gorm:"type:uuid;primaryKey"`
	EventID       string `gorm:"type:uuid;uniqueIndex"`
	UserID        string `gorm:"type:uuid;index"`
	OrderID       string `gorm:"type:uuid;index"`
	Type, Message string
	Read          bool
	CreatedAt     time.Time
}

func (notificationRow) TableName() string { return "order_notifications" }

type notificationServer struct {
	notificationv1.UnimplementedNotificationServiceServer
	mu   sync.RWMutex
	rows map[string]notificationRow
	seen map[string]bool
	db   *gorm.DB
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
func (s *notificationServer) ListNotifications(ctx context.Context, req *notificationv1.ListNotificationsRequest) (*notificationv1.ListNotificationsResponse, error) {
	u, role := actor(ctx)
	if u == "" {
		return nil, status.Error(codes.Unauthenticated, "identity required")
	}
	page, size := int(req.GetPage()), int(req.GetPageSize())
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	if s.db != nil {
		var rows []notificationRow
		q := s.db.WithContext(ctx).Order("created_at desc")
		if role != "admin" {
			q = q.Where("user_id = ?", u)
		}
		var total int64
		q.Model(&notificationRow{}).Count(&total)
		if e := q.Offset((page - 1) * size).Limit(size).Find(&rows).Error; e != nil {
			return nil, status.Error(codes.Internal, "notification query failed")
		}
		return &notificationv1.ListNotificationsResponse{Notifications: toProto(rows), Total: int32(total)}, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []notificationRow{}
	for _, r := range s.rows {
		if role == "admin" || r.UserID == u {
			out = append(out, r)
		}
	}
	return &notificationv1.ListNotificationsResponse{Notifications: toProto(out), Total: int32(len(out))}, nil
}
func (s *notificationServer) UnreadCount(ctx context.Context, _ *notificationv1.UnreadCountRequest) (*notificationv1.UnreadCountResponse, error) {
	u, role := actor(ctx)
	if u == "" {
		return nil, status.Error(codes.Unauthenticated, "identity required")
	}
	if s.db != nil {
		q := s.db.WithContext(ctx).Model(&notificationRow{}).Where("read = false")
		if role != "admin" {
			q = q.Where("user_id = ?", u)
		}
		var n int64
		q.Count(&n)
		return &notificationv1.UnreadCountResponse{Count: int32(n)}, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, r := range s.rows {
		if !r.Read && (role == "admin" || r.UserID == u) {
			n++
		}
	}
	return &notificationv1.UnreadCountResponse{Count: int32(n)}, nil
}
func (s *notificationServer) MarkAsRead(ctx context.Context, req *notificationv1.MarkAsReadRequest) (*notificationv1.Notification, error) {
	u, role := actor(ctx)
	if u == "" {
		return nil, status.Error(codes.Unauthenticated, "identity required")
	}
	if s.db != nil {
		var r notificationRow
		if e := s.db.WithContext(ctx).First(&r, "id = ?", req.GetId()).Error; e != nil {
			return nil, status.Error(codes.NotFound, "notification not found")
		}
		if role != "admin" && r.UserID != u {
			return nil, status.Error(codes.PermissionDenied, "notification ownership required")
		}
		s.db.WithContext(ctx).Model(&r).Update("read", true)
		r.Read = true
		return toProtoOne(r), nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rows[req.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "notification not found")
	}
	if role != "admin" && r.UserID != u {
		return nil, status.Error(codes.PermissionDenied, "notification ownership required")
	}
	r.Read = true
	s.rows[r.ID] = r
	return toProtoOne(r), nil
}
func (s *notificationServer) MarkAllAsRead(ctx context.Context, _ *notificationv1.MarkAllAsReadRequest) (*notificationv1.Empty, error) {
	u, role := actor(ctx)
	if u == "" {
		return nil, status.Error(codes.Unauthenticated, "identity required")
	}
	if s.db != nil {
		q := s.db.WithContext(ctx).Model(&notificationRow{}).Where("read = false")
		if role != "admin" {
			q = q.Where("user_id = ?", u)
		}
		if e := q.Update("read", true).Error; e != nil {
			return nil, status.Error(codes.Internal, "notification update failed")
		}
		return &notificationv1.Empty{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, r := range s.rows {
		if role == "admin" || r.UserID == u {
			r.Read = true
			s.rows[id] = r
		}
	}
	return &notificationv1.Empty{}, nil
}
func toProto(rows []notificationRow) []*notificationv1.Notification {
	out := make([]*notificationv1.Notification, 0, len(rows))
	for _, r := range rows {
		out = append(out, toProtoOne(r))
	}
	return out
}
func toProtoOne(r notificationRow) *notificationv1.Notification {
	return &notificationv1.Notification{Id: r.ID, UserId: r.UserID, OrderId: r.OrderID, Type: r.Type, Message: r.Message, Read: r.Read, CreatedAt: r.CreatedAt.Format(time.RFC3339)}
}

func messageFor(event string) (string, string) {
	switch event {
	case "OrderCreated":
		return "ORDER_CREATED", "Your order was received"
	case "InventoryReserved":
		return "INVENTORY_RESERVED", "Stock was reserved"
	case "InventoryRejected":
		return "INVENTORY_REJECTED", "The requested stock is unavailable"
	case "PaymentCompleted":
		return "PAYMENT_COMPLETED", "Payment completed"
	case "PaymentFailed":
		return "PAYMENT_FAILED", "Payment declined; stock will be released"
	case "OrderConfirmed":
		return "ORDER_CONFIRMED", "Your order is confirmed"
	case "OrderCancelled":
		return "ORDER_CANCELLED", "Your order was cancelled"
	}
	return "", ""
}
func (s *notificationServer) consume(ctx context.Context, brokers string) {
	if brokers == "" {
		return
	}
	r := kafka.NewReader(kafka.ReaderConfig{Brokers: strings.Split(brokers, ","), Topic: contracts.OrderEventsTopic, GroupID: "notification-order-v1", MinBytes: 1, MaxBytes: 1 << 20})
	defer r.Close()
	for {
		m, e := r.FetchMessage(ctx)
		if e != nil {
			return
		}
		var env contracts.Envelope
		if json.Unmarshal(m.Value, &env) != nil || env.Validate() != nil {
			continue
		}
		typ, msg := messageFor(env.EventType)
		if typ == "" {
			continue
		}
		row := notificationRow{ID: uuid.NewString(), EventID: env.EventID, UserID: env.CustomerID, OrderID: env.OrderID, Type: typ, Message: msg, CreatedAt: env.OccurredAt}
		var err error
		if s.db != nil {
			err = s.db.WithContext(ctx).Create(&row).Error
			if err != nil {
				var x notificationRow
				if s.db.First(&x, "event_id = ?", env.EventID).Error == nil {
					err = nil
				}
			}
		} else {
			s.mu.Lock()
			if s.seen[env.EventID] {
				s.mu.Unlock()
				_ = r.CommitMessages(ctx, m)
				continue
			}
			s.seen[env.EventID] = true
			s.rows[row.ID] = row
			s.mu.Unlock()
		}
		if err == nil {
			_ = r.CommitMessages(ctx, m)
		}
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
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &notificationServer{rows: map[string]notificationRow{}, seen: map[string]bool{}}
	if dsn := os.Getenv("NOTIFICATION_DB_DSN"); dsn != "" {
		db, e := openDB(ctx, dsn)
		if e != nil {
			log.Fatal(e)
		}
		s.db = db
		if e = db.AutoMigrate(&notificationRow{}); e != nil {
			log.Fatal(e)
		}
	}
	lis, e := net.Listen("tcp", env("NOTIFICATION_ADDR", ":50058"))
	if e != nil {
		log.Fatal(e)
	}
	go s.consume(ctx, os.Getenv("KAFKA_BROKERS"))
	health(ctx, env("NOTIFICATION_HTTP_ADDR", ":9109"), s.db)
	g := grpc.NewServer()
	notificationv1.RegisterNotificationServiceServer(g, s)
	log.Printf("notification-service listening on %s", lis.Addr())
	if e = g.Serve(lis); e != nil && !strings.Contains(e.Error(), "closed") {
		log.Fatal(e)
	}
}
func health(ctx context.Context, addr string, db *gorm.DB) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		if db != nil {
			if x, e := db.DB(); e != nil || x.PingContext(r.Context()) != nil {
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
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
