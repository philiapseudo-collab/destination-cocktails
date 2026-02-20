package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dumu-tech/destination-cocktails/internal/core"
	"github.com/google/uuid"
)

// BotService handles the bot state machine and message processing
type BotService struct {
	Repo      core.ProductRepository
	Session   core.SessionRepository
	WhatsApp  core.WhatsAppGateway
	Payment   core.PaymentGateway
	OrderRepo core.OrderRepository
	UserRepo  core.UserRepository
}

var fixedCategoryOrder = []string{
	"Cocktails",
	"Chasers",
	"Gin",
	"Whisky",
	"Spirits",
	"Vodka",
	"Brandy",
	"Rum",
	"Shots",
}

// State constants
const (
	StateStart                  = "START"
	StateBrowsing               = "BROWSING"
	StateSelectingProduct       = "SELECTING_PRODUCT"
	StateQuantity               = "QUANTITY"
	StateConfirmOrder           = "CONFIRM_ORDER"
	StateWaitingForPaymentPhone = "WAITING_FOR_PAYMENT_PHONE"
)

// NewBotService creates a new bot service
func NewBotService(repo core.ProductRepository, session core.SessionRepository, whatsapp core.WhatsAppGateway, payment core.PaymentGateway, orderRepo core.OrderRepository, userRepo core.UserRepository) *BotService {
	return &BotService{
		Repo:      repo,
		Session:   session,
		WhatsApp:  whatsapp,
		Payment:   payment,
		OrderRepo: orderRepo,
		UserRepo:  userRepo,
	}
}

// sortProductsAlphabetically sorts products by name (A-Z, case-insensitive)
func sortProductsAlphabetically(products []*core.Product) []*core.Product {
	sorted := make([]*core.Product, len(products))
	copy(sorted, products)
	sort.Slice(sorted, func(i, j int) bool {
		return strings.ToLower(sorted[i].Name) < strings.ToLower(sorted[j].Name)
	})
	return sorted
}

// buildOrderedCategories returns categories in fixed order and appends unknown ones after.
func buildOrderedCategories(menu map[string][]*core.Product) []string {
	categories := make([]string, 0, len(fixedCategoryOrder)+len(menu))
	seen := make(map[string]struct{}, len(fixedCategoryOrder)+len(menu))

	for _, category := range fixedCategoryOrder {
		categories = append(categories, category)
		seen[category] = struct{}{}
	}

	extraCategories := make([]string, 0, len(menu))
	for category := range menu {
		if _, exists := seen[category]; exists {
			continue
		}
		extraCategories = append(extraCategories, category)
	}

	sort.Slice(extraCategories, func(i, j int) bool {
		return strings.ToLower(extraCategories[i]) < strings.ToLower(extraCategories[j])
	})

	categories = append(categories, extraCategories...)

	// Limit to 10 categories (WhatsApp list limit)
	if len(categories) > 10 {
		categories = categories[:10]
	}

	return categories
}

func isCategoryInList(categories []string, target string) bool {
	for _, category := range categories {
		if category == target {
			return true
		}
	}
	return false
}

