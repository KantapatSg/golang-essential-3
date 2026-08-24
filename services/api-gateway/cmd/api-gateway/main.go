package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	_ "embed"
	"encoding/pem"
	"errors"
	"fmt"
	activityorder "github.com/KantapatSg/golang-essential-3/contracts/gen/go/activity/v1"
	activityv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/activity/v1"
	analyticsv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/analytics/v1"
	identityv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/identity/v1"
	inventoryv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/inventory/v1"
	notificationv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/notification/v1"
	orderv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/order/v1"
	paymentv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/payment/v1"
	taskv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/task/v1"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type gateway struct {
	identity       identityv1.IdentityServiceClient
	tasks          taskv1.TaskServiceClient
	activities     activityv1.ActivityServiceClient
	analytics      analyticsv1.AnalyticsServiceClient
	order          orderv1.OrderServiceClient
	inventory      inventoryv1.InventoryServiceClient
	notifications  notificationv1.NotificationServiceClient
	orderActivity  activityorder.OrderActivityServiceClient
	orderAnalytics analyticsv1.OrderAnalyticsServiceClient
	payments       paymentv1.PaymentServiceClient
	public         *rsa.PublicKey
}
type loginBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type refreshBody struct {
	RefreshToken string `json:"refresh_token"`
}
type taskBody struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
}
type orderBody struct {
	Items []struct {
		ProductID string `json:"product_id"`
		Quantity  int32  `json:"quantity"`
	} `json:"items"`
	PaymentScenario string `json:"payment_scenario"`
}
type stockAdjustmentBody struct {
	ProductID string `json:"product_id"`
	Delta     int32  `json:"delta"`
	Reason    string `json:"reason"`
}

var httpRequests atomic.Uint64
var httpDurationNanos atomic.Uint64

func main() {
	idAddr := env("IDENTITY_ADDR", "localhost:50051")
	taskAddr := env("TASK_ADDR", "localhost:50052")
	actAddr := env("ACTIVITY_ADDR", "localhost:50053")
	analyticsAddr := env("ANALYTICS_ADDR", "localhost:50054")
	orderAddr := env("ORDER_ADDR", "localhost:50056")
	inventoryAddr := env("INVENTORY_ADDR", "localhost:50055")
	notificationAddr := env("NOTIFICATION_ADDR", "localhost:50058")
	paymentAddr := env("PAYMENT_ADDR", "localhost:50057")
	// Gateway เป็น composition root: จุดนี้ประกอบ gRPC clients และ transport concerns
	// โดยไม่ดึง business logic ของ service อื่นเข้ามาอยู่ใน public edge
	// gRPC uses the native protobuf codec in production.  The checked-in codec remains
	// available to isolated tests/dev tools, but transport must not silently downgrade.
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	idc, e := grpc.Dial(idAddr, opts...)
	if e != nil {
		log.Fatal(e)
	}
	tc, e := grpc.Dial(taskAddr, opts...)
	if e != nil {
		log.Fatal(e)
	}
	ac, e := grpc.Dial(actAddr, opts...)
	if e != nil {
		log.Fatal(e)
	}
	anc, e := grpc.Dial(analyticsAddr, opts...)
	if e != nil {
		log.Print(e)
	}
	oc, e := grpc.Dial(orderAddr, opts...)
	if e != nil {
		log.Print(e)
	}
	ic, e := grpc.Dial(inventoryAddr, opts...)
	if e != nil {
		log.Print(e)
	}
	nc, e := grpc.Dial(notificationAddr, opts...)
	if e != nil {
		log.Print(e)
	}
	pc, e := grpc.Dial(paymentAddr, opts...)
	if e != nil {
		log.Print(e)
	}
	g := &gateway{identity: identityv1.NewIdentityServiceClient(idc), tasks: taskv1.NewTaskServiceClient(tc), activities: activityv1.NewActivityServiceClient(ac), analytics: analyticsv1.NewAnalyticsServiceClient(anc), order: orderv1.NewOrderServiceClient(oc), inventory: inventoryv1.NewInventoryServiceClient(ic), notifications: notificationv1.NewNotificationServiceClient(nc), payments: paymentv1.NewPaymentServiceClient(pc), orderActivity: activityorder.NewOrderActivityServiceClient(ac), orderAnalytics: analyticsv1.NewOrderAnalyticsServiceClient(anc), public: loadPublic(env("JWT_PUBLIC_KEY_PATH", "deploy/keys/public.pem"))}
	app := fiber.New(fiber.Config{AppName: "golang-essential-3"})
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     env("CORS_ORIGINS", "http://localhost:3000,http://localhost:5173"),
		AllowCredentials: true,
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, X-Request-ID",
		ExposeHeaders:    "X-Cache-Status, X-Request-ID",
	}))
	app.Use(requestID)
	app.Get("/health/live", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"status": "ok"}) })
	app.Get("/health/ready", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"status": "ready"}) })
	// Metrics intentionally expose only low-cardinality operational names; IDs belong in logs.
	app.Get("/metrics", func(c *fiber.Ctx) error {
		c.Type("text")
		return c.SendString(fmt.Sprintf("# HELP service_ready Whether the gateway can accept traffic.\n# TYPE service_ready gauge\nservice_ready 1\n# HELP http_requests_total Total HTTP requests.\n# TYPE http_requests_total counter\nhttp_requests_total %d\n# HELP http_request_duration_seconds_total Total HTTP request duration.\n# TYPE http_request_duration_seconds_total counter\nhttp_request_duration_seconds_total %f\n", httpRequests.Load(), float64(httpDurationNanos.Load())/1e9))
	})
	app.Get("/healthz", func(c *fiber.Ctx) error { return c.Redirect("/health/live", fiber.StatusTemporaryRedirect) })
	g.routes(app)
	addr := env("GATEWAY_ADDR", ":8080")
	go func() {
		log.Printf("api-gateway listening on %s", addr)
		if e := app.Listen(addr); e != nil && !errors.Is(e, http.ErrServerClosed) {
			log.Print(e)
		}
	}()
	waitSignal()
	// Shutdown แบบมีเวลาเส้นตายช่วยหยุดรับ HTTP ใหม่และปล่อย request ที่กำลังทำงานให้จบ
	// ก่อน process ถูกปิดจริง เพื่อลดงานที่ค้างกลางทางระหว่าง deploy
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = app.ShutdownWithContext(ctx)
}

