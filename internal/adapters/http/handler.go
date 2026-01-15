package http

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/dumu-tech/destination-cocktails/internal/adapters/whatsapp"
	"github.com/dumu-tech/destination-cocktails/internal/config"
	"github.com/dumu-tech/destination-cocktails/internal/core"
	"github.com/gofiber/fiber/v2"
)

// Handler handles HTTP requests for WhatsApp webhooks and payment webhooks
type Handler struct {
	verifyToken   string
	appSecret     string
	botService    BotServiceHandler
	paymentGateway PaymentGatewayHandler
	orderRepo      OrderRepositoryHandler
	whatsappGateway WhatsAppGatewayHandler
}

// PaymentGatewayHandler defines the interface for payment gateway
type PaymentGatewayHandler interface {
	VerifyWebhook(ctx context.Context, signature string, payload []byte) bool
	ProcessWebhook(ctx context.Context, payload []byte) (*core.PaymentWebhook, error)
}

// OrderRepositoryHandler defines the interface for order repository
type OrderRepositoryHandler interface {
	UpdateStatus(ctx context.Context, id string, status core.OrderStatus) error
	GetOrderByID(ctx context.Context, id string) (*core.Order, error)
}

// WhatsAppGatewayHandler defines the interface for WhatsApp gateway
type WhatsAppGatewayHandler interface {
	SendText(ctx context.Context, phone string, message string) error
}


// BotServiceHandler defines the interface for bot service
type BotServiceHandler interface {
	HandleIncomingMessage(phone string, message string, messageType string) error
}

// NewHandler creates a new HTTP handler
func NewHandler(botService BotServiceHandler, paymentGateway PaymentGatewayHandler, orderRepo OrderRepositoryHandler, whatsappGateway WhatsAppGatewayHandler) *Handler {
	cfg := config.Get()
	return &Handler{
		verifyToken:    cfg.WhatsAppVerifyToken,
		appSecret:      "", // TODO: Add APP_SECRET to config if available
		botService:     botService,
		paymentGateway: paymentGateway,
		orderRepo:      orderRepo,
		whatsappGateway: whatsappGateway,
	}
}

// VerifyWebhook handles GET requests for webhook verification
func (h *Handler) VerifyWebhook(c *fiber.Ctx) error {
	mode := c.Query("hub.mode")
	token := c.Query("hub.verify_token")
	challenge := c.Query("hub.challenge")

	if mode != "subscribe" {
		return c.Status(http.StatusBadRequest).SendString("Invalid mode")
	}

	if token != h.verifyToken {
		return c.Status(http.StatusForbidden).SendString("Invalid verify token")
	}

	// Return challenge as plain text (not JSON)
	return c.SendString(challenge)
}

// ReceiveMessage handles POST requests for incoming WhatsApp messages
func (h *Handler) ReceiveMessage(c *fiber.Ctx) error {
	// Verify X-Hub-Signature-256 if app secret is configured
	if h.appSecret != "" {
		signature := c.Get("X-Hub-Signature-256")
		if signature == "" {
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
				"error": "Missing signature",
			})
		}

		body := c.Body()
		if !h.verifySignature(signature, body) {
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid signature",
			})
		}
	} else {
		// TODO: Implement proper HMAC-SHA256 verification when APP_SECRET is available
		// For now, we skip verification if app secret is not set
	}

	var payload whatsapp.WebhookPayload
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid payload",
		})
	}

	// Process each entry
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if change.Field != "messages" {
				continue
			}

			value := change.Value
			for _, msg := range value.Messages {
				phone := msg.From
				messageType := msg.Type

				var messageText string
				var interactiveID string

				switch messageType {
				case "text":
					messageText = msg.Text.Body
				case "interactive":
					if msg.Interactive.Type == "button_reply" {
						interactiveID = msg.Interactive.ButtonReply.ID
						messageText = msg.Interactive.ButtonReply.Title
					} else if msg.Interactive.Type == "list_reply" {
						interactiveID = msg.Interactive.ListReply.ID
						messageText = msg.Interactive.ListReply.Title
					}
				default:
					// Unsupported message type
					continue
				}

				// Use interactive ID if available, otherwise use message text
				messageToProcess := interactiveID
				if messageToProcess == "" {
					messageToProcess = messageText
				}

				// Handle message asynchronously (fire and forget for webhook response)
				go func(phoneNum, msgText, msgType string) {
					if err := h.botService.HandleIncomingMessage(phoneNum, msgText, msgType); err != nil {
						// Log error (in production, use proper logging)
						fmt.Printf("Error handling message: %v\n", err)
					}
				}(phone, messageToProcess, messageType)
			}
		}
	}

	// Return 200 OK immediately (WhatsApp requires quick response)
	return c.Status(http.StatusOK).JSON(fiber.Map{
		"status": "ok",
	})
}

// verifySignature verifies the X-Hub-Signature-256 header using HMAC-SHA256
func (h *Handler) verifySignature(signature string, body []byte) bool {
	// Signature format: sha256=<hex_string>
	parts := strings.Split(signature, "=")
	if len(parts) != 2 || parts[0] != "sha256" {
		return false
	}

	expectedSig, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(h.appSecret))
	mac.Write(body)
	computedSig := mac.Sum(nil)

	return hmac.Equal(expectedSig, computedSig)
}

// HandlePaymentWebhook handles POST requests for Kopo Kopo payment webhooks
func (h *Handler) HandlePaymentWebhook(c *fiber.Ctx) error {
	ctx := c.Context()

	// Verify X-KopoKopo-Signature header
	signature := c.Get("X-KopoKopo-Signature")
	if signature == "" {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing signature",
		})
	}

	body := c.Body()
	if !h.paymentGateway.VerifyWebhook(ctx, signature, body) {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid signature",
		})
	}

	// Process webhook
	result, err := h.paymentGateway.ProcessWebhook(ctx, body)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Failed to process webhook",
		})
	}

	// If payment successful, update order status
	if result.Success && result.OrderID != "" {
		// Update order status to PAID
		if err := h.orderRepo.UpdateStatus(ctx, result.OrderID, core.OrderStatusPaid); err != nil {
			// Log error but don't fail the webhook (idempotency)
			fmt.Printf("Error updating order status: %v\n", err)
		} else {
			// Get order to find customer phone
			order, err := h.orderRepo.GetOrderByID(ctx, result.OrderID)
			if err == nil && order != nil {
				// Send WhatsApp notification to user
				message := fmt.Sprintf("✅ Payment Received! Your order #%s (KES %.0f) has been confirmed. Your drinks are coming! 🍹", 
					result.OrderID[:8], order.TotalAmount)
				go func(phone, msg string) {
					if err := h.whatsappGateway.SendText(ctx, phone, msg); err != nil {
						fmt.Printf("Error sending payment confirmation: %v\n", err)
					}
				}(order.CustomerPhone, message)
			}
		}
	}

	// Return 200 OK (Kopo Kopo expects quick response)
	return c.Status(http.StatusOK).JSON(fiber.Map{
		"status": "ok",
	})
}