// HandleIncomingMessage processes incoming WhatsApp messages
func (b *BotService) HandleIncomingMessage(phone string, message string, messageType string) error {
	ctx := context.Background()

	// Global Reset Check: Check for reset keywords before processing state
	normalizedMessage := strings.ToLower(strings.TrimSpace(message))
	resetKeywords := []string{"hi", "hello", "start", "restart", "reset", "menu", "0"}

	for _, keyword := range resetKeywords {
		if normalizedMessage == keyword {
			// Create a completely fresh session
			newSession := &core.Session{
				State:            "START",
				Cart:             []core.CartItem{}, // Explicit empty slice
				CurrentCategory:  "",
				CurrentProductID: "",
			}

			// Save the fresh session to Redis
			if err := b.Session.Set(ctx, phone, newSession, 7200); err != nil {
				return fmt.Errorf("failed to reset session: %w", err)
			}

			// Call handleStart with empty string to show welcome (not search)
			return b.handleStart(ctx, phone, newSession, "")
		}
	}

	// Get or create session
	session, err := b.Session.Get(ctx, phone)
	if err != nil {
		// Session doesn't exist, create new one
		session = &core.Session{
			State: "START",
			Cart:  []core.CartItem{},
		}
		if err := b.Session.Set(ctx, phone, session, 7200); err != nil { // 2 hours TTL
			return fmt.Errorf("failed to create session: %w", err)
		}
	}

	// Handle Retry Payment button (from 15s timeout fallback)
	if strings.HasPrefix(normalizedMessage, "retry_pay_") {
		orderID := strings.TrimPrefix(message, "retry_pay_") // Use original case
		return b.handleRetryPayment(ctx, phone, session, orderID)
	}

	// Route based on state
	switch session.State {
	case "START", "":
		return b.handleStart(ctx, phone, session, message)
	case "MENU":
		return b.handleMenu(ctx, phone, session, message)
	case "BROWSING":
		return b.handleBrowsing(ctx, phone, session, message)
	case "SELECTING_PRODUCT":
		return b.handleSelectingProduct(ctx, phone, session, message)
	case "QUANTITY":
		return b.handleQuantity(ctx, phone, session, message)
	case "CONFIRM_ORDER":
		return b.handleConfirmOrder(ctx, phone, session, message)
	case StateWaitingForPaymentPhone:
		return b.handlePaymentPhoneInput(ctx, phone, session, message)
	default:
		// Unknown state, reset to START
		session.State = "START"
		b.Session.Set(ctx, phone, session, 7200)
		return b.handleStart(ctx, phone, session, message)
	}
}

// handleStart handles the START state - sends welcome message or processes search
func (b *BotService) handleStart(ctx context.Context, phone string, session *core.Session, message string) error {
	messageLower := strings.ToLower(strings.TrimSpace(message))

	// If message is empty (from reset command), show welcome with categories
	if messageLower == "" {
		// Get menu (grouped by category)
		menu, err := b.Repo.GetMenu(ctx)
		if err != nil {
			return fmt.Errorf("failed to get menu: %w", err)
		}

		categories := buildOrderedCategories(menu)

		// Send category list directly
		if err := b.WhatsApp.SendCategoryList(ctx, phone, categories); err != nil {
			return fmt.Errorf("failed to send categories: %w", err)
		}

		// Set state to BROWSING
		session.State = "BROWSING"
		return b.Session.Set(ctx, phone, session, 7200)
	}

	// If message is "order_drinks" button or contains "order", DIRECTLY show menu
	if messageLower == "order_drinks" || messageLower == "order drinks" || strings.Contains(messageLower, "order") {
		// Get menu (grouped by category)
		menu, err := b.Repo.GetMenu(ctx)
		if err != nil {
			return fmt.Errorf("failed to get menu: %w", err)
		}

		categories := buildOrderedCategories(menu)

		// Send category list directly (no welcome message needed)
		if err := b.WhatsApp.SendCategoryList(ctx, phone, categories); err != nil {
			return fmt.Errorf("failed to send categories: %w", err)
		}

		// Set state to BROWSING (skip MENU state)
		session.State = "BROWSING"
		return b.Session.Set(ctx, phone, session, 7200)
	}

	// Otherwise, treat the message as a search query
	searchQuery := strings.TrimSpace(message)

	// Improved search: allow partial matches, handle multiple words
	products, err := b.Repo.SearchProducts(ctx, searchQuery)
	if err != nil {
		return fmt.Errorf("failed to search products: %w", err)
	}

	// If no results found, send error message and "Order Drinks" button
	if len(products) == 0 {
		noResultsMsg := fmt.Sprintf("❌ No products found for '%s'.\n\n💡 Try:\n• Typing just one word (e.g., 'Gin', 'Water')\n• Browsing the full menu below", searchQuery)
		buttons := []core.Button{
			{
				ID:    "order_drinks",
				Title: "View Full Menu",
			},
		}

		if err := b.WhatsApp.SendMenuButtons(ctx, phone, noResultsMsg, buttons); err != nil {
			return fmt.Errorf("failed to send no results message: %w", err)
		}

		// Stay in START state
		return b.Session.Set(ctx, phone, session, 7200)
	}

	// Sort products alphabetically
	sortedProducts := sortProductsAlphabetically(products)

	// Build formatted text message with numbered list
	productList := fmt.Sprintf("🔍 Search results for '*%s*':\n\n", searchQuery)
	for i, product := range sortedProducts {
		productList += fmt.Sprintf("%d. %s - KES %.0f\n", i+1, product.Name, product.Price)
	}
	productList += "\nReply with the number or name to add to cart."

	// Send product list as text message
	if err := b.WhatsApp.SendText(ctx, phone, productList); err != nil {
		return fmt.Errorf("failed to send search results: %w", err)
	}

	// Set a pseudo-category for search results so SELECTING_PRODUCT state can work
	// We'll use a special category name that includes all search results
	session.CurrentCategory = "_SEARCH_" + searchQuery
	session.State = "SELECTING_PRODUCT"
	return b.Session.Set(ctx, phone, session, 7200)
}

