package main

import (
	"context"
	"errors"
	"fmt"
	analyticsv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/analytics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
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
	return nil, nil
}

type analyticsServer struct {
	analyticsv1.UnimplementedAnalyticsServiceServer
	store    eventStore
	requests atomic.Uint64
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
	s.requests.Add(1)
	from, to, _, err := rangeSQL(req.Range, req.Limit)
	if err != nil {
		return nil, err
	}
	if s.store != nil {
		_, err = s.store.Query(ctx, fmt.Sprintf("SELECT event_type,count() FROM analytics.task_events WHERE occurred_at >= parseDateTimeBestEffort('%s') AND occurred_at < parseDateTimeBestEffort('%s') GROUP BY event_type", from, to))
		if err != nil {
			return nil, status.Error(codes.Unavailable, "analytics store unavailable")
		}
	}
	return &analyticsv1.SummaryResponse{TotalEvents: 0, GeneratedAt: time.Now().UTC().Format(time.RFC3339), DataThrough: to}, nil
}
func (s *analyticsServer) Timeseries(ctx context.Context, req *analyticsv1.TimeseriesRequest) (*analyticsv1.TimeseriesResponse, error) {
	from, to, limit, err := rangeSQL(req.Range, req.Limit)
	if err != nil {
		return nil, err
	}
	if s.store != nil {
		_, err = s.store.Query(ctx, fmt.Sprintf("SELECT toDate(occurred_at),event_type,count() FROM analytics.task_events WHERE occurred_at >= parseDateTimeBestEffort('%s') AND occurred_at < parseDateTimeBestEffort('%s') GROUP BY toDate(occurred_at),event_type ORDER BY toDate(occurred_at) LIMIT %d", from, to, limit))
		if err != nil {
			return nil, status.Error(codes.Unavailable, "analytics store unavailable")
		}
	}
	return &analyticsv1.TimeseriesResponse{Points: []*analyticsv1.TimeseriesPoint{}, GeneratedAt: time.Now().UTC().Format(time.RFC3339), DataThrough: to}, nil
}
func (s *analyticsServer) Statuses(ctx context.Context, req *analyticsv1.StatusesRequest) (*analyticsv1.StatusesResponse, error) {
	from, to, limit, err := rangeSQL(req.Range, req.Limit)
	if err != nil {
		return nil, err
	}
	if s.store != nil {
		_, err = s.store.Query(ctx, fmt.Sprintf("SELECT task_status,count() FROM analytics.task_events WHERE occurred_at >= parseDateTimeBestEffort('%s') AND occurred_at < parseDateTimeBestEffort('%s') GROUP BY task_status ORDER BY count() DESC LIMIT %d", from, to, limit))
		if err != nil {
			return nil, status.Error(codes.Unavailable, "analytics store unavailable")
		}
	}
	return &analyticsv1.StatusesResponse{Statuses: []*analyticsv1.StatusCount{}, GeneratedAt: time.Now().UTC().Format(time.RFC3339), DataThrough: to}, nil
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
	health := &http.Server{Addr: env("ANALYTICS_HTTP_ADDR", ":9104"), Handler: metricsHandler(srv)}
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
func metricsHandler(s *analyticsServer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health/live":
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/health/ready":
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"status":"ready"}`))
		case "/metrics":
			_, _ = fmt.Fprintf(w, "analytics_grpc_requests_total %d\n", s.requests.Load())
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
