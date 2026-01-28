package http

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/dumu-tech/destination-cocktails/internal/events"
	"github.com/dumu-tech/destination-cocktails/internal/service"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// DashboardHandler handles dashboard HTTP requests
type DashboardHandler struct {
	dashboardService *service.DashboardService
}

// NewDashboardHandler creates a new dashboard handler
func NewDashboardHandler(dashboardService *service.DashboardService) *DashboardHandler {
	return &DashboardHandler{
		dashboardService: dashboardService,
	}
}

// RequestOTP handles OTP request
// POST /api/admin/auth/request-otp
func (h *DashboardHandler) RequestOTP(c *fiber.Ctx) error {
	var req struct {
		Phone string `json:"phone"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	if req.Phone == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "phone number is required",
		})
	}

	if err := h.dashboardService.RequestOTP(c.Context(), req.Phone); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "OTP sent successfully",
	})
}

// VerifyOTP handles OTP verification
// POST /api/admin/auth/verify-otp
func (h *DashboardHandler) VerifyOTP(c *fiber.Ctx) error {
	var req struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	if req.Phone == "" || req.Code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "phone and code are required",
		})
	}

	token, err := h.dashboardService.VerifyOTP(c.Context(), req.Phone, req.Code)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Set JWT token in HTTP-only cookie
	c.Cookie(&fiber.Cookie{
		Name:     "auth_token",
		Value:    token,
		Expires:  time.Now().Add(7 * 24 * time.Hour),
		HTTPOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: "Lax",
	})

	return c.JSON(fiber.Map{
		"message": "login successful",
		"token":   token,
	})
}

// Logout handles user logout
// POST /api/admin/auth/logout
func (h *DashboardHandler) Logout(c *fiber.Ctx) error {
	// Clear auth cookie
	c.Cookie(&fiber.Cookie{
		Name:     "auth_token",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HTTPOnly: true,
	})

	return c.JSON(fiber.Map{
		"message": "logged out successfully",
	})
}

// GetMe returns current user info
// GET /api/admin/auth/me
func (h *DashboardHandler) GetMe(c *fiber.Ctx) error {
	// Get admin user from database
	phone := c.Locals("phone").(string)
	adminUser, err := h.dashboardService.GetAdminUserByPhone(c.Context(), phone)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to get user",
		})
	}

	return c.JSON(adminUser) // Returns full AdminUser struct
}

// GetProducts retrieves all products
// GET /api/admin/products
func (h *DashboardHandler) GetProducts(c *fiber.Ctx) error {
	products, err := h.dashboardService.GetProducts(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to get products",
		})
	}

	return c.JSON(products)
}

// UpdateStock updates product stock
// PATCH /api/admin/products/:id/stock
func (h *DashboardHandler) UpdateStock(c *fiber.Ctx) error {
	productID := c.Params("id")
	if productID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "product ID is required",
		})
	}

	var req struct {
		StockQuantity int `json:"stock_quantity"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	if err := h.dashboardService.UpdateStock(c.Context(), productID, req.StockQuantity); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "stock updated successfully",
	})
}

// UpdatePrice updates product price
// PATCH /api/admin/products/:id/price
func (h *DashboardHandler) UpdatePrice(c *fiber.Ctx) error {
	productID := c.Params("id")
	if productID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "product ID is required",
		})
	}

	var req struct {
		Price float64 `json:"price"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	if req.Price <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "price must be greater than 0",
		})
	}

	if err := h.dashboardService.UpdatePrice(c.Context(), productID, req.Price); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "price updated successfully",
	})
}

// GetOrders retrieves orders with optional filters
// GET /api/admin/orders?status=PAID&limit=50
func (h *DashboardHandler) GetOrders(c *fiber.Ctx) error {
	status := c.Query("status", "")
	limitStr := c.Query("limit", "100")

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		limit = 100
	}

	orders, err := h.dashboardService.GetOrders(c.Context(), status, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to get orders",
		})
	}

	return c.JSON(orders)
}

// GetAnalyticsOverview retrieves dashboard overview metrics
// GET /api/admin/analytics/overview
func (h *DashboardHandler) GetAnalyticsOverview(c *fiber.Ctx) error {
	analytics, err := h.dashboardService.GetAnalyticsOverview(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to get analytics",
		})
	}

	return c.JSON(analytics)
}

// GetRevenueTrend retrieves revenue trend data
// GET /api/admin/analytics/revenue?days=30
func (h *DashboardHandler) GetRevenueTrend(c *fiber.Ctx) error {
	daysStr := c.Query("days", "30")
	days, err := strconv.Atoi(daysStr)
	if err != nil {
		days = 30
	}

	trends, err := h.dashboardService.GetRevenueTrend(c.Context(), days)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to get revenue trend",
		})
	}

	return c.JSON(trends)
}

// GetTopProducts retrieves top-selling products
// GET /api/admin/analytics/top-products?limit=10
func (h *DashboardHandler) GetTopProducts(c *fiber.Ctx) error {
	limitStr := c.Query("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		limit = 10
	}

	products, err := h.dashboardService.GetTopProducts(c.Context(), limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to get top products",
		})
	}

	return c.JSON(products)
}

// SSEEvents handles Server-Sent Events for real-time updates
// GET /api/admin/events
func (h *DashboardHandler) SSEEvents(c *fiber.Ctx) error {
	// Set headers for SSE
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")

	// Create context with timeout
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()

	// Subscribe to event bus
	subscriberID := uuid.New().String()
	eventChan := h.dashboardService.GetEventBus().Subscribe(ctx, subscriberID)

	// Send initial connection message
	c.Write([]byte("event: connected\ndata: {\"message\":\"connected\"}\n\n"))

	// Stream events
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		// Send heartbeat every 30 seconds
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case event, ok := <-eventChan:
				if !ok {
					return
				}

				// Format and send event
				sseData, err := events.FormatSSE(event)
				if err != nil {
					fmt.Printf("Error formatting SSE: %v\n", err)
					continue
				}

				if _, err := w.Write([]byte(sseData)); err != nil {
					return
				}

				if err := w.Flush(); err != nil {
					return
				}

			case <-ticker.C:
				// Send heartbeat
				if _, err := w.Write([]byte(": heartbeat\n\n")); err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}

			case <-ctx.Done():
				return
			}
		}
	})

	return nil
}
