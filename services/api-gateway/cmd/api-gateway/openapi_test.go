package main

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestOpenAPIContainsPublicPaths(t *testing.T) {
	for _, p := range []string{"/healthz", "/api/v1/auth/login", "/api/v1/auth/refresh", "/api/v1/auth/logout", "/api/v1/tasks", "/api/v1/products", "/api/v1/orders", "/api/v1/notifications/read-all", "/api/v1/admin/activities/orders", "/api/v1/admin/inventory/reservations", "/api/v1/admin/payments", "/api/v1/admin/analytics/orders/summary", "/api/v1/admin/analytics/orders/funnel"} {
		if !strings.Contains(openAPI, p) {
			t.Fatalf("openapi missing %s", p)
		}
	}
}

func TestSwaggerPageLoadsSwaggerUIBundle(t *testing.T) {
	if !strings.Contains(swaggerUIHTML, "SwaggerUIBundle") || !strings.Contains(swaggerUIHTML, "'/openapi.yaml'") {
		t.Fatal("swagger page must initialize Swagger UI against the embedded OpenAPI contract")
	}
}

func TestActivityRouteRequiresAdmin(t *testing.T) {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("role", "member")
		return c.Next()
	})
	app.Get("/activities", (&gateway{}).listActivities)

	res, err := app.Test(httptest.NewRequest("GET", "/activities", nil))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != fiber.StatusForbidden {
		t.Fatalf("expected 403 for member, got %d", res.StatusCode)
	}
}
