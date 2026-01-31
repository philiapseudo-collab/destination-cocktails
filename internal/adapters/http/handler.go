package http

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"strings"

	"github.com/dumu-tech/destination-cocktails/internal/adapters/whatsapp"
	"github.com/dumu-tech/destination-cocktails/internal/config"
	"github.com/dumu-tech/destination-cocktails/internal/core"
	"github.com/dumu-tech/destination-cocktails/internal/events"
	"github.com/gofiber/fiber/v2"
)

// Handler handles HTTP requests for WhatsApp webhooks and payment webhooks
type Handler struct {
	verifyToken     string
	appSecret       string
	botService      BotServiceHandler
	paymentGateway  PaymentGatewayHandler
	orderRepo       OrderRepositoryHandler
	whatsappGateway WhatsAppGatewayHandler
	eventBus        *events.EventBus
}

// PaymentGatewayHandler defines the interface for payment gateway
type PaymentGatewayHandler interface {
	VerifyWebhook(ctx context.Context, signature string, payload []byte) bool
	ProcessWebhook(ctx context.Context, payload []byte) (*core.PaymentWebhook, error)
}

// OrderRepositoryHandler defines the interface for order repository
type OrderRepositoryHandler interface {
	UpdateStatus(ctx context.Context, id string, status core.OrderStatus) error
	GetByID(ctx context.Context, id string) (*core.Order, error)
	FindPendingByPhoneAndAmount(ctx context.Context, phone string, amount float64) (*core.Order, error)
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
	verifyToken := strings.TrimSpace(cfg.WhatsAppVerifyToken)

	if verifyToken == "" {
		log.Printf("WARNING: WHATSAPP_VERIFY_TOKEN is not set or empty!")
	}

	fmt.Printf("Handler initialized - Verify token length: %d, starts with: %s\n",
		len(verifyToken),
		maskToken(verifyToken))

	return &Handler{
		verifyToken:     verifyToken,
		appSecret:       "", // TODO: Add APP_SECRET to config if available
		botService:      botService,
		paymentGateway:  paymentGateway,
		orderRepo:       orderRepo,
		whatsappGateway: whatsappGateway,
		eventBus:        nil, // Will be set via SetEventBus
	}
}

// SetEventBus sets the event bus for real-time event emission
func (h *Handler) SetEventBus(eventBus *events.EventBus) {
	h.eventBus = eventBus
}

// VerifyWebhook handles GET requests for webhook verification
func (h *Handler) VerifyWebhook(c *fiber.Ctx) error {
	mode := c.Query("hub.mode")
	token := c.Query("hub.verify_token")
	challenge := c.Query("hub.challenge")

	// Log for debugging
	log.Printf("Webhook verification request received:")
	log.Printf("  Mode: %s", mode)
	log.Printf("  Token provided: %s (length: %d)", maskToken(token), len(token))
	log.Printf("  Token expected: %s (length: %d)", maskToken(h.verifyToken), len(h.verifyToken))
	log.Printf("  Challenge: %s", challenge)

	if mode != "subscribe" {
		log.Printf("Webhook verification FAILED: invalid mode '%s' (expected 'subscribe')", mode)
		return c.Status(http.StatusBadRequest).SendString("Invalid mode")
	}

	// Trim whitespace from both tokens for comparison
	providedToken := strings.TrimSpace(token)
	expectedToken := strings.TrimSpace(h.verifyToken)

	if providedToken != expectedToken {
		log.Printf("Webhook verification FAILED: token mismatch")
		log.Printf("  Provided: [%s] (bytes: %v)", providedToken, []byte(providedToken))
		log.Printf("  Expected: [%s] (bytes: %v)", expectedToken, []byte(expectedToken))
		return c.Status(http.StatusForbidden).SendString("Invalid verify token")
	}

	log.Println("Webhook verification SUCCESSFUL - returning challenge")
	// Return challenge as plain text (not JSON) - this is what WhatsApp expects
	return c.SendString(challenge)
}