// handleMenu handles the MENU state - shows categories
func (b *BotService) handleMenu(ctx context.Context, phone string, session *core.Session, message string) error {
	messageLower := strings.ToLower(strings.TrimSpace(message))

	// Accept button ID or text containing "order"
	if messageLower != "order_drinks" && messageLower != "order drinks" && !strings.Contains(messageLower, "order") {
		// Invalid input - resend the category list
		menu, err := b.Repo.GetMenu(ctx)
		if err != nil {
			return fmt.Errorf("failed to get menu: %w", err)
		}

		categories := buildOrderedCategories(menu)

		errorMsg := "That menu is expired. Here is the latest one."
		// Send error message first, then the list
		if err := b.WhatsApp.SendText(ctx, phone, errorMsg); err != nil {
			return fmt.Errorf("failed to send error message: %w", err)
		}

		if err := b.WhatsApp.SendCategoryList(ctx, phone, categories); err != nil {
			return fmt.Errorf("failed to send categories: %w", err)
		}

		// Set state to BROWSING
		session.State = "BROWSING"
		return b.Session.Set(ctx, phone, session, 7200)
	}

	// Get menu (grouped by category)
	menu, err := b.Repo.GetMenu(ctx)
	if err != nil {
		return fmt.Errorf("failed to get menu: %w", err)
	}

	categories := buildOrderedCategories(menu)

	// Send category list using interactive list
	if err := b.WhatsApp.SendCategoryList(ctx, phone, categories); err != nil {
		return fmt.Errorf("failed to send categories: %w", err)
	}

	// Set state to BROWSING
	session.State = "BROWSING"
	return b.Session.Set(ctx, phone, session, 7200)
}

// handleBrowsing handles the BROWSING state - shows products in a category
func (b *BotService) handleBrowsing(ctx context.Context, phone string, session *core.Session, message string) error {
	// Get menu (grouped by category)
	menu, err := b.Repo.GetMenu(ctx)
	if err != nil {
		return fmt.Errorf("failed to get menu: %w", err)
	}

	// Trust the category ID from list_reply (exact match)
	selectedCategory := strings.TrimSpace(message)

	orderedCategories := buildOrderedCategories(menu)
	if !isCategoryInList(orderedCategories, selectedCategory) {
		// Invalid category - resend the category list
		categories := orderedCategories

		errorMsg := "That menu is expired. Here is the latest one."
		// Send error message first, then the list
		if err := b.WhatsApp.SendText(ctx, phone, errorMsg); err != nil {
			return fmt.Errorf("failed to send error message: %w", err)
		}

		if err := b.WhatsApp.SendCategoryList(ctx, phone, categories); err != nil {
			return fmt.Errorf("failed to send categories: %w", err)
		}

		// Keep state as BROWSING
		return b.Session.Set(ctx, phone, session, 7200)
	}

	// Category is valid in UI order; it may still have no active products in DB.
	products := menu[selectedCategory]

	// Get products for this category
	if len(products) == 0 {
		return b.WhatsApp.SendText(ctx, phone, "No products available in this category.")
	}

	// Sort products alphabetically by name (A-Z)
	sortedProducts := sortProductsAlphabetically(products)

	// Build formatted text message with numbered list
	productList := fmt.Sprintf("Products in *%s*:\n\n", selectedCategory)
	for i, product := range sortedProducts {
		productList += fmt.Sprintf("%d. %s - KES %.0f\n", i+1, product.Name, product.Price)
	}
	productList += "\nReply with the product name or number to add to cart."

	// Send product list as text message
	if err := b.WhatsApp.SendText(ctx, phone, productList); err != nil {
		return fmt.Errorf("failed to send products: %w", err)
	}

	// Update session with current category
	session.CurrentCategory = selectedCategory
	session.State = "SELECTING_PRODUCT"
	return b.Session.Set(ctx, phone, session, 7200)
}

