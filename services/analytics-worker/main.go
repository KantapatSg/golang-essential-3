package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/segmentio/kafka-go"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"time"
)

type taskEvent struct {
	SchemaVersion int    `json:"schema_version"`
	EventID       string `json:"event_id"`
	EventType     string `json:"event_type"`
	Task          struct {
		ID      string `json:"ID"`
		OwnerID string `json:"OwnerID"`
		Status  string `json:"Status"`
	} `json:"Task"`
	OccurredAt time.Time `json:"occurred_at"`
}

// Task Service's native JSON payloads historically used Go field names while
// the public event contract uses snake_case. Accept both during the v1
// transition so replaying an existing outbox never drops an event.
func (e *taskEvent) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	read := func(keys ...string) json.RawMessage {
		for _, key := range keys {
			if value, ok := raw[key]; ok {
				return value
			}
		}
		return nil
	}
	if value := read("schema_version", "SchemaVersion"); len(value) > 0 {
		_ = json.Unmarshal(value, &e.SchemaVersion)
	}
	if value := read("event_id", "EventID"); len(value) > 0 {
		_ = json.Unmarshal(value, &e.EventID)
	}
	if value := read("event_type", "EventType"); len(value) > 0 {
		_ = json.Unmarshal(value, &e.EventType)
	}
	if value := read("occurred_at", "OccurredAt"); len(value) > 0 {
		_ = json.Unmarshal(value, &e.OccurredAt)
	}
	var taskRaw map[string]json.RawMessage
	if value := read("task", "Task"); len(value) > 0 {
		if err := json.Unmarshal(value, &taskRaw); err != nil {
			return err
		}
	}
	for key, target := range map[string]*string{"ID": &e.Task.ID, "OwnerID": &e.Task.OwnerID, "Status": &e.Task.Status} {
		if value, ok := taskRaw[key]; ok {
			_ = json.Unmarshal(value, target)
		} else if value, ok := taskRaw[strings.ToLower(key)]; ok {
			_ = json.Unmarshal(value, target)
		} else if key == "OwnerID" {
			if value, ok := taskRaw["owner_id"]; ok {
				_ = json.Unmarshal(value, target)
			}
		}
	}
	return nil
}

type worker struct {
	endpoint          string
	http              *http.Client
	processed         atomic.Uint64
	retries           atomic.Uint64
	dlq               atomic.Uint64
	lastEventUnixNano atomic.Int64
	consumerLag       atomic.Int64
}

