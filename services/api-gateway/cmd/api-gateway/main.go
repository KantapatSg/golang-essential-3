package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	activityv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/activity/v1"
	analyticsv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/analytics/v1"
	identityv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/identity/v1"
	taskv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/task/v1"
	"github.com/gofiber/fiber/v2"
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
	identity   identityv1.IdentityServiceClient
	tasks      taskv1.TaskServiceClient
	activities activityv1.ActivityServiceClient
	analytics  analyticsv1.AnalyticsServiceClient
	public     *rsa.PublicKey
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

var httpRequests atomic.Uint64
var httpDurationNanos atomic.Uint64

func main() {
	idAddr := env("IDENTITY_ADDR", "localhost:50051")
	taskAddr := env("TASK_ADDR", "localhost:50052")
	actAddr := env("ACTIVITY_ADDR", "localhost:50053")
	analyticsAddr := env("ANALYTICS_ADDR", "localhost:50054")
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
	g := &gateway{identity: identityv1.NewIdentityServiceClient(idc), tasks: taskv1.NewTaskServiceClient(tc), activities: activityv1.NewActivityServiceClient(ac), analytics: analyticsv1.NewAnalyticsServiceClient(anc), public: loadPublic(env("JWT_PUBLIC_KEY_PATH", "deploy/keys/public.pem"))}
	app := fiber.New(fiber.Config{AppName: "golang-essential-3"})
	app.Use(recover.New())
	app.Use(requestID)
	app.Get("/health/live", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"status": "ok"}) })
	app.Get("/health/ready", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"status": "ready"}) })
	// Metrics intentionally expose only low-cardinality operational names; IDs belong in logs.
	app.Get("/metrics", func(c *fiber.Ctx) error {
		c.Type("text")
		return c.SendString(fmt.Sprintf("service_ready 1\nhttp_requests_total %d\nhttp_request_duration_seconds_total %f\n", httpRequests.Load(), float64(httpDurationNanos.Load())/1e9))
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
	protected.Get("/activities", g.listActivities)
	protected.Get("/analytics/summary", g.analyticsSummary)
	protected.Get("/analytics/timeseries", g.analyticsTimeseries)
	protected.Get("/analytics/statuses", g.analyticsStatuses)
	app.Get("/openapi.yaml", func(c *fiber.Ctx) error { c.Type("yaml"); return c.SendString(openAPI) })
	app.Get("/swagger/", func(c *fiber.Ctx) error {
		return c.Type("html").SendString("<!doctype html><title>Swagger</title><p>OpenAPI: <a href='/openapi.yaml'>/openapi.yaml</a></p>")
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
	// Refresh credentials are session secrets: prefer an HttpOnly cookie so browser
	// JavaScript cannot exfiltrate them. JSON is retained for non-browser API clients.
	if r.RefreshToken != "" {
		c.Cookie(&fiber.Cookie{Name: "refresh_token", Value: r.RefreshToken, HTTPOnly: true, Secure: env("COOKIE_SECURE", "false") == "true", SameSite: "Lax", Path: "/api/v1/auth", MaxAge: int(7 * 24 * time.Hour.Seconds())})
	}
	return c.JSON(r)
}
func (g *gateway) logout(c *fiber.Ctx) error {
	var b refreshBody
	_ = c.BodyParser(&b)
	ctx, cancel := rpcCtx(c)
	defer cancel()
	_, e := g.identity.Logout(ctx, &identityv1.LogoutRequest{RefreshToken: b.RefreshToken})
	if e != nil {
		return grpcHTTP(c, e)
	}
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
func (g *gateway) listActivities(c *fiber.Ctx) error {
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
	// Local-only fallback lets the gateway boot without the identity volume; compose normally shares the real key.
	k, _ := rsa.GenerateKey(rand.Reader, 1024)
	return &k.PublicKey
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func waitSignal() { ch := make(chan os.Signal, 1); signal.Notify(ch, os.Interrupt); <-ch }

var openAPI = `openapi: 3.0.3
info:
  title: golang-essential-3 API
  version: 1.0.0
paths:
  /health/live: {get: {responses: {'200': {description: ok}}}}
  /health/ready: {get: {responses: {'200': {description: ready}}}}
  /healthz: {get: {responses: {'307': {description: legacy redirect}}}}
  /api/v1/auth/login: {post: {responses: {'200': {description: token}}}}
  /api/v1/tasks: {get: {responses: {'200': {description: paginated tasks}}}, post: {responses: {'201': {description: task}}}}
  /api/v1/tasks/{id}: {get: {responses: {'200': {description: task}, '404': {description: not found}}}, put: {responses: {'200': {description: task}}}, delete: {responses: {'204': {description: deleted}}}}
  /api/v1/analytics/summary: {get: {responses: {'200': {description: admin summary}, '403': {description: forbidden}}}}
  /api/v1/analytics/timeseries: {get: {responses: {'200': {description: admin timeseries}}}}
  /api/v1/analytics/statuses: {get: {responses: {'200': {description: admin statuses}}}}
`