// handleSelectingProduct handles the SELECTING_PRODUCT state - user selects a product
func (b *BotService) handleSelectingProduct(ctx context.Context, phone string, session *core.Session, message string) error {
	var sortedProducts []*core.Product

	// Check if we're in search mode (category starts with "_SEARCH_")
	isSearchMode := strings.HasPrefix(session.CurrentCategory, "_SEARCH_")

	if isSearchMode {
		// Extract search query from category
		searchQuery := strings.TrimPrefix(session.CurrentCategory, "_SEARCH_")
		products, err := b.Repo.SearchProducts(ctx, searchQuery)
		if err != nil {
			return fmt.Errorf("failed to search products: %w", err)
		}
		if len(products) == 0 {
			return b.WhatsApp.SendText(ctx, phone, "No products available. Please search again.")
		}
		sortedProducts = sortProductsAlphabetically(products)
	} else {
		// Get products from current category (normal menu flow)
		menu, err := b.Repo.GetMenu(ctx)
		if err != nil {
			return fmt.Errorf("failed to get menu: %w", err)
		}

		products := menu[session.CurrentCategory]
		if len(products) == 0 {
			return b.WhatsApp.SendText(ctx, phone, "No products available. Please select another category.")
		}

		// Sort products alphabetically (same order as displayed in handleBrowsing)
		sortedProducts = sortProductsAlphabetically(products)
	}

	// Try to match by UUID first, then number or name
	var selectedProduct *core.Product
	messageTrimmed := strings.TrimSpace(message)
	messageLower := strings.ToLower(messageTrimmed)

	// Try UUID first (from interactive list reply - backward compatibility)
	if productID, err := uuid.Parse(messageTrimmed); err == nil {
		// Valid UUID - fetch product by ID
		product, err := b.Repo.GetByID(ctx, productID.String())
		if err == nil && product != nil {
			// Verify product is in current category (skip check for search mode)
			if isSearchMode {
				// For search mode, verify product is in the sorted list
				for _, p := range sortedProducts {
					if p.ID == product.ID {
						selectedProduct = product
						break
					}
				}
			} else {
				// For normal category mode, verify category matches
				if product.Category == session.CurrentCategory {
					selectedProduct = product
				}
			}
		}
	}

	// If not found by UUID, try number or name
	if selectedProduct == nil {
		// Try number first (map to sorted products)
		if num, err := strconv.Atoi(messageTrimmed); err == nil {
			if num > 0 && num <= len(sortedProducts) {
				selectedProduct = sortedProducts[num-1]
			}
		} else {
			// Try name match: exact match first, then partial match
			var exactMatch *core.Product
			var partialMatches []*core.Product

			for _, product := range sortedProducts {
				productNameLower := strings.ToLower(product.Name)
				// Exact match (case-insensitive)
				if productNameLower == messageLower {
					exactMatch = product
					break
				}
				// Partial match (contains)
				if strings.Contains(productNameLower, messageLower) {
					partialMatches = append(partialMatches, product)
				}
			}

			// Use exact match if found, otherwise use first partial match
			if exactMatch != nil {
				selectedProduct = exactMatch
			} else if len(partialMatches) > 0 {
				// If multiple partial matches, use the first one
				selectedProduct = partialMatches[0]
			}
		}
	}

	if selectedProduct == nil {
		// Invalid selection - send short error message (don't resend list)
		errorMsg := "Invalid option. Please reply with the number (e.g., '1') or the name of the drink."
		if err := b.WhatsApp.SendText(ctx, phone, errorMsg); err != nil {
			return fmt.Errorf("failed to send error message: %w", err)
		}

		// Keep state as SELECTING_PRODUCT
		return b.Session.Set(ctx, phone, session, 7200)
	}

	// Check stock
	if selectedProduct.StockQuantity <= 0 {
		return b.WhatsApp.SendText(ctx, phone, fmt.Sprintf("Sorry, %s is out of stock. Please select another product.", selectedProduct.Name))
	}

	// Store selected product
	session.CurrentProductID = selectedProduct.ID

	// Ask for quantity
	quantityMsg := fmt.Sprintf("You selected: *%s*\nPrice: KES %.0f\n\nHow many would you like? (Enter a number)",
		selectedProduct.Name, selectedProduct.Price)

	if err := b.WhatsApp.SendText(ctx, phone, quantityMsg); err != nil {
		return fmt.Errorf("failed to send quantity prompt: %w", err)
	}

	// Set state to QUANTITY
	session.State = "QUANTITY"
	return b.Session.Set(ctx, phone, session, 7200)
}

