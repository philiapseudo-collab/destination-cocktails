package middleware

import (
	"fmt"
	"strings"

	"github.com/dumu-tech/destination-cocktails/internal/service"
	"github.com/gofiber/fiber/v2"
)

// AuthMiddleware creates a JWT authentication middleware
func AuthMiddleware(dashboardService *service.DashboardService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get token from cookie
		token := c.Cookies("auth_token")

		// If no cookie, try Authorization header
		if token == "" {
			authHeader := c.Get("Authorization")
			if authHeader != "" {
				// Extract token from "Bearer <token>"
				parts := strings.Split(authHeader, " ")
				if len(parts) == 2 && parts[0] == "Bearer" {
					token = parts[1]
				}
			}
		}

		// EventSource cannot set Authorization headers in browsers.
		// Allow token query param fallback for the SSE endpoint only.
		if token == "" && strings.HasSuffix(c.Path(), "/events") {
			token = strings.TrimSpace(c.Query("token"))
		}

		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "unauthorized: no token provided",
			})
		}

		// Validate token
		claims, err := dashboardService.ValidateJWT(token)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "unauthorized: invalid token",
			})
		}

		// Store claims in context for use in handlers
		c.Locals("user_id", fmt.Sprintf("%v", claims["user_id"]))
		c.Locals("phone", fmt.Sprintf("%v", claims["phone"]))
		c.Locals("name", fmt.Sprintf("%v", claims["name"]))
		c.Locals("role", strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", claims["role"]))))

		return c.Next()
	}
}

// RequireRoles enforces role-based access control after AuthMiddleware.
func RequireRoles(allowedRoles ...string) fiber.Handler {
	allowed := make(map[string]struct{}, len(allowedRoles))
	for _, role := range allowedRoles {
		normalizedRole := strings.ToUpper(strings.TrimSpace(role))
		if normalizedRole != "" {
			allowed[normalizedRole] = struct{}{}
		}
	}

	return func(c *fiber.Ctx) error {
		role := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", c.Locals("role"))))
		if role == "" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "forbidden: role not found in token",
			})
		}

		if _, ok := allowed[role]; !ok {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "forbidden: insufficient permissions",
			})
		}

		return c.Next()
	}
}
