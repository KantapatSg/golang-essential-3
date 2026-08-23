package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
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
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

//go:embed migrations/001_init.sql
var migrationSQL string

//go:embed migrations/002_indexes.sql
var migration2SQL string
var activityProcessed, activityFailed, activityDuplicate atomic.Uint64
var activityLag atomic.Int64

type activity struct {
	ID, EventID, EventType, TaskID, ActorID string
	OccurredAt                              time.Time
}
type eventPayload struct {
	SchemaVersion int
	EventID       string
	EventType     string
	Task          struct{ ID, OwnerID string }
	OccurredAt    time.Time
}

// Unmarshal both the versioned snake_case envelope and Project 2's native Go
// field names so old outbox rows remain replayable during the contract change.
func (p *eventPayload) UnmarshalJSON(data []byte) error {
	var raw struct {
		SchemaVersion    int       `json:"schema_version"`
		LegacySchema     int       `json:"SchemaVersion"`
		EventID          string    `json:"event_id"`
		LegacyEventID    string    `json:"EventID"`
		EventType        string    `json:"event_type"`
		LegacyEventType  string    `json:"EventType"`
		OccurredAt       time.Time `json:"occurred_at"`
		LegacyOccurredAt time.Time `json:"OccurredAt"`
		Task             struct {
			ID          string `json:"id"`
			LegacyID    string `json:"ID"`
			OwnerID     string `json:"owner_id"`
			LegacyOwner string `json:"OwnerID"`
		} `json:"task"`
		LegacyTask struct {
			ID      string `json:"ID"`
			OwnerID string `json:"OwnerID"`
		} `json:"Task"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.SchemaVersion = raw.SchemaVersion
	if p.SchemaVersion == 0 {
		p.SchemaVersion = raw.LegacySchema
	}
	p.EventID, p.EventType, p.OccurredAt = raw.EventID, raw.EventType, raw.OccurredAt
	if p.EventID == "" {
		p.EventID = raw.LegacyEventID
	}
	if p.EventType == "" {
		p.EventType = raw.LegacyEventType
	}
	if p.OccurredAt.IsZero() {
		p.OccurredAt = raw.LegacyOccurredAt
	}
	p.Task.ID, p.Task.OwnerID = raw.Task.ID, raw.Task.OwnerID
	if p.Task.ID == "" {
		p.Task.ID = raw.Task.LegacyID
	}
	if p.Task.OwnerID == "" {
		p.Task.OwnerID = raw.Task.LegacyOwner
	}
	if p.Task.ID == "" {
		p.Task.ID = raw.LegacyTask.ID
	}
	if p.Task.OwnerID == "" {
		p.Task.OwnerID = raw.LegacyTask.OwnerID
	}
	return nil
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
			out = append(out, &activityv1.Activity{Id: a.ID, EventId: a.EventID, EventType: a.EventType, TaskId: a.TaskID, ActorId: a.ActorID, OccurredAt: a.OccurredAt.Format(time.RFC3339)})
		}
		return &activityv1.ListActivitiesResponse{Activities: out}, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*activityv1.Activity, 0, len(s.items))
	for _, a := range s.items {
		out = append(out, &activityv1.Activity{Id: a.ID, EventId: a.EventID, EventType: a.EventType, TaskId: a.TaskID, ActorId: a.ActorID, OccurredAt: a.OccurredAt.Format(time.RFC3339)})
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
	// FetchMessage แยกการอ่านออกจากการ commit; จึงไม่ยืนยัน offset จน side effect สำเร็จ
	if strings.TrimSpace(brokers) == "" {
		return
	}
	r := kafka.NewReader(activityReaderConfig(brokers))
	defer r.Close()
	for {
		m, e := r.FetchMessage(ctx)
		if e != nil {
			if !errors.Is(e, context.Canceled) {
				log.Printf("kafka read retry: %v", e)
			}
			return
		}
		activityLag.Store(int64(r.Stats().Lag))
		var p eventPayload
		if json.Unmarshal(m.Value, &p) != nil || p.SchemaVersion > 1 || p.EventID == "" || p.EventType == "" || p.Task.ID == "" {
			activityFailed.Add(1)
			log.Printf("DLQ task.events.v1 offset=%d: invalid payload", m.Offset)
			continue
		}
		if e = s.recordEvent(ctx, p); e != nil {
			activityFailed.Add(1)
			log.Printf("activity event retry event=%s: %v", p.EventID, e)
			continue
		}
		activityProcessed.Add(1)
		// Commit หลัง transaction สำเร็จ: ถ้า DB ล่ม event เดิมจะถูกส่งซ้ำและ idempotency กันซ้ำให้เอง
		if e = r.CommitMessages(ctx, m); e != nil {
			log.Printf("activity offset commit retry: %v", e)
		}
	}
}

func activityReaderConfig(brokers string) kafka.ReaderConfig {
	return kafka.ReaderConfig{
		Brokers: strings.Split(brokers, ","), Topic: "task.events.v1", GroupID: "activity-service-v1",
		MinBytes: 1, MaxBytes: 10 << 20, StartOffset: kafka.FirstOffset,
		// Topic อาจถูก auto-create หลัง group เริ่ม จึงต้อง watch partition เพื่อไม่ค้าง assignment ว่าง
		WatchPartitionChanges: true, PartitionWatchInterval: time.Second,
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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
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
		if e = runMigrations(db, []string{migrationSQL, migration2SQL}); e != nil {
			log.Fatal(e)
		}
	}
	lis, e := net.Listen("tcp", env("ACTIVITY_ADDR", ":50053"))
	if e != nil {
		log.Fatal(e)
	}
	s := &activityServer{seen: map[string]struct{}{}, db: db}
	brokers := strings.TrimSpace(os.Getenv("KAFKA_BROKERS"))
	startHealthServer(ctx, env("ACTIVITY_HTTP_ADDR", ":9103"), func(checkCtx context.Context) error {
		if db != nil {
			sqlDB, err := db.DB()
			if err != nil {
				return err
			}
			if err = sqlDB.PingContext(checkCtx); err != nil {
				return err
			}
		}
		if brokers == "" {
			return nil
		}
		conn, err := (&kafka.Dialer{Timeout: 500 * time.Millisecond}).DialContext(checkCtx, "tcp", strings.TrimSpace(strings.Split(brokers, ",")[0]))
		if err != nil {
			return err
		}
		return conn.Close()
	})
	// Kafka consumer ทำงานเป็น background goroutine และรับ context เดียวกับ service lifecycle
	go s.consume(ctx, os.Getenv("KAFKA_BROKERS"))
	g := grpc.NewServer()
	activityv1.RegisterActivityServiceServer(g, s)
	go func() { <-ctx.Done(); g.GracefulStop() }()
	log.Printf("activity-service listening on %s", lis.Addr())
	if e = g.Serve(lis); e != nil && !errors.Is(e, grpc.ErrServerStopped) {
		log.Fatal(e)
	}
	if db != nil {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
}
func startHealthServer(ctx context.Context, addr string, ready func(context.Context) error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, _ *http.Request) {
		checkCtx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
		defer cancel()
		if err := ready(checkCtx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"not_ready"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "# HELP service_ready Whether the activity service can accept traffic.\n# TYPE service_ready gauge\nservice_ready 1\n# HELP kafka_consumer_messages_total Activity events processed.\n# TYPE kafka_consumer_messages_total counter\nkafka_consumer_messages_total{consumer=\"activity\"} %d\n# HELP kafka_consumer_failures_total Activity event failures.\n# TYPE kafka_consumer_failures_total counter\nkafka_consumer_failures_total{consumer=\"activity\"} %d\n# HELP kafka_consumer_lag Messages behind the end of the Kafka partition.\n# TYPE kafka_consumer_lag gauge\nkafka_consumer_lag{consumer=\"activity\"} %d\n", activityProcessed.Load(), activityFailed.Load(), activityLag.Load())
	})
	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(stop)
	}()
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("health server: %v", err)
		}
	}()
}
func runMigrations(db *gorm.DB, migrations []string) error {
	if err := db.Exec("CREATE TABLE IF NOT EXISTS schema_migrations (version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())").Error; err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for i, sql := range migrations {
			if err := tx.Exec(sql).Error; err != nil {
				return err
			}
			if err := tx.Exec("INSERT INTO schema_migrations(version) VALUES (?) ON CONFLICT DO NOTHING", i+1).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func isDev() bool { return strings.EqualFold(os.Getenv("DEV_MODE"), "true") }