// handleQuantity handles the QUANTITY state - user enters quantity
func (b *BotService) handleQuantity(ctx context.Context, phone string, session *core.Session, message string) error {
	// Parse quantity
	quantity, err := strconv.Atoi(strings.TrimSpace(message))
	if err != nil || quantity <= 0 {
		// Invalid input - forgiving state: keep in QUANTITY
		return b.WhatsApp.SendText(ctx, phone, "Please enter a valid number (e.g., 2)")
	}

	// Get product details
	product, err := b.Repo.GetByID(ctx, session.CurrentProductID)
	if err != nil {
		return fmt.Errorf("failed to get product: %w", err)
	}

	// Check stock
	if product.StockQuantity < quantity {
		return b.WhatsApp.SendText(ctx, phone,
			fmt.Sprintf("Sorry, only %d available in stock. Please enter a smaller quantity.", product.StockQuantity))
	}

	// Add to cart
	cartItem := core.CartItem{
		ProductID: product.ID,
		Quantity:  quantity,
		Name:      product.Name,
		Price:     product.Price,
	}

	session.Cart = append(session.Cart, cartItem)

	// Calculate total
	total := 0.0
	for _, item := range session.Cart {
		total += item.Price * float64(item.Quantity)
	}

	// Build cart summary showing all items with prices before total
	cartSummary := "✅ Added to cart!\n\n📦 Your cart:\n"
	for _, item := range session.Cart {
		itemTotal := item.Price * float64(item.Quantity)
		cartSummary += fmt.Sprintf("%s x%d = KES %.0f\n", item.Name, item.Quantity, itemTotal)
	}
	cartSummary += fmt.Sprintf("\n💰 Cart total: KES %.0f", total)

	// Confirm addition with interactive buttons
	confirmMsg := cartSummary

	buttons := []core.Button{
		{
			ID:    "add_more",
			Title: "Add More",
		},
		{
			ID:    "checkout",
			Title: "Checkout",
		},
	}

	if err := b.WhatsApp.SendMenuButtons(ctx, phone, confirmMsg, buttons); err != nil {
		return fmt.Errorf("failed to send confirmation: %w", err)
	}

	// Set state to CONFIRM_ORDER
	session.State = "CONFIRM_ORDER"
	return b.Session.Set(ctx, phone, session, 7200)
}

// handleConfirmOrder handles the CONFIRM_ORDER state - user can add more or checkout
func (b *BotService) handleConfirmOrder(ctx context.Context, phone string, session *core.Session, message string) error {
	messageLower := strings.ToLower(strings.TrimSpace(message))

	// Check for button IDs first, then fallback to text matching for backward compatibility
	if messageLower == "add_more" || strings.Contains(messageLower, "add more") || strings.Contains(messageLower, "continue") {
		// Go back to categories
		return b.handleMenu(ctx, phone, session, "Order Drinks")
	}

	if messageLower == "checkout" || strings.Contains(messageLower, "checkout") {
		return b.handleCheckout(ctx, phone, session)
	}

	// Handle payment confirmation buttons (pay_self, pay_other)
	if messageLower == "pay_self" {
		return b.handlePaySelf(ctx, phone, session)
	}

	if messageLower == "pay_other" {
		return b.handlePayOther(ctx, phone, session)
	}

	// Invalid input - resend buttons
	confirmMsg := "Please select an option:"
	buttons := []core.Button{
		{
			ID:    "add_more",
			Title: "Add More",
		},
		{
			ID:    "checkout",
			Title: "Checkout",
		},
	}
	return b.WhatsApp.SendMenuButtons(ctx, phone, confirmMsg, buttons)
}