func (w *worker) validate(b []byte) (taskEvent, error) {
	var e taskEvent
	if err := json.Unmarshal(b, &e); err != nil {
		return e, fmt.Errorf("invalid JSON: %w", err)
	}
	if e.SchemaVersion > 1 {
		return e, fmt.Errorf("unsupported event schema version %d", e.SchemaVersion)
	}
	if e.EventID == "" || e.EventType == "" || e.Task.ID == "" {
		return e, errors.New("event_id, event_type and task.id are required")
	}
	if e.EventType != "task.created" && e.EventType != "task.updated" && e.EventType != "task.deleted" {
		return e, fmt.Errorf("unsupported event type %q", e.EventType)
	}
	return e, nil
}
func (w *worker) insert(ctx context.Context, events []taskEvent) error {
	if len(events) == 0 {
		return nil
	}
	if w.endpoint == "" && strings.EqualFold(os.Getenv("DEV_MODE"), "true") {
		w.processed.Add(uint64(len(events)))
		return nil
	}
	if w.endpoint == "" {
		return errors.New("CLICKHOUSE_URL is required")
	}
	var body strings.Builder
	for _, e := range events {
		// ClickHouse DateTime64 รับรูปแบบ datetime แบบไม่มี RFC3339 suffix; แปลงตรงนี้
		// เป็น boundary เดียวก่อนส่ง batch เพื่อไม่ให้ event ที่ valid ถูก reject ทั้งชุด
		occurredAt := e.OccurredAt.UTC().Format("2006-01-02 15:04:05.000")
		b, _ := json.Marshal(map[string]any{"event_id": e.EventID, "event_type": e.EventType, "task_id": e.Task.ID, "actor_id": e.Task.OwnerID, "task_status": e.Task.Status, "occurred_at": occurredAt, "payload_json": string(mustJSON(e))})
		body.Write(b)
		body.WriteByte('\n')
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.endpoint+"?query=INSERT%20INTO%20analytics.task_events%20FORMAT%20JSONEachRow", strings.NewReader(body.String()))
	if err != nil {
		return err
	}
	resp, err := w.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("clickhouse status %s", resp.Status)
	}
	w.processed.Add(uint64(len(events)))
	for _, event := range events {
		if event.OccurredAt.UnixNano() > w.lastEventUnixNano.Load() {
			w.lastEventUnixNano.Store(event.OccurredAt.UnixNano())
		}
	}
	return nil
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func (w *worker) retryInsert(ctx context.Context, events []taskEvent) error {
	var err error
	for i := 0; i < 4; i++ {
		if err = w.insert(ctx, events); err == nil {
			return nil
		}
		if i < 3 {
			w.retries.Add(1)
			wait := time.Duration(1<<i) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}
	}
	return err
}
func (w *worker) run(ctx context.Context, brokers string) error {
	if strings.TrimSpace(brokers) == "" {
		<-ctx.Done()
		return nil
	}
	reader := kafka.NewReader(kafka.ReaderConfig{Brokers: strings.Split(brokers, ","), Topic: "task.events.v1", GroupID: "analytics-service-v1", MinBytes: 1, MaxBytes: 10 << 20})
	defer reader.Close()
	dlq := &kafka.Writer{Addr: kafka.TCP(strings.Split(brokers, ",")...), Topic: "task.events.v1.dlq", Balancer: &kafka.LeastBytes{}}
	defer dlq.Close()
	batch := make([]kafka.Message, 0, 50)
	flush := time.NewTicker(2 * time.Second)
	defer flush.Stop()
	type fetched struct {
		message kafka.Message
		err     error
	}
	fetchedCh := make(chan fetched, 1)
	fetchCtx, fetchCancel := context.WithCancel(ctx)
	defer fetchCancel()
	go func() {
		for {
			message, err := reader.FetchMessage(fetchCtx)
			select {
			case fetchedCh <- fetched{message: message, err: err}:
			case <-fetchCtx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return w.flush(shutdownCtx, reader, batch)
		case <-flush.C:
			if err := w.flush(ctx, reader, batch); err != nil {
				return err
			}
			batch = batch[:0]
		case result := <-fetchedCh:
			m, err := result.message, result.err
			w.consumerLag.Store(int64(reader.Stats().Lag))
			if err != nil {
				if errors.Is(err, context.Canceled) && ctx.Err() != nil {
					shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					return w.flush(shutdownCtx, reader, batch)
				}
				return err
			}
			e, err := w.validate(m.Value)
			if err != nil {
				if de := dlq.WriteMessages(ctx, kafka.Message{Key: m.Key, Value: append(m.Value, []byte("\nreason="+err.Error())...)}); de != nil {
					return de
				}
				w.dlq.Add(1)
				if err = reader.CommitMessages(ctx, m); err != nil {
					return err
				}
				continue
			}
			batch = append(batch, m)
			if len(batch) >= 50 {
				if err = w.flush(ctx, reader, batch); err != nil {
					return err
				}
				batch = batch[:0]
			}
			_ = e
		}
	}
}
func (w *worker) flush(ctx context.Context, reader *kafka.Reader, batch []kafka.Message) error {
	if len(batch) == 0 {
		return nil
	}
	events := make([]taskEvent, 0, len(batch))
	for _, m := range batch {
		e, err := w.validate(m.Value)
		if err != nil {
			return err
		}
		events = append(events, e)
	}
	if err := w.retryInsert(ctx, events); err != nil {
		return err
	} // commit only after ClickHouse confirms the whole batch
	return reader.CommitMessages(ctx, batch...)
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	w := &worker{endpoint: os.Getenv("CLICKHOUSE_URL"), http: &http.Client{Timeout: 5 * time.Second}}
	brokers := strings.TrimSpace(os.Getenv("KAFKA_BROKERS"))
	ready := func(checkCtx context.Context) error {
		if w.endpoint == "" {
			if strings.EqualFold(os.Getenv("DEV_MODE"), "true") {
				return nil
			}
			return errors.New("CLICKHOUSE_URL is required")
		}
		req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, strings.TrimRight(w.endpoint, "/")+"/ping", nil)
		if err != nil {
			return err
		}
		resp, err := w.http.Do(req)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 300 {
			return fmt.Errorf("clickhouse status %s", resp.Status)
		}
		if brokers == "" {
			return nil
		}
		conn, err := (&kafka.Dialer{Timeout: 500 * time.Millisecond}).DialContext(checkCtx, "tcp", strings.TrimSpace(strings.Split(brokers, ",")[0]))
		if err != nil {
			return err
		}
		return conn.Close()
	}
	health := &http.Server{Addr: env("ANALYTICS_WORKER_HTTP_ADDR", ":9105"), Handler: workerMetrics(w, ready)}
	go func() {
		if err := health.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("worker health: %v", err)
		}
	}()
	if err := w.run(ctx, os.Getenv("KAFKA_BROKERS")); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("analytics worker stopped: %v", err)
	}
	shutdownCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	_ = health.Shutdown(shutdownCtx)
	log.Printf("analytics worker processed=%d retries=%d dlq=%d", w.processed.Load(), w.retries.Load(), w.dlq.Load())
}
func workerMetrics(w *worker, ready func(context.Context) error) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health/live":
			rw.WriteHeader(http.StatusOK)
			_, _ = rw.Write([]byte(`{"status":"ok"}`))
		case "/health/ready":
			checkCtx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
			defer cancel()
			if err := ready(checkCtx); err != nil {
				rw.WriteHeader(http.StatusServiceUnavailable)
				_, _ = rw.Write([]byte(`{"status":"not_ready"}`))
				return
			}
			rw.WriteHeader(http.StatusOK)
			_, _ = rw.Write([]byte(`{"status":"ready"}`))
		case "/metrics":
			rw.Header().Set("Content-Type", "text/plain; version=0.0.4")
			delay := 0.0
			if eventAt := w.lastEventUnixNano.Load(); eventAt > 0 {
				delay = time.Since(time.Unix(0, eventAt)).Seconds()
				if delay < 0 {
					delay = 0
				}
			}
			_, _ = fmt.Fprintf(rw, "# HELP service_ready Whether the analytics worker dependencies are reachable.\n# TYPE service_ready gauge\nservice_ready 1\n# HELP analytics_processed_events_total Events written to ClickHouse.\n# TYPE analytics_processed_events_total counter\nanalytics_processed_events_total %d\n# HELP analytics_retry_total ClickHouse retry attempts.\n# TYPE analytics_retry_total counter\nanalytics_retry_total %d\n# HELP analytics_dlq_total Invalid events routed to DLQ.\n# TYPE analytics_dlq_total counter\nanalytics_dlq_total %d\n# HELP analytics_ingestion_delay_seconds Age of the newest processed event.\n# TYPE analytics_ingestion_delay_seconds gauge\nanalytics_ingestion_delay_seconds %f\n# HELP kafka_consumer_lag Messages behind the end of the Kafka partition.\n# TYPE kafka_consumer_lag gauge\nkafka_consumer_lag{consumer=\"analytics\"} %d\n", w.processed.Load(), w.retries.Load(), w.dlq.Load(), delay, w.consumerLag.Load())
		default:
			http.NotFound(rw, r)
		}
	})
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
