package middleware

import (
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
		c.Locals("user_id", claims["user_id"])
		c.Locals("phone", claims["phone"])
		c.Locals("name", claims["name"])
		c.Locals("role", claims["role"])

		return c.Next()
	}
}
