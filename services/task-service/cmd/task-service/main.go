package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	gen "github.com/KantapatSg/golang-essential-3/contracts/gen/go"
	taskv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/task/v1"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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

type task struct {
	ID                   string `gorm:"type:uuid;primaryKey"`
	OwnerID              string `gorm:"type:uuid;index"`
	Title                string
	Description          string
	Status               string
	CreatedAt, UpdatedAt time.Time
}
type outboxEvent struct {
	EventID, EventType string
	Task               task
	OccurredAt         time.Time
}
type outboxRow struct {
	EventID     string `gorm:"type:uuid;primaryKey"`
	EventType   string
	Payload     string
	OccurredAt  time.Time
	PublishedAt *time.Time
}

func (outboxRow) TableName() string { return "outbox" }

type taskServer struct {
	taskv1.UnimplementedTaskServiceServer
	mu             sync.RWMutex
	data           map[string]task
	cache          map[string][]task
	outbox         chan outboxEvent
	writer, reader *gorm.DB
	redis          *redis.Client
}

func actor(ctx context.Context) (string, string) {
	// Identity ที่ Gateway ส่งมาเป็น input ของ authorization ไม่ใช่ business data ที่ client กำหนดเอง
	// Service จึงต้องตรวจ ownership ทุกครั้งก่อนอ่านหรือแก้ Task
	md, _ := metadata.FromIncomingContext(ctx)
	u, r := "", "member"
	if x := md.Get("x-user-id"); len(x) > 0 {
		u = x[0]
	}
	if x := md.Get("x-user-role"); len(x) > 0 {
		r = x[0]
	}
	if u == "" {
		u = "anonymous"
	}
	return u, r
}
func (s *taskServer) ListTasks(ctx context.Context, _ *taskv1.ListTasksRequest) (*taskv1.ListTasksResponse, error) {
	u, r := actor(ctx)
	scope := u
	if r == "admin" {
		scope = "all"
	}
	if s.redis != nil {
		// Redis เป็น cache-aside optimization; cache miss/failure ยังอ่านจาก read DB ได้
		var cached []*taskv1.Task
		if b, e := s.redis.Get(ctx, "tasks:"+scope).Bytes(); e == nil && json.Unmarshal(b, &cached) == nil {
			return &taskv1.ListTasksResponse{Tasks: cached}, nil
		}
	}
	if s.reader != nil {
		var rows []task
		q := s.reader.WithContext(ctx)
		if r != "admin" {
			q = q.Where("owner_id = ?", u)
		}
		if e := q.Order("created_at desc").Find(&rows).Error; e != nil {
			return nil, status.Error(codes.Internal, e.Error())
		}
		out := make([]*taskv1.Task, 0, len(rows))
		for _, x := range rows {
			out = append(out, toProto(x))
		}
		if s.redis != nil {
			if b, e := json.Marshal(out); e == nil {
				_ = s.redis.Set(ctx, "tasks:"+scope, b, 60*time.Second).Err()
			}
		}
		return &taskv1.ListTasksResponse{Tasks: out}, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*taskv1.Task{}
	for _, x := range s.data {
		if r == "admin" || x.OwnerID == u {
			out = append(out, toProto(x))
		}
	}
	return &taskv1.ListTasksResponse{Tasks: out}, nil
}
func (s *taskServer) GetTask(ctx context.Context, req *taskv1.GetTaskRequest) (*taskv1.Task, error) {
	// Query ใช้ reader connection ตาม CQRS ส่วน policy ยังคงอยู่ใน use case เดียวกับการอ่านข้อมูล
	u, r := actor(ctx)
	var t task
	var ok bool
	if s.reader != nil {
		e := s.reader.WithContext(ctx).First(&t, "id = ?", req.ID).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "task not found")
		}
		if e != nil {
			return nil, status.Error(codes.Internal, e.Error())
		}
		ok = true
	} else {
		s.mu.RLock()
		t, ok = s.data[req.ID]
		s.mu.RUnlock()
	}
	if !ok {
		return nil, status.Error(codes.NotFound, "task not found")
	}
	if r != "admin" && t.OwnerID != u {
		return nil, status.Error(codes.PermissionDenied, "task ownership required")
	}
	return toProto(t), nil
}
func (s *taskServer) CreateTask(ctx context.Context, req *taskv1.CreateTaskRequest) (*taskv1.Task, error) {
	u, _ := actor(ctx)
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, status.Error(codes.InvalidArgument, "title is required")
	}
	now := time.Now().UTC()
	t := task{ID: uuid.NewString(), OwnerID: u, Title: title, Description: strings.TrimSpace(req.Description), Status: "todo", CreatedAt: now, UpdatedAt: now}
	event := outboxEvent{EventID: uuid.NewString(), EventType: "task.created", Task: t, OccurredAt: now}
	if s.writer != nil {
		b, _ := json.Marshal(event)
		// command และ outbox row commit ใน transaction เดียวกัน จึงไม่เกิด task สำเร็จแต่ event หาย
		e := s.writer.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if e := tx.Create(&t).Error; e != nil {
				return e
			}
			return tx.Create(&outboxRow{EventID: event.EventID, EventType: event.EventType, Payload: string(b), OccurredAt: event.OccurredAt}).Error
		})
		if e != nil {
			return nil, status.Error(codes.Internal, e.Error())
		}
	} else {
		s.mu.Lock()
		s.data[t.ID] = t
		s.mu.Unlock()
		s.enqueue(event)
	}
	s.invalidate(ctx, u)
	return toProto(t), nil
}
func (s *taskServer) UpdateTask(ctx context.Context, req *taskv1.UpdateTaskRequest) (*taskv1.Task, error) {
	// Mutation ใช้ writer และสร้าง outbox event ใน transaction เดียวกันเหมือน CreateTask
	// เพื่อไม่ให้สถานะ Task เปลี่ยนแต่ downstream analytics/activity ไม่ได้รับ event
	u, r := actor(ctx)
	if strings.TrimSpace(req.Title) == "" || !map[string]bool{"todo": true, "doing": true, "done": true}[req.Status] {
		return nil, status.Error(codes.InvalidArgument, "title and status are required")
	}
	var t task
	if s.writer != nil {
		if e := s.writer.WithContext(ctx).First(&t, "id = ?", req.ID).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return nil, status.Error(codes.NotFound, "task not found")
			}
			return nil, status.Error(codes.Internal, e.Error())
		}
	} else {
		s.mu.RLock()
		var ok bool
		t, ok = s.data[req.ID]
		s.mu.RUnlock()
		if !ok {
			return nil, status.Error(codes.NotFound, "task not found")
		}
	}
	if r != "admin" && t.OwnerID != u {
		return nil, status.Error(codes.PermissionDenied, "task ownership required")
	}
	t.Title = strings.TrimSpace(req.Title)
	t.Description = strings.TrimSpace(req.Description)
	t.Status = req.Status
	t.UpdatedAt = time.Now().UTC()
	event := outboxEvent{EventID: uuid.NewString(), EventType: "task.updated", Task: t, OccurredAt: t.UpdatedAt}
	if s.writer != nil {
		b, _ := json.Marshal(event)
		if e := s.writer.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if e := tx.Save(&t).Error; e != nil {
				return e
			}
			return tx.Create(&outboxRow{EventID: event.EventID, EventType: event.EventType, Payload: string(b), OccurredAt: event.OccurredAt}).Error
		}); e != nil {
			return nil, status.Error(codes.Internal, e.Error())
		}
	} else {
		s.mu.Lock()
		s.data[t.ID] = t
		s.mu.Unlock()
		s.enqueue(event)
	}
	s.invalidate(ctx, t.OwnerID)
	return toProto(t), nil
}
func (s *taskServer) DeleteTask(ctx context.Context, req *taskv1.DeleteTaskRequest) (*taskv1.Empty, error) {
	u, r := actor(ctx)
	var t task
	if s.writer != nil {
		if e := s.writer.WithContext(ctx).First(&t, "id = ?", req.ID).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return nil, status.Error(codes.NotFound, "task not found")
			}
			return nil, status.Error(codes.Internal, e.Error())
		}
	} else {
		s.mu.RLock()
		var ok bool
		t, ok = s.data[req.ID]
		s.mu.RUnlock()
		if !ok {
			return nil, status.Error(codes.NotFound, "task not found")
		}
	}
	if r != "admin" && t.OwnerID != u {
		return nil, status.Error(codes.PermissionDenied, "task ownership required")
	}
	event := outboxEvent{EventID: uuid.NewString(), EventType: "task.deleted", Task: t, OccurredAt: time.Now().UTC()}
	if s.writer != nil {
		b, _ := json.Marshal(event)
		if e := s.writer.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if e := tx.Delete(&task{}, "id = ?", req.ID).Error; e != nil {
				return e
			}
			return tx.Create(&outboxRow{EventID: event.EventID, EventType: event.EventType, Payload: string(b), OccurredAt: event.OccurredAt}).Error
		}); e != nil {
			return nil, status.Error(codes.Internal, e.Error())
		}
	} else {
		s.mu.Lock()
		delete(s.data, req.ID)
		s.mu.Unlock()
		s.enqueue(event)
	}
	s.invalidate(ctx, t.OwnerID)
	return &taskv1.Empty{}, nil
}
func (s *taskServer) invalidate(ctx context.Context, owner string) {
	// Cache invalidation เกิดหลัง transaction commit; ลบทั้ง owner scope และ admin scope
	// เพราะทั้งสอง key อาจมี Task เดียวกันอยู่
	if s.redis != nil {
		_ = s.redis.Del(ctx, "tasks:"+owner, "tasks:all").Err()
	}
	s.mu.Lock()
	s.cache = map[string][]task{}
	s.mu.Unlock()
}
func (s *taskServer) enqueue(e outboxEvent) {
	if s.outbox != nil {
		select {
		case s.outbox <- e:
		default:
			log.Printf("dev outbox queue full: %s", e.EventID)
		}
	}
}
func toProto(t task) *taskv1.Task {
	return &taskv1.Task{ID: t.ID, OwnerID: t.OwnerID, Title: t.Title, Description: t.Description, Status: t.Status, CreatedAt: t.CreatedAt.Format(time.RFC3339), UpdatedAt: t.UpdatedAt.Format(time.RFC3339)}
}
func (s *taskServer) publishOutbox(ctx context.Context, brokers string) {
	// Outbox worker แยก lifecycle จาก gRPC request: ผู้ใช้รอเพียง DB commit ส่วน Kafka retry ภายหลังได้
	var w *kafka.Writer
	if brokers != "" {
		w = &kafka.Writer{Addr: kafka.TCP(strings.Split(brokers, ",")...), Topic: "task.events.v1", Balancer: &kafka.LeastBytes{}}
		defer w.Close()
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if s.writer == nil {
				continue
			}
			// poll เฉพาะ outbox ที่ยังไม่ published และ mark หลัง Kafka ack เพื่อ retry ได้
			var rows []outboxRow
			if e := s.writer.WithContext(ctx).Where("published_at IS NULL").Order("occurred_at").Limit(20).Find(&rows).Error; e != nil {
				log.Printf("outbox poll: %v", e)
				continue
			}
			for _, row := range rows {
				if w == nil {
					continue
				}
				if e := w.WriteMessages(ctx, kafka.Message{Key: []byte(row.EventID), Value: []byte(row.Payload)}); e != nil {
					log.Printf("outbox publish retry event=%s: %v", row.EventID, e)
					continue
				}
				now := time.Now().UTC()
				_ = s.writer.WithContext(ctx).Model(&outboxRow{}).Where("event_id = ? AND published_at IS NULL", row.EventID).Update("published_at", now).Error
			}
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
	wd := env("TASK_DB_WRITE_DSN", "")
	rd := env("TASK_DB_READ_DSN", wd)
	// Local ใช้ PostgreSQL ตัวเดียวได้ แต่ DSN แยกทำให้ production เปลี่ยน reader เป็น read replica
	// โดยไม่แตะ business logic หรือ gRPC contract
	if wd == "" && !isDev() {
		log.Fatal("TASK_DB_WRITE_DSN is required")
	}
	var writer, reader *gorm.DB
	var e error
	if wd != "" {
		writer, e = openDB(ctx, wd)
		if e != nil {
			log.Fatal(e)
		}
		reader, e = openDB(ctx, rd)
		if e != nil {
			log.Fatal(e)
		}
		if e = writer.Exec(migrationSQL).Error; e != nil {
			log.Fatal(e)
		}
	}
	var rc *redis.Client
	if a := os.Getenv("REDIS_ADDR"); a != "" {
		rc = redis.NewClient(&redis.Options{Addr: a})
		if e = rc.Ping(ctx).Err(); e != nil {
			log.Fatal(e)
		}
	} else if !isDev() {
		log.Fatal("REDIS_ADDR is required")
	}
	lis, e := net.Listen("tcp", env("TASK_ADDR", ":50052"))
	if e != nil {
		log.Fatal(e)
	}
	s := &taskServer{data: map[string]task{}, cache: map[string][]task{}, outbox: make(chan outboxEvent, 128), writer: writer, reader: reader, redis: rc}
	go s.publishOutbox(ctx, os.Getenv("KAFKA_BROKERS"))
	g := grpc.NewServer(grpc.ForceServerCodec(gen.JSONCodec{}), grpc.UnaryInterceptor(deadlineInterceptor))
	taskv1.RegisterTaskServiceServer(g, s)
	go func() { <-ctx.Done(); g.GracefulStop() }()
	log.Printf("task-service listening on %s", lis.Addr())
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
func deadlineInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// ป้องกัน RPC ที่ไม่มี deadline ค้างและยึด connection/DB resource ไม่สิ้นสุด
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}
	return handler(ctx, req)
}