// generatePickupCode generates a random 4-digit pickup code
func generatePickupCode() string {
	return fmt.Sprintf("%04d", time.Now().UnixNano()%10000)
}

// handleCheckout initiates the checkout process by asking for payment number confirmation
func (b *BotService) handleCheckout(ctx context.Context, phone string, session *core.Session) error {
	// Validate cart
	if len(session.Cart) == 0 {
		return b.WhatsApp.SendText(ctx, phone, "Your cart is empty. Please add items first.")
	}

	// DUPLICATE CHECKOUT PREVENTION: Check if user has a pending order
	if session.PendingOrderID != "" {
		// Check if the order is still pending
		order, err := b.OrderRepo.GetByID(ctx, session.PendingOrderID)
		if err == nil && order != nil && order.Status == core.OrderStatusPending {
			// Order still pending - show helpful message with retry option
			msg := "⏳ *Payment Already Pending*\n\n" +
				"An M-Pesa prompt was already sent for your order.\n\n" +
				"*What to do:*\n" +
				"1. Check your phone for the M-Pesa prompt\n" +
				"2. Enter your PIN to complete payment\n" +
				"3. If you missed it, wait 30 seconds then try again\n\n" +
				"_If the prompt expired, type 'hi' to start fresh._"
			return b.WhatsApp.SendText(ctx, phone, msg)
		}
		// Order is no longer pending (paid, failed, or cancelled) - clear and continue
		session.PendingOrderID = ""
	}

	// Calculate total
	total := 0.0
	for _, item := range session.Cart {
		total += item.Price * float64(item.Quantity)
	}

	// Send button prompt asking which number to charge
	promptMsg := fmt.Sprintf("Your total is *KES %.0f*.\n\nWhich M-Pesa number should we charge?", total)

	buttons := []core.Button{
		{
			ID:    "pay_self",
			Title: "Use My Number",
		},
		{
			ID:    "pay_other",
			Title: "Different Number",
		},
	}

	if err := b.WhatsApp.SendMenuButtons(ctx, phone, promptMsg, buttons); err != nil {
		return fmt.Errorf("failed to send payment prompt: %w", err)
	}

	// Keep state as CONFIRM_ORDER (user will respond with button click)
	return b.Session.Set(ctx, phone, session, 7200)
}

// handlePaySelf handles when user chooses to use their own WhatsApp number
func (b *BotService) handlePaySelf(ctx context.Context, phone string, session *core.Session) error {
	// Use the WhatsApp phone number
	return b.processPayment(ctx, phone, session, phone)
}

// handlePayOther handles when user chooses to use a different number
func (b *BotService) handlePayOther(ctx context.Context, phone string, session *core.Session) error {
	// Prompt for phone number
	promptMsg := "Please type the Safaricom M-Pesa number you want to use (e.g., 0712345678)."

	if err := b.WhatsApp.SendText(ctx, phone, promptMsg); err != nil {
		return fmt.Errorf("failed to send phone prompt: %w", err)
	}

	// Set state to wait for phone input
	session.State = StateWaitingForPaymentPhone
	return b.Session.Set(ctx, phone, session, 7200)
}

// handlePaymentPhoneInput handles user input when waiting for alternative payment phone
func (b *BotService) handlePaymentPhoneInput(ctx context.Context, phone string, session *core.Session, message string) error {
	// Normalize and validate the phone number
	normalizedPhone, err := normalizePhone(message)
	if err != nil || !isValidKenyanMobile(normalizedPhone) {
		// Invalid phone number - ask to try again (keep state)
		errorMsg := "That doesn't look like a valid phone number. Please try again (e.g., 0712345678)."
		return b.WhatsApp.SendText(ctx, phone, errorMsg)
	}

	// Process payment with the normalized phone
	return b.processPayment(ctx, phone, session, normalizedPhone)
}