// REST is the public edge: it adds request IDs/deadlines and translates gRPC status to HTTP.
func (g *gateway) routes(app *fiber.App) {
	app.Post("/api/v1/auth/login", g.login)
	app.Post("/api/v1/auth/refresh", g.refresh)
	app.Post("/api/v1/auth/logout", g.logout)
	protected := app.Group("/api/v1", g.auth)
	protected.Get("/tasks", g.listTasks)
	protected.Get("/tasks/:id", g.getTask)
	protected.Post("/tasks", g.createTask)
	protected.Put("/tasks/:id", g.updateTask)
	protected.Delete("/tasks/:id", g.deleteTask)
	protected.Get("/products", g.listProducts)
	protected.Post("/orders", g.createOrder)
	protected.Get("/orders", g.listOrders)
	protected.Get("/orders/:id", g.getOrder)
	protected.Get("/notifications", g.listNotifications)
	protected.Get("/notifications/unread-count", g.unreadNotifications)
	protected.Patch("/notifications/:id/read", g.markNotificationRead)
	protected.Post("/notifications/read-all", g.markAllNotificationsRead)
	protected.Get("/activities", g.listActivities)
	protected.Get("/analytics/summary", g.analyticsSummary)
	protected.Get("/analytics/timeseries", g.analyticsTimeseries)
	protected.Get("/analytics/statuses", g.analyticsStatuses)
	protected.Get("/admin/analytics/orders/summary", g.orderSummary)
	protected.Get("/admin/analytics/orders/funnel", g.orderFunnel)
	protected.Get("/admin/activities/orders", g.listOrderActivities)
	protected.Get("/admin/inventory/reservations", g.listReservations)
	protected.Get("/admin/inventory", g.listInventory)
	protected.Post("/admin/inventory/adjustments", g.adjustInventory)
	protected.Get("/admin/inventory/movements", g.listStockMovements)
	protected.Get("/admin/payments", g.listPayments)
	app.Get("/openapi.yaml", func(c *fiber.Ctx) error { c.Type("yaml"); return c.SendString(openAPI) })
	app.Get("/swagger/", func(c *fiber.Ctx) error {
		return c.Type("html").SendString(swaggerUIHTML)
	})
}
func (g *gateway) login(c *fiber.Ctx) error {
	var b loginBody
	if e := c.BodyParser(&b); e != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid JSON"})
	}
	if strings.TrimSpace(b.Email) == "" || b.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email and password are required"})
	}
	ctx, cancel := rpcCtx(c)
	defer cancel()
	r, e := g.identity.Login(ctx, &identityv1.LoginRequest{Email: b.Email, Password: b.Password})
	if e != nil {
		return grpcHTTP(c, e)
	}
	setRefreshCookie(c, r.RefreshToken)
	return c.JSON(r)
}
func (g *gateway) refresh(c *fiber.Ctx) error {
	var b refreshBody
	if e := c.BodyParser(&b); e != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid JSON"})
	}
	if strings.TrimSpace(b.RefreshToken) == "" {
		b.RefreshToken = c.Cookies("refresh_token")
	}
	if strings.TrimSpace(b.RefreshToken) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "refresh_token is required"})
	}
	ctx, cancel := rpcCtx(c)
	defer cancel()
	r, e := g.identity.Refresh(ctx, &identityv1.RefreshRequest{RefreshToken: b.RefreshToken})
	if e != nil {
		return grpcHTTP(c, e)
	}
	setRefreshCookie(c, r.RefreshToken)
	return c.JSON(r)
}

