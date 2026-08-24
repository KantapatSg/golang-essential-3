package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/KantapatSg/golang-essential-3/contracts"
	inventoryv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/inventory/v1"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type product struct {
	id, name, currency string
	price              int64
	available          int32
}
type inventoryServer struct {
	inventoryv1.UnimplementedInventoryServiceServer
	mu           sync.RWMutex
	products     map[string]product
	reservations map[string]reservation
	processed    map[string]bool
	stockKeys    map[string]stockAdjustment
	db           *gorm.DB
	redis        *redis.Client
}
type stockAdjustment struct {
	hash string
	out  *inventoryv1.AdjustStockResponse
}

const catalogCacheKey = "catalog:v1:list:all"

var cacheHits, cacheMisses, cacheBypasses, cacheErrors, cacheInvalidations atomic.Uint64
var inventoryTransactions, inventoryTransactionErrors, inventoryAdjustments atomic.Uint64

type reservation struct {
	ID, OrderID, ProductID, Status, Reason string
	Quantity                               int32
	CreatedAt                              time.Time
}
type inventoryProductRow struct {
	ID, Name, Currency string `gorm:"primaryKey"`
	UnitPriceMinor     int64
	OnHand             int32
	Reserved           int32
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
type stockMovementRow struct {
	ID, ProductID, Reason, IdempotencyKey string
	RequestHash                           string
	Delta, BalanceAfter                   int32
	CreatedAt                             time.Time
}
type inventoryReservationRow struct {
	ID, OrderID, ProductID, Status, Reason string
	Quantity                               int32
	CreatedAt                              time.Time
}
type inventoryProcessedEventRow struct {
	EventID, EventType string
	ProcessedAt        time.Time
}
type inventoryOutboxRow struct {
	EventID, EventType, AggregateID, Payload string
	OccurredAt                               time.Time
	PublishedAt                              *time.Time
}

func (inventoryProductRow) TableName() string        { return "inventory_products" }
func (stockMovementRow) TableName() string           { return "inventory_stock_movements" }
func (inventoryReservationRow) TableName() string    { return "inventory_reservations" }
func (inventoryProcessedEventRow) TableName() string { return "inventory_processed_events" }
func (inventoryOutboxRow) TableName() string         { return "inventory_outbox" }

type createdPayload struct {
	Order struct {
		ID              string `json:"id"`
		CustomerID      string `json:"customer_id"`
		TotalMinor      int64  `json:"total_minor"`
		Currency        string `json:"currency"`
		PaymentScenario string `json:"payment_scenario"`
		Items           []struct {
			ProductID string `json:"product_id"`
			Quantity  int32  `json:"quantity"`
		} `json:"items"`
	} `json:"order"`
}
type compensationPayload struct {
	OrderID         string `json:"order_id"`
	CustomerID      string `json:"customer_id"`
	Reason          string `json:"reason"`
	AmountMinor     int64  `json:"amount_minor,omitempty"`
	Currency        string `json:"currency,omitempty"`
	PaymentScenario string `json:"payment_scenario,omitempty"`
}

func (s *inventoryServer) ListProducts(ctx context.Context, req *inventoryv1.ListProductsRequest) (*inventoryv1.ListProductsResponse, error) {
	cacheStatus := "BYPASS"
	if s.redis != nil {
		if b, err := s.redis.Get(ctx, catalogCacheKey).Bytes(); err == nil {
			var cached inventoryv1.ListProductsResponse
			if json.Unmarshal(b, &cached) == nil {
				cacheHits.Add(1)
				cacheStatus = "HIT"
				_ = grpc.SetHeader(ctx, metadata.Pairs("x-cache-status", cacheStatus))
				return &cached, nil
			}
			cacheErrors.Add(1)
		} else if errors.Is(err, redis.Nil) {
			cacheMisses.Add(1)
		} else {
			cacheErrors.Add(1)
		}
	} else {
		cacheBypasses.Add(1)
	}
	var out *inventoryv1.ListProductsResponse
	var err error
	if s.db != nil {
		out, err = s.listInventoryDB(req)
	} else {
		out, err = s.listProductsMemory(req)
	}
	if err == nil && s.redis != nil {
		if b, marshalErr := json.Marshal(out); marshalErr == nil && s.redis.Set(ctx, catalogCacheKey, b, 60*time.Second).Err() == nil {
			cacheStatus = "MISS"
		} else {
			cacheErrors.Add(1)
			cacheStatus = "BYPASS"
		}
	}
	_ = grpc.SetHeader(ctx, metadata.Pairs("x-cache-status", cacheStatus))
	return out, err
}
func (s *inventoryServer) listProductsMemory(req *inventoryv1.ListProductsRequest) (*inventoryv1.ListProductsResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	page, size := int(req.GetPage()), int(req.GetPageSize())
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	all := make([]product, 0, len(s.products))
	for _, p := range s.products {
		all = append(all, p)
	}
	start := (page - 1) * size
	if start > len(all) {
		start = len(all)
	}
	end := start + size
	if end > len(all) {
		end = len(all)
	}
	out := make([]*inventoryv1.Product, 0, end-start)
	for _, p := range all[start:end] {
		out = append(out, toProto(p))
	}
	return &inventoryv1.ListProductsResponse{Products: out, Total: int32(len(all))}, nil
}
func (s *inventoryServer) listInventoryDB(req *inventoryv1.ListProductsRequest) (*inventoryv1.ListProductsResponse, error) {
	page, size := int(req.GetPage()), int(req.GetPageSize())
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	var rows []inventoryProductRow
	q := s.db.Order("id").Offset((page - 1) * size).Limit(size).Find(&rows)
	if q.Error != nil {
		return nil, status.Error(codes.Internal, q.Error.Error())
	}
	var total int64
	if err := s.db.Model(&inventoryProductRow{}).Count(&total).Error; err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	out := make([]*inventoryv1.Product, 0, len(rows))
	for _, row := range rows {
		out = append(out, dbProductProto(row))
	}
	return &inventoryv1.ListProductsResponse{Products: out, Total: int32(total)}, nil
}
func (s *inventoryServer) QuoteProducts(_ context.Context, req *inventoryv1.QuoteProductsRequest) (*inventoryv1.QuoteProductsResponse, error) {
	if req == nil || len(req.GetItems()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one item is required")
	}
	if s.db != nil {
		return s.quoteProductsDB(req)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*inventoryv1.Product, 0, len(req.GetItems()))
	var total int64
	currency := "USD"
	for _, item := range req.GetItems() {
		if item.GetQuantity() < 1 || item.GetQuantity() > 10000 {
			return nil, status.Error(codes.InvalidArgument, "quantity must be between 1 and 10000")
		}
		p, ok := s.products[item.GetProductId()]
		if !ok {
			return nil, status.Error(codes.NotFound, "product not found")
		}
		currency = p.currency
		total += p.price * int64(item.GetQuantity())
		out = append(out, toProto(p))
	}
	return &inventoryv1.QuoteProductsResponse{Products: out, TotalMinor: total, Currency: currency}, nil
}
func (s *inventoryServer) quoteProductsDB(req *inventoryv1.QuoteProductsRequest) (*inventoryv1.QuoteProductsResponse, error) {
	out := make([]*inventoryv1.Product, 0, len(req.GetItems()))
	var total int64
	currency := "USD"
	for _, item := range req.GetItems() {
		if item.GetQuantity() < 1 || item.GetQuantity() > 10000 {
			return nil, status.Error(codes.InvalidArgument, "quantity must be between 1 and 10000")
		}
		var row inventoryProductRow
		if err := s.db.First(&row, "id = ?", item.GetProductId()).Error; err != nil {
			return nil, status.Error(codes.NotFound, "product not found")
		}
		currency = row.Currency
		total += row.UnitPriceMinor * int64(item.GetQuantity())
		out = append(out, dbProductProto(row))
	}
	return &inventoryv1.QuoteProductsResponse{Products: out, TotalMinor: total, Currency: currency}, nil
}
func (s *inventoryServer) ListReservations(context.Context, *inventoryv1.ListReservationsRequest) (*inventoryv1.ListReservationsResponse, error) {
	if s.db != nil {
		var rows []inventoryReservationRow
		if err := s.db.Order("created_at desc").Find(&rows).Error; err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		out := make([]*inventoryv1.Reservation, 0, len(rows))
		for _, r := range rows {
			out = append(out, &inventoryv1.Reservation{Id: r.ID, OrderId: r.OrderID, ProductId: r.ProductID, Quantity: r.Quantity, Status: r.Status, Reason: r.Reason, CreatedAt: r.CreatedAt.Format(time.RFC3339)})
		}
		return &inventoryv1.ListReservationsResponse{Reservations: out, Total: int32(len(out))}, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*inventoryv1.Reservation, 0, len(s.reservations))
	for _, r := range s.reservations {
		out = append(out, &inventoryv1.Reservation{Id: r.ID, OrderId: r.OrderID, ProductId: r.ProductID, Quantity: r.Quantity, Status: r.Status, Reason: r.Reason, CreatedAt: r.CreatedAt.Format(time.RFC3339)})
	}
	return &inventoryv1.ListReservationsResponse{Reservations: out, Total: int32(len(out))}, nil
}
func (s *inventoryServer) ListInventory(ctx context.Context, req *inventoryv1.ListInventoryRequest) (*inventoryv1.ListProductsResponse, error) {
	return s.ListProducts(ctx, &inventoryv1.ListProductsRequest{Page: req.GetPage(), PageSize: req.GetPageSize()})
}
func (s *inventoryServer) AdjustStock(ctx context.Context, req *inventoryv1.AdjustStockRequest) (*inventoryv1.AdjustStockResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	role := "member"
	if values := md.Get("x-user-role"); len(values) > 0 {
		role = values[0]
	}
	if role != "admin" {
		return nil, status.Error(codes.PermissionDenied, "admin role required")
	}
	if req == nil || req.GetProductId() == "" || req.GetDelta() == 0 || strings.TrimSpace(req.GetIdempotencyKey()) == "" {
		return nil, status.Error(codes.InvalidArgument, "product, non-zero delta and idempotency-key are required")
	}
	if s.db == nil {
		return s.adjustStockMemory(req)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s", req.GetProductId(), req.GetDelta(), req.GetReason()))))
	var result inventoryv1.AdjustStockResponse
	inventoryAdjustments.Add(1)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var prior stockMovementRow
		if err := tx.Where("idempotency_key = ?", req.GetIdempotencyKey()).First(&prior).Error; err == nil {
			if prior.RequestHash != hash {
				return status.Error(codes.AlreadyExists, "idempotency key payload mismatch")
			}
			var row inventoryProductRow
			if err := tx.First(&row, "id = ?", prior.ProductID).Error; err != nil {
				return err
			}
			result.Product, result.Movement, result.Replayed = dbProductProto(row), dbMovementProto(prior), true
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var row inventoryProductRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id = ?", req.GetProductId()).Error; err != nil {
			return status.Error(codes.NotFound, "product not found")
		}
		available := row.OnHand - row.Reserved
		if available+req.GetDelta() < 0 {
			return status.Error(codes.FailedPrecondition, "stock cannot become negative")
		}
		row.OnHand += req.GetDelta()
		row.Version++
		row.UpdatedAt = time.Now().UTC()
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		movement := stockMovementRow{ID: uuid.NewString(), ProductID: row.ID, Delta: req.GetDelta(), Reason: req.GetReason(), BalanceAfter: row.OnHand - row.Reserved, IdempotencyKey: req.GetIdempotencyKey(), RequestHash: hash, CreatedAt: time.Now().UTC()}
		if err := tx.Create(&movement).Error; err != nil {
			return err
		}
		result.Product, result.Movement = dbProductProto(row), dbMovementProto(movement)
		return tx.Create(&inventoryOutboxRow{EventID: uuid.NewString(), EventType: "StockAdjusted", AggregateID: row.ID, Payload: fmt.Sprintf(`{"product_id":%q,"delta":%d,"balance_after":%d}`, row.ID, req.GetDelta(), movement.BalanceAfter), OccurredAt: movement.CreatedAt}).Error
	})
	if err != nil {
		inventoryTransactionErrors.Add(1)
		return nil, err
	}
	s.invalidateCatalog(ctx)
	return &result, nil
}
func (s *inventoryServer) adjustStockMemory(req *inventoryv1.AdjustStockRequest) (*inventoryv1.AdjustStockResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stockKeys == nil {
		s.stockKeys = map[string]stockAdjustment{}
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s", req.GetProductId(), req.GetDelta(), req.GetReason()))))
	if prior, ok := s.stockKeys[req.GetIdempotencyKey()]; ok {
		if prior.hash != hash {
			return nil, status.Error(codes.AlreadyExists, "idempotency key payload mismatch")
		}
		copy := *prior.out
		copy.Replayed = true
		return &copy, nil
	}
	p, ok := s.products[req.GetProductId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "product not found")
	}
	if p.available+req.GetDelta() < 0 {
		return nil, status.Error(codes.FailedPrecondition, "stock cannot become negative")
	}
	p.available += req.GetDelta()
	s.products[p.id] = p
	m := &inventoryv1.StockMovement{Id: uuid.NewString(), ProductId: p.id, Delta: req.GetDelta(), Reason: req.GetReason(), BalanceAfter: p.available, IdempotencyKey: req.GetIdempotencyKey(), CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	out := &inventoryv1.AdjustStockResponse{Product: toProto(p), Movement: m}
	s.stockKeys[req.GetIdempotencyKey()] = stockAdjustment{hash: hash, out: out}
	s.invalidateCatalog(context.Background())
	return out, nil
}
func (s *inventoryServer) ListStockMovements(ctx context.Context, req *inventoryv1.ListStockMovementsRequest) (*inventoryv1.ListStockMovementsResponse, error) {
	if s.db == nil {
		return &inventoryv1.ListStockMovementsResponse{}, nil
	}
	var rows []stockMovementRow
	q := s.db.WithContext(ctx).Order("created_at desc")
	if req.GetProductId() != "" {
		q = q.Where("product_id = ?", req.GetProductId())
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	out := make([]*inventoryv1.StockMovement, 0, len(rows))
	for _, row := range rows {
		out = append(out, dbMovementProto(row))
	}
	return &inventoryv1.ListStockMovementsResponse{Movements: out, Total: int32(len(out))}, nil
}
func dbProductProto(row inventoryProductRow) *inventoryv1.Product {
	return &inventoryv1.Product{Id: row.ID, Name: row.Name, UnitPriceMinor: row.UnitPriceMinor, Currency: row.Currency, Available: row.OnHand - row.Reserved}
}
func dbMovementProto(row stockMovementRow) *inventoryv1.StockMovement {
	return &inventoryv1.StockMovement{Id: row.ID, ProductId: row.ProductID, Delta: row.Delta, Reason: row.Reason, BalanceAfter: row.BalanceAfter, IdempotencyKey: row.IdempotencyKey, CreatedAt: row.CreatedAt.Format(time.RFC3339)}
}
func (s *inventoryServer) invalidateCatalog(ctx context.Context) {
	if s.redis == nil {
		return
	}
	if err := s.redis.Del(ctx, catalogCacheKey).Err(); err != nil {
		cacheErrors.Add(1)
		return
	}
	cacheInvalidations.Add(1)
}
func (s *inventoryServer) reserve(e contracts.Envelope) (contracts.Envelope, error) {
	var p createdPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return contracts.Envelope{}, err
	}
	if p.Order.ID == "" {
		p.Order.ID = e.OrderID
	}
	if p.Order.CustomerID == "" {
		p.Order.CustomerID = e.CustomerID
	}
	if s.db != nil {
		return s.reserveDB(e, p)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.processed[e.EventID] {
		return contracts.Envelope{}, nil
	}
	for _, item := range p.Order.Items {
		pr, ok := s.products[item.ProductID]
		if !ok || pr.available < item.Quantity {
			s.processed[e.EventID] = true
			return s.outcome(e, p.Order.CustomerID, "InventoryRejected", "OUT_OF_STOCK", 0, p.Order.Currency, p.Order.PaymentScenario), nil
		}
	}
	for _, item := range p.Order.Items {
		pr := s.products[item.ProductID]
		pr.available -= item.Quantity
		s.products[item.ProductID] = pr
		id := uuid.NewString()
		s.reservations[id] = reservation{ID: id, OrderID: p.Order.ID, ProductID: item.ProductID, Quantity: item.Quantity, Status: "RESERVED", CreatedAt: time.Now().UTC()}
	}
	s.processed[e.EventID] = true
	s.invalidateCatalog(context.Background())
	return s.outcome(e, p.Order.CustomerID, "InventoryReserved", "", p.Order.TotalMinor, p.Order.Currency, p.Order.PaymentScenario), nil
}
func (s *inventoryServer) reserveDB(e contracts.Envelope, p createdPayload) (contracts.Envelope, error) {
	inventoryTransactions.Add(1)
	out := s.outcome(e, p.Order.CustomerID, contracts.EventInventoryReserved, "", p.Order.TotalMinor, p.Order.Currency, p.Order.PaymentScenario)
	err := s.db.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
		var seen inventoryProcessedEventRow
		if err := tx.First(&seen, "event_id = ?", e.EventID).Error; err == nil {
			out = contracts.Envelope{}
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		for _, item := range p.Order.Items {
			var row inventoryProductRow
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id = ?", item.ProductID).Error; err != nil {
				return status.Error(codes.NotFound, "product not found")
			}
			if row.OnHand-row.Reserved < item.Quantity {
				out = s.outcome(e, p.Order.CustomerID, contracts.EventInventoryRejected, "OUT_OF_STOCK", 0, p.Order.Currency, p.Order.PaymentScenario)
				break
			}
		}
		if out.EventType == contracts.EventInventoryReserved {
			for _, item := range p.Order.Items {
				var row inventoryProductRow
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id = ?", item.ProductID).Error; err != nil {
					return err
				}
				row.Reserved += item.Quantity
				row.Version++
				row.UpdatedAt = time.Now().UTC()
				if err := tx.Save(&row).Error; err != nil {
					return err
				}
				if err := tx.Create(&inventoryReservationRow{ID: uuid.NewString(), OrderID: p.Order.ID, ProductID: item.ProductID, Quantity: item.Quantity, Status: "RESERVED", CreatedAt: time.Now().UTC()}).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Create(&inventoryProcessedEventRow{EventID: e.EventID, EventType: e.EventType, ProcessedAt: time.Now().UTC()}).Error; err != nil {
			return err
		}
		return s.createInventoryOutbox(tx, out, p.Order.ID)
	})
	if err != nil {
		inventoryTransactionErrors.Add(1)
		return contracts.Envelope{}, err
	}
	s.invalidateCatalog(context.Background())
	return out, nil
}
func (s *inventoryServer) release(e contracts.Envelope) (contracts.Envelope, error) {
	var p compensationPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return contracts.Envelope{}, err
	}
	if p.OrderID == "" {
		p.OrderID = e.OrderID
	}
	if p.CustomerID == "" {
		p.CustomerID = e.CustomerID
	}
	if s.db != nil {
		return s.releaseDB(e, p)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.processed[e.EventID] {
		return contracts.Envelope{}, nil
	}
	for id, r := range s.reservations {
		if r.OrderID == p.OrderID && r.Status == "RESERVED" {
			pr := s.products[r.ProductID]
			pr.available += r.Quantity
			s.products[r.ProductID] = pr
			r.Status = "RELEASED"
			s.reservations[id] = r
		}
	}
	s.processed[e.EventID] = true
	s.invalidateCatalog(context.Background())
	return s.outcome(e, p.CustomerID, "InventoryReleased", "", 0, "", ""), nil
}
func (s *inventoryServer) releaseDB(e contracts.Envelope, p compensationPayload) (contracts.Envelope, error) {
	inventoryTransactions.Add(1)
	out := s.outcome(e, p.CustomerID, contracts.EventInventoryReleased, "", 0, p.Currency, "")
	err := s.db.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
		var seen inventoryProcessedEventRow
		if err := tx.First(&seen, "event_id = ?", e.EventID).Error; err == nil {
			out = contracts.Envelope{}
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var rows []inventoryReservationRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_id = ? AND status = ?", p.OrderID, "RESERVED").Find(&rows).Error; err != nil {
			return err
		}
		for _, reservation := range rows {
			var product inventoryProductRow
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, "id = ?", reservation.ProductID).Error; err != nil {
				return err
			}
			product.Reserved -= reservation.Quantity
			product.Version++
			product.UpdatedAt = time.Now().UTC()
			if err := tx.Save(&product).Error; err != nil {
				return err
			}
			reservation.Status = "RELEASED"
			if err := tx.Save(&reservation).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&inventoryProcessedEventRow{EventID: e.EventID, EventType: e.EventType, ProcessedAt: time.Now().UTC()}).Error; err != nil {
			return err
		}
		return s.createInventoryOutbox(tx, out, p.OrderID)
	})
	if err != nil {
		inventoryTransactionErrors.Add(1)
		return contracts.Envelope{}, err
	}
	s.invalidateCatalog(context.Background())
	return out, nil
}
func (s *inventoryServer) consumeReservation(e contracts.Envelope) (contracts.Envelope, error) {
	var p compensationPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return contracts.Envelope{}, err
	}
	if p.OrderID == "" {
		p.OrderID = e.OrderID
	}
	if p.CustomerID == "" {
		p.CustomerID = e.CustomerID
	}
	if s.db != nil {
		return s.consumeReservationDB(e, p)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.processed[e.EventID] {
		return contracts.Envelope{}, nil
	}
	for id, r := range s.reservations {
		if r.OrderID == p.OrderID && r.Status == "RESERVED" {
			r.Status = "CONSUMED"
			s.reservations[id] = r
		}
	}
	s.processed[e.EventID] = true
	s.invalidateCatalog(context.Background())
	return s.outcome(e, p.CustomerID, contracts.EventInventoryConsumed, "", 0, p.Currency, ""), nil
}
func (s *inventoryServer) consumeReservationDB(e contracts.Envelope, p compensationPayload) (contracts.Envelope, error) {
	inventoryTransactions.Add(1)
	out := s.outcome(e, p.CustomerID, contracts.EventInventoryConsumed, "", 0, p.Currency, "")
	err := s.db.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
		var seen inventoryProcessedEventRow
		if err := tx.First(&seen, "event_id = ?", e.EventID).Error; err == nil {
			out = contracts.Envelope{}
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var rows []inventoryReservationRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_id = ? AND status = ?", p.OrderID, "RESERVED").Find(&rows).Error; err != nil {
			return err
		}
		for _, reservation := range rows {
			var product inventoryProductRow
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, "id = ?", reservation.ProductID).Error; err != nil {
				return err
			}
			product.Reserved -= reservation.Quantity
			product.OnHand -= reservation.Quantity
			product.Version++
			product.UpdatedAt = time.Now().UTC()
			if err := tx.Save(&product).Error; err != nil {
				return err
			}
			reservation.Status = "CONSUMED"
			if err := tx.Save(&reservation).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&inventoryProcessedEventRow{EventID: e.EventID, EventType: e.EventType, ProcessedAt: time.Now().UTC()}).Error; err != nil {
			return err
		}
		return s.createInventoryOutbox(tx, out, p.OrderID)
	})
	if err != nil {
		inventoryTransactionErrors.Add(1)
		return contracts.Envelope{}, err
	}
	s.invalidateCatalog(context.Background())
	return out, nil
}
func (s *inventoryServer) createInventoryOutbox(tx *gorm.DB, out contracts.Envelope, aggregateID string) error {
	if out.EventID == "" {
		return nil
	}
	b, _ := json.Marshal(out)
	return tx.Create(&inventoryOutboxRow{EventID: out.EventID, EventType: out.EventType, AggregateID: aggregateID, Payload: string(b), OccurredAt: out.OccurredAt}).Error
}
func (s *inventoryServer) outcome(e contracts.Envelope, customer, eventType, reason string, amount int64, currency, scenario string) contracts.Envelope {
	b, _ := json.Marshal(compensationPayload{OrderID: e.OrderID, CustomerID: customer, Reason: reason, AmountMinor: amount, Currency: currency, PaymentScenario: scenario})
	return contracts.Envelope{SchemaVersion: 1, EventID: uuid.NewString(), EventType: eventType, CorrelationID: e.CorrelationID, CausationID: e.EventID, OccurredAt: time.Now().UTC(), CustomerID: customer, OrderID: e.OrderID, Payload: b}
}
func (s *inventoryServer) consume(ctx context.Context, brokers string) {
	if brokers == "" {
		return
	}
	reader := kafka.NewReader(kafka.ReaderConfig{Brokers: strings.Split(brokers, ","), Topic: contracts.OrderEventsTopic, GroupID: "inventory-reservation-v1", MinBytes: 1, MaxBytes: 1 << 20})
	defer reader.Close()
	writer := &kafka.Writer{Addr: kafka.TCP(strings.Split(brokers, ",")...), Topic: contracts.OrderEventsTopic}
	defer writer.Close()
	for {
		m, err := reader.FetchMessage(ctx)
		if err != nil {
			return
		}
		var e contracts.Envelope
		if json.Unmarshal(m.Value, &e) != nil || e.Validate() != nil {
			continue
		}
		var out contracts.Envelope
		if e.EventType == contracts.EventOrderCreated {
			out, err = s.reserve(e)
		} else if e.EventType == contracts.EventInventoryReleaseRequested {
			out, err = s.release(e)
		} else if e.EventType == contracts.EventOrderConfirmed {
			out, err = s.consumeReservation(e)
		}
		if err != nil {
			continue
		}
		if out.EventID != "" && s.db == nil {
			b, _ := json.Marshal(out)
			if err := writer.WriteMessages(ctx, kafka.Message{Key: []byte(out.EventID), Value: b}); err != nil {
				continue
			}
		}
		// commit หลัง reservation/release/consume และ publish สำเร็จ เพื่อให้ retry ปลอดภัย
		if err := reader.CommitMessages(ctx, m); err != nil {
			log.Printf("inventory event commit retry event=%s: %v", e.EventID, err)
		}
	}
}
func (s *inventoryServer) publishOutbox(ctx context.Context, brokers string) {
	if s.db == nil || strings.TrimSpace(brokers) == "" {
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
			var rows []inventoryOutboxRow
			if err := s.db.WithContext(ctx).Where("published_at IS NULL").Order("occurred_at").Limit(50).Find(&rows).Error; err != nil {
				continue
			}
			for _, row := range rows {
				if err := writer.WriteMessages(ctx, kafka.Message{Key: []byte(row.EventID), Value: []byte(row.Payload)}); err != nil {
					continue
				}
				now := time.Now().UTC()
				_ = s.db.WithContext(ctx).Model(&inventoryOutboxRow{}).Where("event_id = ? AND published_at IS NULL", row.EventID).Update("published_at", now).Error
			}
		}
	}
}
func toProto(p product) *inventoryv1.Product {
	return &inventoryv1.Product{Id: p.id, Name: p.name, UnitPriceMinor: p.price, Currency: p.currency, Available: p.available}
}
func health(ctx context.Context, addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(fmt.Sprintf("# HELP service_ready Whether the service can accept traffic.\n# TYPE service_ready gauge\nservice_ready 1\n# TYPE inventory_catalog_cache_hits_total counter\ninventory_catalog_cache_hits_total %d\n# TYPE inventory_catalog_cache_misses_total counter\ninventory_catalog_cache_misses_total %d\n# TYPE inventory_catalog_cache_bypasses_total counter\ninventory_catalog_cache_bypasses_total %d\n# TYPE inventory_catalog_cache_errors_total counter\ninventory_catalog_cache_errors_total %d\n# TYPE inventory_catalog_cache_invalidations_total counter\ninventory_catalog_cache_invalidations_total %d\n# TYPE inventory_transactions_total counter\ninventory_transactions_total %d\n# TYPE inventory_transaction_errors_total counter\ninventory_transaction_errors_total %d\n# TYPE inventory_adjustments_total counter\ninventory_adjustments_total %d\n", cacheHits.Load(), cacheMisses.Load(), cacheBypasses.Load(), cacheErrors.Load(), cacheInvalidations.Load(), inventoryTransactions.Load(), inventoryTransactionErrors.Load(), inventoryAdjustments.Load())))
	})
	srv := &http.Server{Addr: addr, Handler: mux}
	go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("health: %v", err)
		}
	}()
}
func openInventoryDB(ctx context.Context, dsn string) (*gorm.DB, error) {
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
func seedInventoryDB(db *gorm.DB) error {
	seeds := []inventoryProductRow{{ID: "prod-coffee", Name: "House Coffee", Currency: "USD", UnitPriceMinor: 1299, OnHand: 100}, {ID: "prod-mug", Name: "Portfolio Mug", Currency: "USD", UnitPriceMinor: 1899, OnHand: 25}, {ID: "prod-shirt", Name: "Demo T-Shirt", Currency: "USD", UnitPriceMinor: 2499, OnHand: 0}}
	for _, seed := range seeds {
		var existing inventoryProductRow
		err := db.First(&existing, "id = ?", seed.ID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := db.Create(&seed).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	return nil
}
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &inventoryServer{products: map[string]product{
		"prod-coffee": {"prod-coffee", "House Coffee", "USD", 1299, 100},
		"prod-mug":    {"prod-mug", "Portfolio Mug", "USD", 1899, 25},
		"prod-shirt":  {"prod-shirt", "Demo T-Shirt", "USD", 2499, 0},
	}, reservations: map[string]reservation{}, processed: map[string]bool{}}
	if dsn := os.Getenv("INVENTORY_DB_DSN"); dsn != "" {
		db, err := openInventoryDB(ctx, dsn)
		if err != nil {
			log.Fatal(err)
		}
		s.db = db
		if err = db.AutoMigrate(&inventoryProductRow{}, &stockMovementRow{}, &inventoryReservationRow{}, &inventoryProcessedEventRow{}, &inventoryOutboxRow{}); err != nil {
			log.Fatal(err)
		}
		if err = seedInventoryDB(db); err != nil {
			log.Fatal(err)
		}
	}
	if addr := strings.TrimSpace(os.Getenv("REDIS_ADDR")); addr != "" {
		s.redis = redis.NewClient(&redis.Options{Addr: addr})
	}
	lis, err := net.Listen("tcp", env("INVENTORY_ADDR", ":50055"))
	if err != nil {
		log.Fatal(err)
	}
	health(ctx, env("INVENTORY_HTTP_ADDR", ":9106"))
	go s.consume(ctx, os.Getenv("KAFKA_BROKERS"))
	go s.publishOutbox(ctx, os.Getenv("KAFKA_BROKERS"))
	grpcServer := grpc.NewServer()
	inventoryv1.RegisterInventoryServiceServer(grpcServer, s)
	log.Printf("inventory-service listening on %s", lis.Addr())
	if err := grpcServer.Serve(lis); err != nil && !strings.Contains(err.Error(), "closed") {
		log.Fatal(err)
	}
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