// handleRetryPayment handles the Retry Payment button click from the 15s timeout fallback
// It re-initiates STK push for an existing PENDING order (SILENT - no WhatsApp message)
func (b *BotService) handleRetryPayment(ctx context.Context, whatsappPhone string, session *core.Session, orderID string) error {
	// Fetch the existing order
	order, err := b.OrderRepo.GetByID(ctx, orderID)
	if err != nil {
		b.WhatsApp.SendText(ctx, whatsappPhone, "Order not found. Please start a new order.")
		return nil
	}

	// Check if order is still PENDING (payment not yet completed)
	if order.Status != core.OrderStatusPending {
		b.WhatsApp.SendText(ctx, whatsappPhone, "This order has already been processed.")
		return nil
	}

	// Re-initiate STK Push to the payment phone (SILENT - no confirmation message)
	err = b.Payment.InitiateSTKPush(ctx, orderID, order.CustomerPhone, order.TotalAmount)
	if err != nil {
		// Send error message - safe because no STK push was sent
		b.WhatsApp.SendText(ctx, whatsappPhone, "⚠️ Payment system busy. Please try again in a moment.")
		return nil
	}

	// SAFETY NET: Launch goroutine to check order status after 45 seconds
	// Note: M-Pesa STK prompts can take 20-40 seconds to arrive, so we wait longer
	go func(oID string, waPhone string) {
		time.Sleep(45 * time.Second)

		checkCtx := context.Background()
		order, err := b.OrderRepo.GetByID(checkCtx, oID)
		if err != nil {
			return
		}

		if order.Status == core.OrderStatusPending {
			// Order still pending - send retry button again
			timeoutMsg := "⏳ *Waiting for M-Pesa*\n\n" +
				"The payment prompt can take up to 60 seconds to appear.\n\n" +
				"*If it hasn't appeared yet:*\n" +
				"• Check your phone for the M-Pesa prompt\n" +
				"• Make sure you have network signal\n" +
				"• Tap 'Retry' below if needed\n\n" +
				"_If you already completed payment, please wait for confirmation._"
			buttons := []core.Button{
				{
					ID:    "retry_pay_" + oID,
					Title: "Retry Payment",
				},
			}
			b.WhatsApp.SendMenuButtons(checkCtx, waPhone, timeoutMsg, buttons)
		}
	}(orderID, whatsappPhone)

	return nil
}

