package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	gen "github.com/KantapatSg/golang-essential-3/contracts/gen/go"
	activityv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/activity/v1"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

//go:embed migrations/001_init.sql
var migrationSQL string

type activity struct {
	ID, EventID, EventType, TaskID, ActorID string
	OccurredAt                              time.Time
}
type eventPayload struct {
	EventID, EventType string
	Task               struct{ ID, OwnerID string }
	OccurredAt         time.Time
}
type activityRow struct {
	ID         string `gorm:"type:uuid;primaryKey"`
	EventID    string `gorm:"type:uuid;uniqueIndex"`
	EventType  string
	TaskID     string `gorm:"type:uuid"`
	ActorID    string `gorm:"type:uuid"`
	OccurredAt time.Time
}
type processedEvent struct {
	EventID     string `gorm:"type:uuid;primaryKey"`
	ProcessedAt time.Time
}

func (activityRow) TableName() string    { return "activity_logs" }
func (processedEvent) TableName() string { return "processed_events" }

type activityServer struct {
	activityv1.UnimplementedActivityServiceServer
	mu    sync.RWMutex
	items []activity
	seen  map[string]struct{}
	db    *gorm.DB
}

func (s *activityServer) ListActivities(ctx context.Context, _ *activityv1.ListActivitiesRequest) (*activityv1.ListActivitiesResponse, error) {
	// Activity เป็น read model ที่สร้างจาก event ไม่ได้ join ข้ามฐานข้อมูลของ Task Service
	// จึงรักษา database ownership และยอมรับ eventual consistency โดยตั้งใจ
	if s.db != nil {
		var rows []activityRow
		if e := s.db.WithContext(ctx).Order("occurred_at desc").Find(&rows).Error; e != nil {
			return nil, status.Error(codes.Internal, e.Error())
		}
		out := make([]*activityv1.Activity, 0, len(rows))
		for _, a := range rows {
			out = append(out, &activityv1.Activity{ID: a.ID, EventID: a.EventID, EventType: a.EventType, TaskID: a.TaskID, ActorID: a.ActorID, OccurredAt: a.OccurredAt.Format(time.RFC3339)})
		}
		return &activityv1.ListActivitiesResponse{Activities: out}, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*activityv1.Activity, 0, len(s.items))
	for _, a := range s.items {
		out = append(out, &activityv1.Activity{ID: a.ID, EventID: a.EventID, EventType: a.EventType, TaskID: a.TaskID, ActorID: a.ActorID, OccurredAt: a.OccurredAt.Format(time.RFC3339)})
	}
	return &activityv1.ListActivitiesResponse{Activities: out}, nil
}

// recordEvent บันทึก idempotency key และ activity ใน transaction เดียวกัน
// unique key ในฐานข้อมูลเป็นด่านสุดท้ายเมื่อ Kafka ส่ง event เดิมซ้ำ
func (s *activityServer) recordEvent(ctx context.Context, p eventPayload) error {
	if s.db != nil {
		a := activityRow{ID: uuid.NewString(), EventID: p.EventID, EventType: p.EventType, TaskID: p.Task.ID, ActorID: p.Task.OwnerID, OccurredAt: p.OccurredAt}
		e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if e := tx.Create(&processedEvent{EventID: p.EventID, ProcessedAt: time.Now().UTC()}).Error; e != nil {
				var existing processedEvent
				if tx.Where("event_id = ?", p.EventID).First(&existing).Error == nil {
					return nil
				}
				return e
			}
			return tx.Create(&a).Error
		})
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.seen[p.EventID]; ok {
		return nil
	}
	s.seen[p.EventID] = struct{}{}
	s.items = append(s.items, activity{ID: uuid.NewString(), EventID: p.EventID, EventType: p.EventType, TaskID: p.Task.ID, ActorID: p.Task.OwnerID, OccurredAt: p.OccurredAt})
	return nil
}
func (s *activityServer) consume(ctx context.Context, brokers string) {
	// Consumer group ของ Activity แยกจาก Analytics เพื่อให้ทั้งสอง bounded context ได้รับ event ชุดเดียวกัน
	// Phase 3 ต้องเปลี่ยนเป็น FetchMessage + CommitMessages หลัง DB commit เพื่อควบคุม offset อย่างชัดเจน
	if strings.TrimSpace(brokers) == "" {
		return
	}
	r := kafka.NewReader(kafka.ReaderConfig{Brokers: strings.Split(brokers, ","), Topic: "task.events.v1", GroupID: "activity-service-v1", MinBytes: 1, MaxBytes: 10 << 20})
	defer r.Close()
	for {
		m, e := r.ReadMessage(ctx)
		if e != nil {
			if !errors.Is(e, context.Canceled) {
				log.Printf("kafka read retry: %v", e)
			}
			return
		}
		var p eventPayload
		if json.Unmarshal(m.Value, &p) != nil || p.EventID == "" {
			log.Printf("DLQ task.events.v1 offset=%d: invalid payload", m.Offset)
			continue
		}
		if e = s.recordEvent(ctx, p); e != nil {
			log.Printf("activity event retry event=%s: %v", p.EventID, e)
		}
	}
}
func openDB(ctx context.Context, dsn string) (*gorm.DB, error) {
	for i := 0; i < 10; i++ {
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			sqlDB, _ := db.DB()
			if err = sqlDB.PingContext(ctx); err == nil {
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
	dsn := os.Getenv("ACTIVITY_DB_DSN")
	if dsn == "" && !isDev() {
		log.Fatal("ACTIVITY_DB_DSN is required")
	}
	var db *gorm.DB
	var e error
	if dsn != "" {
		db, e = openDB(ctx, dsn)
		if e != nil {
			log.Fatal(e)
		}
		if e = db.Exec(migrationSQL).Error; e != nil {
			log.Fatal(e)
		}
	}
	lis, e := net.Listen("tcp", env("ACTIVITY_ADDR", ":50053"))
	if e != nil {
		log.Fatal(e)
	}
	s := &activityServer{seen: map[string]struct{}{}, db: db}
	// Kafka consumer ทำงานเป็น background goroutine และรับ context เดียวกับ service lifecycle
	go s.consume(ctx, os.Getenv("KAFKA_BROKERS"))
	g := grpc.NewServer(grpc.ForceServerCodec(gen.JSONCodec{}))
	activityv1.RegisterActivityServiceServer(g, s)
	go func() { <-ctx.Done(); g.GracefulStop() }()
	log.Printf("activity-service listening on %s", lis.Addr())
	if e = g.Serve(lis); e != nil && !errors.Is(e, grpc.ErrServerStopped) {
		log.Fatal(e)
	}
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func isDev() bool { return strings.EqualFold(os.Getenv("DEV_MODE"), "true") }
