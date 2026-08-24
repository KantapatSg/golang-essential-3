package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	analyticsv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/analytics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type eventStore interface {
	Query(ctx context.Context, sql string) ([][]string, error)
}
type clickhouseStore struct {
	endpoint string
	client   *http.Client
}

func (s *clickhouseStore) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(s.endpoint, "/")+"/ping", nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("clickhouse status %s", resp.Status)
	}
	return nil
}

func (s *clickhouseStore) Query(ctx context.Context, sql string) ([][]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, strings.NewReader(sql+" FORMAT JSONEachRow"))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("clickhouse status %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// JSONEachRow keeps the transport simple while preserving the bounded query
	// contract; rows are normalized into the small shape consumed by each RPC.
	var rows [][]string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &object); err != nil {
			return nil, fmt.Errorf("decode clickhouse row: %w", err)
		}
		keys := []string{"event_type", "count()"}
		if strings.Contains(sql, "task_status") {
			keys = []string{"task_status", "count()"}
		}
		if strings.Contains(sql, "toDate(occurred_at)") {
			keys = []string{"toDate(occurred_at)", "event_type", "count()"}
		}
		row := make([]string, 0, len(keys))
		for _, key := range keys {
			raw, ok := object[key]
			if !ok {
				raw = object[strings.TrimSuffix(key, "()")]
				if !ok && key == "count()" {
					raw = object["count"]
				}
			}
			if len(raw) == 0 {
				return nil, fmt.Errorf("clickhouse row missing %s", key)
			}
			var value string
			if raw[0] == '"' {
				if err := json.Unmarshal(raw, &value); err != nil {
					return nil, err
				}
			} else {
				value = string(raw)
			}
			row = append(row, value)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

type analyticsServer struct {
	analyticsv1.UnimplementedAnalyticsServiceServer
	analyticsv1.UnimplementedOrderAnalyticsServiceServer
	store              eventStore
	requests           atomic.Uint64
	queryErrors        atomic.Uint64
	queryDurationNanos atomic.Uint64
}

func (s *analyticsServer) OrderSummary(ctx context.Context, req *analyticsv1.OrderSummaryRequest) (*analyticsv1.OrderSummaryResponse, error) {
	if req == nil {
		req = &analyticsv1.OrderSummaryRequest{}
	}
	from, to, _, err := rangeSQL(&analyticsv1.TimeRange{From: req.GetFrom(), To: req.GetTo()}, 100)
	if err != nil {
		return nil, err
	}
	out := &analyticsv1.OrderSummaryResponse{Currency: "USD", Through: to}
	if s.store == nil {
		return out, nil
	}
	rows, e := s.store.Query(ctx, fmt.Sprintf("SELECT event_type,count() FROM analytics.order_events FINAL WHERE occurred_at >= parseDateTimeBestEffort('%s') AND occurred_at < parseDateTimeBestEffort('%s') GROUP BY event_type", from, to))
	if e != nil {
		return nil, status.Error(codes.Unavailable, "analytics store unavailable")
	}
	for _, r := range rows {
		if len(r) != 2 {
			continue
		}
		n, _ := strconv.ParseInt(r[1], 10, 64)
		switch r[0] {
		case "OrderCreated":
			out.Created += n
		case "OrderConfirmed", "PaymentCompleted":
			out.Confirmed += n
		case "InventoryRejected":
			out.Rejected += n
		case "OrderCancelled", "PaymentFailed":
			out.Cancelled += n
		}
	}
	return out, nil
}
func (s *analyticsServer) OrderFunnel(ctx context.Context, req *analyticsv1.OrderSummaryRequest) (*analyticsv1.OrderFunnelResponse, error) {
	if req == nil {
		req = &analyticsv1.OrderSummaryRequest{}
	}
	from, to, _, err := rangeSQL(&analyticsv1.TimeRange{From: req.GetFrom(), To: req.GetTo()}, 100)
	if err != nil {
		return nil, err
	}
	out := &analyticsv1.OrderFunnelResponse{Through: to}
	if s.store == nil {
		return out, nil
	}
	rows, e := s.store.Query(ctx, fmt.Sprintf("SELECT event_type,count() FROM analytics.order_events FINAL WHERE occurred_at >= parseDateTimeBestEffort('%s') AND occurred_at < parseDateTimeBestEffort('%s') GROUP BY event_type", from, to))
	if e != nil {
		return nil, status.Error(codes.Unavailable, "analytics store unavailable")
	}
	for _, r := range rows {
		if len(r) != 2 {
			continue
		}
		n, _ := strconv.ParseInt(r[1], 10, 64)
		switch r[0] {
		case "OrderCreated":
			out.Created += n
		case "InventoryReserved":
			out.Reserved += n
		case "PaymentCompleted":
			out.Paid += n
		case "OrderConfirmed":
			out.Confirmed += n
		case "InventoryRejected":
			out.Rejected += n
		case "OrderCancelled":
			out.Cancelled += n
		}
	}
	return out, nil
}

func (s *analyticsServer) observe(start time.Time, err error) {
	s.queryDurationNanos.Add(uint64(time.Since(start).Nanoseconds()))
	if err != nil {
		s.queryErrors.Add(1)
	}
}

func rangeSQL(r *analyticsv1.TimeRange, limit uint32) (string, string, uint32, error) {
	if r == nil {
		r = &analyticsv1.TimeRange{}
	}
	from, to := r.From, r.To
	if from == "" {
		from = time.Now().UTC().Add(-7 * 24 * time.Hour).Format(time.RFC3339)
	}
	if to == "" {
		to = time.Now().UTC().Format(time.RFC3339)
	}
	if _, err := time.Parse(time.RFC3339, from); err != nil {
		return "", "", 0, status.Error(codes.InvalidArgument, "from must be RFC3339")
	}
	if _, err := time.Parse(time.RFC3339, to); err != nil {
		return "", "", 0, status.Error(codes.InvalidArgument, "to must be RFC3339")
	}
	if limit == 0 {
		limit = 100
	}
	if limit > 500 {
		return "", "", 0, status.Error(codes.InvalidArgument, "limit must be between 1 and 500")
	}
	return from, to, limit, nil
}
func (s *analyticsServer) Summary(ctx context.Context, req *analyticsv1.SummaryRequest) (*analyticsv1.SummaryResponse, error) {
	started := time.Now()
	s.requests.Add(1)
	if req == nil {
		req = &analyticsv1.SummaryRequest{}
	}
	from, to, _, err := rangeSQL(req.Range, req.Limit)
	if err != nil {
		return nil, err
	}
	var response analyticsv1.SummaryResponse
	if s.store != nil {
		rows, queryErr := s.store.Query(ctx, fmt.Sprintf("SELECT event_type,count() FROM analytics.task_events FINAL WHERE occurred_at >= parseDateTimeBestEffort('%s') AND occurred_at < parseDateTimeBestEffort('%s') GROUP BY event_type", from, to))
		err = queryErr
		if err != nil {
			s.observe(started, err)
			return nil, status.Error(codes.Unavailable, "analytics store unavailable")
		}
		for _, row := range rows {
			if len(row) != 2 {
				continue
			}
			n, parseErr := strconv.ParseUint(row[1], 10, 64)
			if parseErr != nil {
				return nil, status.Error(codes.Internal, "invalid analytics count")
			}
			response.TotalEvents += n
			switch row[0] {
			case "task.created":
				response.Created += n
			case "task.updated":
				response.Updated += n
			case "task.deleted":
				response.Deleted += n
			}
		}
	}
	response.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	response.DataThrough = to
	s.observe(started, nil)
	return &response, nil
}
func (s *analyticsServer) Timeseries(ctx context.Context, req *analyticsv1.TimeseriesRequest) (*analyticsv1.TimeseriesResponse, error) {
	started := time.Now()
	if req == nil {
		req = &analyticsv1.TimeseriesRequest{}
	}
	from, to, limit, err := rangeSQL(req.Range, req.Limit)
	if err != nil {
		return nil, err
	}
	points := map[string]*analyticsv1.TimeseriesPoint{}
	if s.store != nil {
		rows, queryErr := s.store.Query(ctx, fmt.Sprintf("SELECT toDate(occurred_at),event_type,count() FROM analytics.task_events FINAL WHERE occurred_at >= parseDateTimeBestEffort('%s') AND occurred_at < parseDateTimeBestEffort('%s') GROUP BY toDate(occurred_at),event_type ORDER BY toDate(occurred_at) LIMIT %d", from, to, limit))
		err = queryErr
		if err != nil {
			s.observe(started, err)
			return nil, status.Error(codes.Unavailable, "analytics store unavailable")
		}
		for _, row := range rows {
			if len(row) != 3 {
				continue
			}
			n, parseErr := strconv.ParseUint(row[2], 10, 64)
			if parseErr != nil {
				return nil, status.Error(codes.Internal, "invalid analytics count")
			}
			p := points[row[0]]
			if p == nil {
				p = &analyticsv1.TimeseriesPoint{Day: row[0]}
				points[row[0]] = p
			}
			switch row[1] {
			case "task.created":
				p.Created += n
			case "task.updated":
				p.Updated += n
			case "task.deleted":
				p.Deleted += n
			}
		}
	}
	ordered := make([]*analyticsv1.TimeseriesPoint, 0, len(points))
	for _, p := range points {
		ordered = append(ordered, p)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Day < ordered[j].Day })
	s.observe(started, nil)
	return &analyticsv1.TimeseriesResponse{Points: ordered, GeneratedAt: time.Now().UTC().Format(time.RFC3339), DataThrough: to}, nil
}
func (s *analyticsServer) Statuses(ctx context.Context, req *analyticsv1.StatusesRequest) (*analyticsv1.StatusesResponse, error) {
	started := time.Now()
	if req == nil {
		req = &analyticsv1.StatusesRequest{}
	}
	from, to, limit, err := rangeSQL(req.Range, req.Limit)
	if err != nil {
		return nil, err
	}
	statuses := make([]*analyticsv1.StatusCount, 0)
	if s.store != nil {
		rows, queryErr := s.store.Query(ctx, fmt.Sprintf("SELECT task_status,count() FROM analytics.task_events FINAL WHERE occurred_at >= parseDateTimeBestEffort('%s') AND occurred_at < parseDateTimeBestEffort('%s') GROUP BY task_status ORDER BY count() DESC LIMIT %d", from, to, limit))
		err = queryErr
		if err != nil {
			s.observe(started, err)
			return nil, status.Error(codes.Unavailable, "analytics store unavailable")
		}
		for _, row := range rows {
			if len(row) != 2 {
				continue
			}
			n, parseErr := strconv.ParseUint(row[1], 10, 64)
			if parseErr != nil {
				return nil, status.Error(codes.Internal, "invalid analytics count")
			}
			statuses = append(statuses, &analyticsv1.StatusCount{Status: row[0], Count: n})
		}
	}
	s.observe(started, nil)
	return &analyticsv1.StatusesResponse{Statuses: statuses, GeneratedAt: time.Now().UTC().Format(time.RFC3339), DataThrough: to}, nil
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	var store eventStore
	if endpoint := os.Getenv("CLICKHOUSE_URL"); endpoint != "" {
		store = &clickhouseStore{endpoint: endpoint, client: &http.Client{Timeout: 4 * time.Second}}
	}
	srv := &analyticsServer{store: store}
	lis, err := net.Listen("tcp", env("ANALYTICS_ADDR", ":50054"))
	if err != nil {
		log.Fatal(err)
	}
	g := grpc.NewServer()
	analyticsv1.RegisterAnalyticsServiceServer(g, srv)
	analyticsv1.RegisterOrderAnalyticsServiceServer(g, srv)
	ready := func(checkCtx context.Context) error { return nil }
	if ch, ok := store.(*clickhouseStore); ok {
		ready = ch.Ping
	}
	health := &http.Server{Addr: env("ANALYTICS_HTTP_ADDR", ":9104"), Handler: metricsHandler(srv, ready)}
	go func() {
		if err := health.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("analytics health: %v", err)
		}
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = health.Shutdown(shutdownCtx)
		g.GracefulStop()
	}()
	log.Printf("analytics-service listening on %s", lis.Addr())
	if err := g.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		log.Fatal(err)
	}
}
func metricsHandler(s *analyticsServer, ready func(context.Context) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health/live":
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/health/ready":
			checkCtx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
			defer cancel()
			if err := ready(checkCtx); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"status":"not_ready"}`))
				return
			}
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"status":"ready"}`))
		case "/metrics":
			_, _ = fmt.Fprintf(w, "# HELP service_ready Whether the analytics service can accept traffic.\n# TYPE service_ready gauge\nservice_ready 1\n# HELP analytics_grpc_requests_total Analytics RPC requests.\n# TYPE analytics_grpc_requests_total counter\nanalytics_grpc_requests_total %d\n# HELP analytics_query_errors_total Analytics query failures.\n# TYPE analytics_query_errors_total counter\nanalytics_query_errors_total %d\n# HELP analytics_query_duration_seconds_total Total analytics query duration.\n# TYPE analytics_query_duration_seconds_total counter\nanalytics_query_duration_seconds_total %f\n", s.requests.Load(), s.queryErrors.Load(), float64(s.queryDurationNanos.Load())/1e9)
		default:
			http.NotFound(w, r)
		}
	})
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
