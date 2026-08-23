package main

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestOpenAPIContainsPublicPaths(t *testing.T) {
	for _, p := range []string{"/healthz", "/api/v1/auth/login", "/api/v1/tasks"} {
		if !strings.Contains(openAPI, p) {
			t.Fatalf("openapi missing %s", p)
		}
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