func setRefreshCookie(c *fiber.Ctx, token string) {
	if token == "" {
		return
	}
	// Refresh token เป็น session secret: cookie แบบ HttpOnly ทำให้ browser refresh ได้หลัง reload
	// โดยไม่เปิด token ให้ JavaScript อ่านหรือส่งต่อผิด boundary
	c.Cookie(&fiber.Cookie{Name: "refresh_token", Value: token, HTTPOnly: true, Secure: env("COOKIE_SECURE", "false") == "true", SameSite: "Lax", Path: "/api/v1/auth", Domain: env("COOKIE_DOMAIN", ""), MaxAge: int(7 * 24 * time.Hour.Seconds())})
}
func (g *gateway) logout(c *fiber.Ctx) error {
	var b refreshBody
	_ = c.BodyParser(&b)
	if strings.TrimSpace(b.RefreshToken) == "" {
		b.RefreshToken = c.Cookies("refresh_token")
	}
	ctx, cancel := rpcCtx(c)
	defer cancel()
	_, e := g.identity.Logout(ctx, &identityv1.LogoutRequest{RefreshToken: b.RefreshToken})
	if e != nil {
		return grpcHTTP(c, e)
	}
	// ลบ cookie ด้วย Path/Domain เดียวกับตอนออก token ไม่เช่นนั้น browser จะเก็บ session secret ไว้
	c.Cookie(&fiber.Cookie{Name: "refresh_token", Value: "", HTTPOnly: true, Secure: env("COOKIE_SECURE", "false") == "true", SameSite: "Lax", Path: "/api/v1/auth", Domain: env("COOKIE_DOMAIN", ""), MaxAge: -1, Expires: time.Unix(1, 0).UTC()})
	return c.SendStatus(204)
}
func (g *gateway) auth(c *fiber.Ctx) error {
	// Gateway ตรวจลายเซ็นด้วย public key เท่านั้น ส่วน Identity Service เป็นผู้ถือ private key
	// ทำให้ public edge ยืนยันตัวตนได้โดยไม่มีสิทธิ์ออก token เอง
	h := c.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return c.Status(401).JSON(fiber.Map{"error": "missing bearer token"})
	}
	tok, e := jwt.Parse(strings.TrimPrefix(h, "Bearer "), func(t *jwt.Token) (interface{}, error) {
		if t.Method.Alg() != "RS256" {
			return nil, errors.New("unexpected signing method")
		}
		return g.public, nil
	})
	if e != nil || !tok.Valid {
		return c.Status(401).JSON(fiber.Map{"error": "invalid token"})
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "invalid claims"})
	}
	sub, _ := claims["sub"].(string)
	role, _ := claims["role"].(string)
	c.Locals("user_id", sub)
	c.Locals("role", role)
	return c.Next()
}
func (g *gateway) listTasks(c *fiber.Ctx) error {
	ctx, cancel := rpcCtx(c)
	defer cancel()
	r, e := g.tasks.ListTasks(withActor(ctx, c), &taskv1.ListTasksRequest{})
	if e != nil {
		return grpcHTTP(c, e)
	}
	page, size, e := pagination(c)
	if e != nil {
		return e
	}
	start := (page - 1) * size
	if start >= len(r.Tasks) {
		return c.JSON(fiber.Map{"items": []*taskv1.Task{}, "page": page, "page_size": size, "total": len(r.Tasks)})
	}
	end := start + size
	if end > len(r.Tasks) {
		end = len(r.Tasks)
	}
	return c.JSON(fiber.Map{"items": r.Tasks[start:end], "page": page, "page_size": size, "total": len(r.Tasks)})
}
func (g *gateway) getTask(c *fiber.Ctx) error {
	ctx, cancel := rpcCtx(c)
	defer cancel()
	r, e := g.tasks.GetTask(withActor(ctx, c), &taskv1.GetTaskRequest{Id: c.Params("id")})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.JSON(r)
}
func (g *gateway) createTask(c *fiber.Ctx) error {
	var b taskBody
	if e := c.BodyParser(&b); e != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid JSON"})
	}
	if strings.TrimSpace(b.Title) == "" || len([]rune(b.Title)) > 200 {
		return c.Status(400).JSON(fiber.Map{"error": "title is required and must be at most 200 characters"})
	}
	ctx, cancel := rpcCtx(c)
	defer cancel()
	r, e := g.tasks.CreateTask(withActor(ctx, c), &taskv1.CreateTaskRequest{Title: b.Title, Description: b.Description})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.Status(201).JSON(r)
}
func (g *gateway) updateTask(c *fiber.Ctx) error {
	var b taskBody
	if e := c.BodyParser(&b); e != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid JSON"})
	}
	if strings.TrimSpace(b.Title) == "" || !map[string]bool{"todo": true, "doing": true, "done": true}[b.Status] {
		return c.Status(400).JSON(fiber.Map{"error": "title and status are required"})
	}
	ctx, cancel := rpcCtx(c)
	defer cancel()
	r, e := g.tasks.UpdateTask(withActor(ctx, c), &taskv1.UpdateTaskRequest{Id: c.Params("id"), Title: b.Title, Description: b.Description, Status: b.Status})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.JSON(r)
}
func (g *gateway) deleteTask(c *fiber.Ctx) error {
	ctx, cancel := rpcCtx(c)
	defer cancel()
	_, e := g.tasks.DeleteTask(withActor(ctx, c), &taskv1.DeleteTaskRequest{Id: c.Params("id")})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.SendStatus(204)
}
func (g *gateway) listProducts(c *fiber.Ctx) error {
	ctx, cancel := rpcCtx(c)
	defer cancel()
	var headers metadata.MD
	r, e := g.inventory.ListProducts(withActor(ctx, c), &inventoryv1.ListProductsRequest{Page: 1, PageSize: 100}, grpc.Header(&headers))
	if e != nil {
		return grpcHTTP(c, e)
	}
	if values := headers.Get("x-cache-status"); len(values) > 0 {
		c.Set("X-Cache-Status", values[0])
	}
	return c.JSON(fiber.Map{"products": r.Products, "total": r.Total})
}
func (g *gateway) createOrder(c *fiber.Ctx) error {
	key := strings.TrimSpace(c.Get("Idempotency-Key"))
	if key == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Idempotency-Key is required"})
	}
	var b orderBody
	if e := c.BodyParser(&b); e != nil || len(b.Items) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "items are required"})
	}
	items := make([]*orderv1.CreateOrderItem, 0, len(b.Items))
	for _, i := range b.Items {
		items = append(items, &orderv1.CreateOrderItem{ProductId: i.ProductID, Quantity: i.Quantity})
	}
	ctx, cancel := rpcCtx(c)
	defer cancel()
	r, e := g.order.CreateOrder(withActor(ctx, c), &orderv1.CreateOrderRequest{CustomerId: c.Locals("user_id").(string), Items: items, PaymentScenario: b.PaymentScenario, IdempotencyKey: key})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.Status(202).JSON(r)
}
func (g *gateway) listOrders(c *fiber.Ctx) error {
	ctx, cancel := rpcCtx(c)
	defer cancel()
	r, e := g.order.ListOrders(withActor(ctx, c), &orderv1.ListOrdersRequest{Page: 1, PageSize: 100})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.JSON(fiber.Map{"items": r.Orders, "total": r.Total})
}
func (g *gateway) getOrder(c *fiber.Ctx) error {
	ctx, cancel := rpcCtx(c)
	defer cancel()
	r, e := g.order.GetOrder(withActor(ctx, c), &orderv1.GetOrderRequest{Id: c.Params("id")})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.JSON(r)
}
func (g *gateway) listNotifications(c *fiber.Ctx) error {
	ctx, cancel := rpcCtx(c)
	defer cancel()
	r, e := g.notifications.ListNotifications(withActor(ctx, c), &notificationv1.ListNotificationsRequest{Page: 1, PageSize: 100})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.JSON(fiber.Map{"items": r.Notifications, "total": r.Total})
}
func (g *gateway) unreadNotifications(c *fiber.Ctx) error {
	ctx, cancel := rpcCtx(c)
	defer cancel()
	r, e := g.notifications.UnreadCount(withActor(ctx, c), &notificationv1.UnreadCountRequest{})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.JSON(r)
}
func (g *gateway) markNotificationRead(c *fiber.Ctx) error {
	ctx, cancel := rpcCtx(c)
	defer cancel()
	r, e := g.notifications.MarkAsRead(withActor(ctx, c), &notificationv1.MarkAsReadRequest{Id: c.Params("id")})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.JSON(r)
}
func (g *gateway) markAllNotificationsRead(c *fiber.Ctx) error {
	ctx, cancel := rpcCtx(c)
	defer cancel()
	_, e := g.notifications.MarkAllAsRead(withActor(ctx, c), &notificationv1.MarkAllAsReadRequest{})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.SendStatus(204)
}
func (g *gateway) orderSummary(c *fiber.Ctx) error {
	if e := adminOnly(c); e != nil {
		return e
	}
	ctx, cancel := rpcCtx(c)
	defer cancel()
	r, e := g.orderAnalytics.OrderSummary(ctx, &analyticsv1.OrderSummaryRequest{From: c.Query("from"), To: c.Query("to")})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.JSON(r)
}
func (g *gateway) orderFunnel(c *fiber.Ctx) error {
	if e := adminOnly(c); e != nil {
		return e
	}
	ctx, cancel := rpcCtx(c)
	defer cancel()
	r, e := g.orderAnalytics.OrderFunnel(ctx, &analyticsv1.OrderSummaryRequest{From: c.Query("from"), To: c.Query("to")})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.JSON(r)
}
func (g *gateway) listOrderActivities(c *fiber.Ctx) error {
	if e := adminOnly(c); e != nil {
		return e
	}
	ctx, cancel := rpcCtx(c)
	defer cancel()
	r, e := g.orderActivity.ListOrderActivities(withActor(ctx, c), &activityorder.ListOrderActivitiesRequest{Page: 1, PageSize: 100})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.JSON(fiber.Map{"items": r.Activities, "total": r.Total})
}
func (g *gateway) listReservations(c *fiber.Ctx) error {
	if e := adminOnly(c); e != nil {
		return e
	}
	ctx, cancel := rpcCtx(c)
	defer cancel()
	r, e := g.inventory.ListReservations(ctx, &inventoryv1.ListReservationsRequest{Page: 1, PageSize: 100})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.JSON(r)
}
func (g *gateway) listInventory(c *fiber.Ctx) error {
	if e := adminOnly(c); e != nil {
		return e
	}
	ctx, cancel := rpcCtx(c)
	defer cancel()
	r, e := g.inventory.ListInventory(withActor(ctx, c), &inventoryv1.ListInventoryRequest{Page: 1, PageSize: 100})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.JSON(fiber.Map{"products": r.Products, "total": r.Total})
}
func (g *gateway) adjustInventory(c *fiber.Ctx) error {
	if e := adminOnly(c); e != nil {
		return e
	}
	key := strings.TrimSpace(c.Get("Idempotency-Key"))
	if key == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Idempotency-Key is required"})
	}
	var b stockAdjustmentBody
	if e := c.BodyParser(&b); e != nil || strings.TrimSpace(b.ProductID) == "" || b.Delta == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "product_id and non-zero delta are required"})
	}
	ctx, cancel := rpcCtx(c)
	defer cancel()
	r, e := g.inventory.AdjustStock(withActor(ctx, c), &inventoryv1.AdjustStockRequest{ProductId: b.ProductID, Delta: b.Delta, Reason: b.Reason, IdempotencyKey: key})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.JSON(r)
}
func (g *gateway) listStockMovements(c *fiber.Ctx) error {
	if e := adminOnly(c); e != nil {
		return e
	}
	ctx, cancel := rpcCtx(c)
	defer cancel()
	r, e := g.inventory.ListStockMovements(withActor(ctx, c), &inventoryv1.ListStockMovementsRequest{ProductId: c.Query("product_id"), Page: 1, PageSize: 100})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.JSON(r)
}
func (g *gateway) listPayments(c *fiber.Ctx) error {
	if e := adminOnly(c); e != nil {
		return e
	}
	ctx, cancel := rpcCtx(c)
	defer cancel()
	r, e := g.payments.ListPayments(ctx, &paymentv1.ListPaymentsRequest{Page: 1, PageSize: 100})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.JSON(r)
}
func (g *gateway) listActivities(c *fiber.Ctx) error {
	if orderID := c.Query("order_id"); orderID != "" && g.orderActivity != nil {
		ctx, cancel := rpcCtx(c)
		defer cancel()
		r, e := g.orderActivity.ListOrderActivities(withActor(ctx, c), &activityorder.ListOrderActivitiesRequest{OrderId: orderID, Page: 1, PageSize: 100})
		if e != nil {
			return grpcHTTP(c, e)
		}
		return c.JSON(r.Activities)
	}
	// Activity log เป็นข้อมูลรวมของระบบ จึงจำกัดให้ admin แม้ JWT จะผ่านแล้ว
	// การตรวจ policy ที่ edge ช่วยตอบ 403 ได้เร็ว ก่อนยิง RPC ไปอีก service
	if role, _ := c.Locals("role").(string); role != "admin" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "admin role required"})
	}
	ctx, cancel := rpcCtx(c)
	defer cancel()
	r, e := g.activities.ListActivities(withActor(ctx, c), &activityv1.ListActivitiesRequest{})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.JSON(r.Activities)
}
func adminOnly(c *fiber.Ctx) error {
	if role, _ := c.Locals("role").(string); role != "admin" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "admin role required"})
	}
	return nil
}
func timeRange(c *fiber.Ctx) (*analyticsv1.TimeRange, uint32, error) {
	from, to := c.Query("from"), c.Query("to")
	limit := uint32(100)
	if v := c.Query("limit"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 || n > 500 {
			return nil, 0, c.Status(400).JSON(fiber.Map{"error": "limit must be between 1 and 500"})
		}
		limit = uint32(n)
	}
	return &analyticsv1.TimeRange{From: from, To: to, Timezone: c.Query("timezone", "UTC")}, limit, nil
}
func (g *gateway) analyticsSummary(c *fiber.Ctx) error {
	if e := adminOnly(c); e != nil {
		return e
	}
	if g.analytics == nil {
		return c.Status(503).JSON(fiber.Map{"error": "analytics unavailable"})
	}
	r, limit, e := timeRange(c)
	if e != nil {
		return e
	}
	ctx, cancel := rpcCtx(c)
	defer cancel()
	out, e := g.analytics.Summary(ctx, &analyticsv1.SummaryRequest{Range: r, Limit: limit})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.JSON(out)
}
func (g *gateway) analyticsTimeseries(c *fiber.Ctx) error {
	if e := adminOnly(c); e != nil {
		return e
	}
	if g.analytics == nil {
		return c.Status(503).JSON(fiber.Map{"error": "analytics unavailable"})
	}
	r, limit, e := timeRange(c)
	if e != nil {
		return e
	}
	ctx, cancel := rpcCtx(c)
	defer cancel()
	out, e := g.analytics.Timeseries(ctx, &analyticsv1.TimeseriesRequest{Range: r, Limit: limit})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.JSON(out)
}
func (g *gateway) analyticsStatuses(c *fiber.Ctx) error {
	if e := adminOnly(c); e != nil {
		return e
	}
	if g.analytics == nil {
		return c.Status(503).JSON(fiber.Map{"error": "analytics unavailable"})
	}
	r, limit, e := timeRange(c)
	if e != nil {
		return e
	}
	ctx, cancel := rpcCtx(c)
	defer cancel()
	out, e := g.analytics.Statuses(ctx, &analyticsv1.StatusesRequest{Range: r, Limit: limit})
	if e != nil {
		return grpcHTTP(c, e)
	}
	return c.JSON(out)
}
func withActor(ctx context.Context, c *fiber.Ctx) context.Context {
	// ส่ง identity ข้าม network boundary ด้วย gRPC metadata เพื่อให้ service ปลายทาง
	// บังคับ ownership/RBAC ซ้ำได้ ไม่พึ่งการตรวจที่ Gateway เพียงชั้นเดียว
	return metadataAppend(ctx, "x-user-id", c.Locals("user_id").(string), "x-user-role", c.Locals("role").(string))
}
func rpcCtx(c *fiber.Ctx) (context.Context, context.CancelFunc) {
	// ทุก synchronous RPC ต้องมี deadline เพื่อไม่ให้ HTTP request แขวนเมื่อ service ภายในล่ม
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	return metadataAppend(ctx, "x-request-id", c.Get("X-Request-ID")), cancel
}
func metadataAppend(ctx context.Context, kv ...string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, kv...)
}
func grpcHTTP(c *fiber.Ctx, e error) error {
	// Mapping อยู่ที่ edge เพราะ HTTP client ไม่ควรรู้จัก gRPC status code ภายในระบบ
	code := status.Code(e)
	m := map[codes.Code]int{codes.InvalidArgument: 400, codes.Unauthenticated: 401, codes.PermissionDenied: 403, codes.NotFound: 404, codes.DeadlineExceeded: 504, codes.Unavailable: 503, codes.Canceled: 499}
	if v, ok := m[code]; ok {
		return c.Status(v).JSON(fiber.Map{"error": status.Convert(e).Message()})
	}
	return c.Status(500).JSON(fiber.Map{"error": status.Convert(e).Message()})
}
func pagination(c *fiber.Ctx) (int, int, error) {
	page, size := 1, 20
	if v := c.Query("page"); v != "" {
		if _, e := fmt.Sscanf(v, "%d", &page); e != nil || page < 1 {
			return 0, 0, c.Status(400).JSON(fiber.Map{"error": "page must be a positive integer"})
		}
	}
	if v := c.Query("page_size"); v != "" {
		if _, e := fmt.Sscanf(v, "%d", &size); e != nil || size < 1 || size > 100 {
			return 0, 0, c.Status(400).JSON(fiber.Map{"error": "page_size must be between 1 and 100"})
		}
	}
	return page, size, nil
}
func requestID(c *fiber.Ctx) error {
	started := time.Now()
	defer func() { httpRequests.Add(1); httpDurationNanos.Add(uint64(time.Since(started).Nanoseconds())) }()
	if c.Get("X-Request-ID") == "" {
		c.Set("X-Request-ID", uuid.NewString())
	}
	return c.Next()
}
func loadPublic(path string) *rsa.PublicKey {
	for i := 0; i < 20; i++ {
		b, e := os.ReadFile(path)
		if e == nil {
			if block, _ := pem.Decode(b); block != nil {
				if x, e := x509.ParsePKIXPublicKey(block.Bytes); e == nil {
					if k, ok := x.(*rsa.PublicKey); ok {
						return k
					}
				}
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	if strings.EqualFold(os.Getenv("DEV_MODE"), "true") {
		// DEV_MODE เท่านั้นที่ยอมใช้ key ชั่วคราวสำหรับ unit test; production ต้อง fail fast
		// เพราะ fallback key ทำให้ token จาก Identity ตรวจลายเซ็นไม่ผ่านและปิดบัง config ผิดพลาด
		k, _ := rsa.GenerateKey(rand.Reader, 1024)
		return &k.PublicKey
	}
	log.Fatalf("JWT public key is required: %s", path)
	return nil
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func waitSignal() { ch := make(chan os.Signal, 1); signal.Notify(ch, os.Interrupt); <-ch }

//go:embed openapi.yaml
var openAPI string

const swaggerUIHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>golang-essential-3 Swagger</title>
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css"></head>
<body><div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>window.ui = SwaggerUIBundle({url: '/openapi.yaml', dom_id: '#swagger-ui', deepLinking: true});</script>
</body></html>`