// processPayment creates the order and initiates STK push
// SILENT CHECKOUT: No WhatsApp messages are sent during STK push to prevent iPhone UI freeze
func (b *BotService) processPayment(ctx context.Context, whatsappPhone string, session *core.Session, paymentPhone string) error {
	// Calculate total
	total := 0.0
	for _, item := range session.Cart {
		total += item.Price * float64(item.Quantity)
	}

	// Upsert user (Get or Create) using WhatsApp phone
	user, err := b.UserRepo.GetOrCreateByPhone(ctx, whatsappPhone)
	if err != nil {
		return fmt.Errorf("failed to get or create user: %w", err)
	}

	// Generate order ID
	orderID := uuid.New().String()

	// Generate 4-digit pickup code
	pickupCode := generatePickupCode()

	// Create order items from cart
	orderItems := make([]core.OrderItem, len(session.Cart))
	for i, cartItem := range session.Cart {
		orderItems[i] = core.OrderItem{
			ID:          uuid.New().String(),
			OrderID:     orderID,
			ProductID:   cartItem.ProductID,
			Quantity:    cartItem.Quantity,
			PriceAtTime: cartItem.Price,
		}
	}

	// Create order with PENDING status
	// CRITICAL: Use paymentPhone for CustomerPhone (for webhook matching)
	order := &core.Order{
		ID:            orderID,
		UserID:        user.ID,
		CustomerPhone: paymentPhone, // Use payment phone for webhook matching
		TableNumber:   "",           // TODO: Ask for table number or get from session
		TotalAmount:   total,
		Status:        core.OrderStatusPending,
		PaymentMethod: string(core.PaymentMethodMpesa),
		PickupCode:    pickupCode,
		Items:         orderItems,
		CreatedAt:     time.Now(),
	}

	if err := b.OrderRepo.CreateOrder(ctx, order); err != nil {
		return fmt.Errorf("failed to create order: %w", err)
	}

	// CRITICAL: Store pending order ID in session for duplicate checkout prevention
	session.PendingOrderID = orderID

	// Initiate STK Push to the payment phone
	// SILENT MODE: No success message is sent - this prevents iPhone UI freeze
	err = b.Payment.InitiateSTKPush(ctx, orderID, paymentPhone, total)
	if err != nil {
		// If queueing fails (system busy), update order status to FAILED and clear pending ID
		b.OrderRepo.UpdateStatus(ctx, orderID, core.OrderStatusFailed)
		session.PendingOrderID = ""
		b.Session.Set(ctx, whatsappPhone, session, 7200)
		// Send error message - safe because no STK push was sent to freeze the phone
		b.WhatsApp.SendText(ctx, whatsappPhone, "⚠️ Payment system busy. Please try again in a moment.")
		return fmt.Errorf("failed to initiate STK push: %w", err)
	}

	// Clear cart and reset state, but KEEP PendingOrderID until payment is processed
	session.Cart = []core.CartItem{}
	session.State = "START"
	b.Session.Set(ctx, whatsappPhone, session, 7200)

	// SAFETY NET: Launch goroutine to check order status after 45 seconds
	// If order is still PENDING, send a Retry button to the user
	// Note: M-Pesa STK prompts can take 20-40 seconds to arrive, so we wait longer
	go func(oID string, waPhone string, payPhone string) {
		time.Sleep(45 * time.Second)

		// Check if order is still PENDING
		checkCtx := context.Background()
		order, err := b.OrderRepo.GetByID(checkCtx, oID)
		if err != nil {
			return // Order not found or error, skip
		}

		if order.Status == core.OrderStatusPending {
			// Order still pending after 45 seconds - send retry button
			timeoutMsg := "⏳ *Waiting for M-Pesa*\n\n" +
				"The payment prompt can take up to 60 seconds to appear.\n\n" +
				"*If it hasn't appeared yet:*\n" +
				"• Check your phone for the M-Pesa prompt\n" +
				"• Make sure you have network signal\n" +
				"• Tap 'Retry' below if needed\n\n" +
				"_If you already completed payment, please wait for confirmation._"
			buttons := []core.Button{
				{
					ID:    "retry_pay_" + oID,
					Title: "Retry Payment",
				},
			}
			b.WhatsApp.SendMenuButtons(checkCtx, waPhone, timeoutMsg, buttons)
		}
	}(orderID, whatsappPhone, paymentPhone)

	return nil
}

// normalizePhone normalizes a Kenyan phone number to +254xxxxxxxxx format
// Supports: 07..., 01..., 254..., +254..., 7..., 1...
func normalizePhone(phone string) (string, error) {
	// Remove spaces and dashes
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	phone = strings.TrimSpace(phone)

	// Remove leading +
	phone = strings.TrimPrefix(phone, "+")

	// Handle different formats
	if strings.HasPrefix(phone, "254") {
		// Already in 254xxxxxxxxx format
		if len(phone) == 12 {
			return "+" + phone, nil
		}
		return "", fmt.Errorf("invalid phone number format")
	} else if strings.HasPrefix(phone, "07") || strings.HasPrefix(phone, "01") {
		// 07xxxxxxxx or 01xxxxxxxx -> +2547xxxxxxxx or +2541xxxxxxxx
		if len(phone) == 10 {
			return "+254" + phone[1:], nil
		}
		return "", fmt.Errorf("invalid phone number format")
	} else if strings.HasPrefix(phone, "7") || strings.HasPrefix(phone, "1") {
		// 7xxxxxxxx or 1xxxxxxxx -> +2547xxxxxxxx or +2541xxxxxxxx
		if len(phone) == 9 {
			return "+254" + phone, nil
		}
		return "", fmt.Errorf("invalid phone number format")
	}

	return "", fmt.Errorf("unsupported phone number format")
}

// isValidKenyanMobile validates that a normalized phone starts with +2547 or +2541
func isValidKenyanMobile(normalizedPhone string) bool {
	return strings.HasPrefix(normalizedPhone, "+2547") || strings.HasPrefix(normalizedPhone, "+2541")
}