// maskToken masks a token for logging (shows first 3 and last 3 chars)
func maskToken(token string) string {
	if token == "" {
		return "<empty>"
	}
	if len(token) <= 6 {
		return "***"
	}
	return token[:3] + "***" + token[len(token)-3:]
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

				// Check if this is a "Mark Done" button from bar staff
				if strings.HasPrefix(messageToProcess, "complete_") {
					orderID := strings.TrimPrefix(messageToProcess, "complete_")
					go h.handleOrderCompletion(c.Context(), phone, orderID)
					continue
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

	// Handle payment status
	if result.Success {
		var order *core.Order
		var err error
		
		// Strategy 1: Use OrderID if available (from incoming_payment webhook)
		if result.OrderID != "" {
			order, err = h.orderRepo.GetByID(ctx, result.OrderID)
			if err != nil {
				fmt.Printf("Error finding order by ID %s: %v\n", result.OrderID, err)
			} else if order != nil {
				fmt.Printf("[DEBUG] Found order by ID: %s (status: %s)\n", order.ID, order.Status)
			}
		}
		
		// Strategy 2: Fallback to phone + amount matching
		if order == nil && result.Phone != "" && result.Amount > 0 {
			order, err = h.orderRepo.FindPendingByPhoneAndAmount(ctx, result.Phone, result.Amount)
			if err != nil {
				fmt.Printf("Error finding order by phone+amount: %v\n", err)
			}
		}
		
		// If no order found, log as orphaned payment
		if order == nil {
			slog.Warn("Orphaned Payment Received - No matching order found",
				"order_id", result.OrderID,
				"amount", result.Amount,
				"phone", result.Phone,
				"reference", result.Reference,
				"status", result.Status)
			
			// Return 200 OK anyway (don't fail the webhook)
			return c.Status(http.StatusOK).JSON(fiber.Map{
				"status": "ok",
				"note":   "payment received but no matching order",
			})
		}
		
		// Update order status to PAID
		if err := h.orderRepo.UpdateStatus(ctx, order.ID, core.OrderStatusPaid); err != nil {
			// Log error but don't fail the webhook (idempotency)
			fmt.Printf("Error updating order status: %v\n", err)
		} else {
			// Send WhatsApp notification to customer with pickup code
			message := fmt.Sprintf("✅ *Payment Received!*\n\n"+
				"Your order has been confirmed 🍹\n\n"+
				"*Pickup Code:* %s\n"+
				"*Total:* KES %.0f\n\n"+
				"Show this code to the bartender when collecting your drinks!\n\n"+
				"_Type 'Menu' to order more._",
				order.PickupCode, order.TotalAmount)
			go func(phone, msg string) {
				if err := h.whatsappGateway.SendText(ctx, phone, msg); err != nil {
					fmt.Printf("Error sending payment confirmation: %v\n", err)
				}
			}(order.CustomerPhone, message)

			// Send notification to bar staff
			go h.notifyBarStaff(ctx, order)

			// Emit new_order event for dashboard SSE
			if h.eventBus != nil {
				h.eventBus.PublishNewOrder(order)
			}
		}
	} else {
		// Payment failed or cancelled
		fmt.Printf("[DEBUG] Payment failed/cancelled - OrderID: %s, Status: %s\n", result.OrderID, result.Status)
		
		var order *core.Order
		var err error
		
		// Try to find order by ID first (from incoming_payment webhook)
		if result.OrderID != "" {
			order, err = h.orderRepo.GetByID(ctx, result.OrderID)
			if err != nil {
				fmt.Printf("Error finding failed order by ID: %v\n", err)
			}
		}
		
		// Fallback to phone + amount matching
		if order == nil && result.Phone != "" && result.Amount > 0 {
			order, err = h.orderRepo.FindPendingByPhoneAndAmount(ctx, result.Phone, result.Amount)
			if err != nil {
				fmt.Printf("Error finding failed order by phone+amount: %v\n", err)
			}
		}
		
		if order != nil {
			if err := h.orderRepo.UpdateStatus(ctx, order.ID, core.OrderStatusFailed); err != nil {
				fmt.Printf("Error updating order status to FAILED: %v\n", err)
			} else {
				// Notify customer of payment failure
				message := fmt.Sprintf("❌ Payment failed for order #%s. Please try again by sending 'hi' to restart.",
					order.ID[:8])
				go func(phone, msg string) {
					if err := h.whatsappGateway.SendText(ctx, phone, msg); err != nil {
						fmt.Printf("Error sending payment failure notification: %v\n", err)
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

// notifyBarStaff sends a WhatsApp notification to bar staff with order details
func (h *Handler) notifyBarStaff(ctx context.Context, order *core.Order) {
	cfg := config.Get()
	barStaffPhone := cfg.BarStaffPhone

	if barStaffPhone == "" {
		log.Println("BAR_STAFF_PHONE not configured, skipping bar staff notification")
		return
	}

	// Build message with pickup code and items
	message := fmt.Sprintf("🔔 *New Order #%s*\n\n", order.PickupCode)
	message += "📦 *Items:*\n"

	for _, item := range order.Items {
		// Note: Item names should be populated by the repository
		message += fmt.Sprintf("• %d x Item\n", item.Quantity)
	}

	message += fmt.Sprintf("\n💰 Total: KES %.0f\n", order.TotalAmount)
	message += fmt.Sprintf("📱 Customer: %s\n", order.CustomerPhone)

	// Send with "Mark Done" button
	buttons := []core.Button{
		{
			ID:    fmt.Sprintf("complete_%s", order.ID),
			Title: "Mark Done",
		},
	}

	// Use WhatsAppGateway interface which has SendMenuButtons
	if gateway, ok := h.whatsappGateway.(core.WhatsAppGateway); ok {
		if err := gateway.SendMenuButtons(ctx, barStaffPhone, message, buttons); err != nil {
			log.Printf("Error sending bar staff notification: %v", err)
		}
	}
}

// handleOrderCompletion handles the "Mark Done" button callback from bar staff
func (h *Handler) handleOrderCompletion(ctx context.Context, barStaffPhone string, orderID string) {
	// Get order to check current status
	order, err := h.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		log.Printf("Error fetching order for completion: %v", err)
		h.whatsappGateway.SendText(ctx, barStaffPhone, "❌ Order not found")
		return
	}

	// Prevent double-clicking
	if order.Status == core.OrderStatusCompleted {
		h.whatsappGateway.SendText(ctx, barStaffPhone, "ℹ️ Order already marked as completed")
		return
	}

	// Update status to COMPLETED
	if err := h.orderRepo.UpdateStatus(ctx, orderID, core.OrderStatusCompleted); err != nil {
		log.Printf("Error updating order status to COMPLETED: %v", err)
		h.whatsappGateway.SendText(ctx, barStaffPhone, "❌ Failed to update order status")
		return
	}

	// Send confirmation to bar staff
	confirmMsg := fmt.Sprintf("✅ Order #%s marked as served!", order.PickupCode)
	h.whatsappGateway.SendText(ctx, barStaffPhone, confirmMsg)

	// Emit order_completed event for dashboard SSE
	if h.eventBus != nil {
		h.eventBus.PublishOrderCompleted(orderID)
	}

	log.Printf("Order %s (pickup: %s) marked as COMPLETED by bar staff", orderID, order.PickupCode)
}
