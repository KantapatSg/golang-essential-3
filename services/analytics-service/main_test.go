package main

import (
	"context"
	analyticsv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/analytics/v1"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeStore struct {
	rows  [][]string
	query string
	err   error
}

func (f *fakeStore) Query(_ context.Context, sql string) ([][]string, error) {
	f.query = sql
	return f.rows, f.err
}

func TestRangeDefaultsAndLimit(t *testing.T) {
	from, to, limit, err := rangeSQL(&analyticsv1.TimeRange{}, 0)
	if err != nil || from == "" || to == "" || limit != 100 {
		t.Fatalf("unexpected defaults: %q %q %d %v", from, to, limit, err)
	}
}
func TestRangeRejectsUnboundedLimit(t *testing.T) {
	if _, _, _, err := rangeSQL(&analyticsv1.TimeRange{}, 501); err == nil {
		t.Fatal("expected limit validation")
	}
}
func TestAnalyticsMapsRowsIntoSummary(t *testing.T) {
	s := &analyticsServer{store: &fakeStore{rows: [][]string{{"task.created", "3"}, {"task.updated", "2"}, {"task.deleted", "1"}}}}
	out, err := s.Summary(context.Background(), &analyticsv1.SummaryRequest{})
	if err != nil || out.TotalEvents != 6 || out.Created != 3 || out.Updated != 2 || out.Deleted != 1 {
		t.Fatalf("unexpected summary %#v %v", out, err)
	}
}

func TestOrderSummaryUsesCanonicalConfirmationForRevenue(t *testing.T) {
	s := &analyticsServer{store: &fakeStore{rows: [][]string{{"OrderCreated", "1", "0"}, {"PaymentCompleted", "1", "1299"}, {"OrderConfirmed", "1", "1299"}, {"OrderRejected", "1", "0"}, {"OrderCancelled", "1", "0"}}}}
	out, err := s.OrderSummary(context.Background(), &analyticsv1.OrderSummaryRequest{})
	if err != nil || out.Created != 1 || out.Confirmed != 1 || out.Rejected != 1 || out.Cancelled != 1 || out.RevenueMinor != 1299 {
		t.Fatalf("unexpected canonical summary %#v %v", out, err)
	}
}

func TestAnalyticsQueriesUseClickHouseFinalForImmediateDeduplication(t *testing.T) {
	f := &fakeStore{rows: [][]string{{"task.created", "1"}}}
	s := &analyticsServer{store: f}
	if _, err := s.Summary(context.Background(), &analyticsv1.SummaryRequest{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.query, "task_events FINAL") {
		t.Fatalf("summary must use FINAL, query=%s", f.query)
	}
}
func TestAnalyticsMapsRowsIntoTimeseriesAndStatuses(t *testing.T) {
	f := &fakeStore{rows: [][]string{{"2026-01-01", "task.created", "4"}, {"2026-01-01", "task.done", "1"}}}
	s := &analyticsServer{store: f}
	out, err := s.Timeseries(context.Background(), &analyticsv1.TimeseriesRequest{})
	if err != nil || len(out.Points) != 1 || out.Points[0].Created != 4 {
		t.Fatalf("unexpected timeseries %#v %v", out, err)
	}
	f.rows = [][]string{{"done", "5"}, {"todo", "2"}}
	statusOut, err := s.Statuses(context.Background(), &analyticsv1.StatusesRequest{})
	if err != nil || len(statusOut.Statuses) != 2 || statusOut.Statuses[0].Count != 5 {
		t.Fatalf("unexpected statuses %#v %v", statusOut, err)
	}
}
func TestClickHouseStoreParsesJSONEachRow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"event_type":"task.created","count()":7}` + "\n"))
	}))
	defer server.Close()
	store := &clickhouseStore{endpoint: server.URL, client: server.Client()}
	rows, err := store.Query(context.Background(), "SELECT event_type,count()")
	if err != nil || len(rows) != 1 || rows[0][0] != "task.created" || rows[0][1] != "7" {
		t.Fatalf("unexpected rows %#v %v", rows, err)
	}
}
