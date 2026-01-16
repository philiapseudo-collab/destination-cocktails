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
	Repo          core.ProductRepository
	Session       core.SessionRepository
	WhatsApp      core.WhatsAppGateway
	Payment       core.PaymentGateway
	OrderRepo     core.OrderRepository
	UserRepo      core.UserRepository
}

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

// HandleIncomingMessage processes incoming WhatsApp messages
func (b *BotService) HandleIncomingMessage(phone string, message string, messageType string) error {
	ctx := context.Background()

	// Global Reset Check: Check for reset keywords before processing state
	normalizedMessage := strings.ToLower(strings.TrimSpace(message))
	resetKeywords := []string{"hi", "hello", "start", "restart", "reset", "menu"}
	
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
			
			// Call handleStart to send welcome message and return early
			return b.handleStart(ctx, phone, newSession, message)
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
	default:
		// Unknown state, reset to START
		session.State = "START"
		b.Session.Set(ctx, phone, session, 7200)
		return b.handleStart(ctx, phone, session, message)
	}
}

// handleStart handles the START state - sends welcome message
func (b *BotService) handleStart(ctx context.Context, phone string, session *core.Session, message string) error {
	welcomeText := "Welcome to Destination Cocktails! 🍹"
	buttons := []core.Button{
		{
			ID:    "order_drinks",
			Title: "Order Drinks",
		},
	}

	if err := b.WhatsApp.SendMenuButtons(ctx, phone, welcomeText, buttons); err != nil {
		return fmt.Errorf("failed to send welcome message: %w", err)
	}

	// Set state to MENU
	session.State = "MENU"
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
		
		categories := make([]string, 0, len(menu))
		for category := range menu {
			categories = append(categories, category)
		}
		
		// Limit to 10 categories (WhatsApp list limit)
		if len(categories) > 10 {
			categories = categories[:10]
		}
		
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

	// Extract category names
	categories := make([]string, 0, len(menu))
	for category := range menu {
		categories = append(categories, category)
	}

	// Limit to 10 categories (WhatsApp list limit)
	if len(categories) > 10 {
		categories = categories[:10]
	}

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
	
	// Check if category exists in menu
	products, categoryExists := menu[selectedCategory]
	if !categoryExists {
		// Invalid category - resend the category list
		categories := make([]string, 0, len(menu))
		for category := range menu {
			categories = append(categories, category)
		}
		
		// Limit to 10 categories (WhatsApp list limit)
		if len(categories) > 10 {
			categories = categories[:10]
		}
		
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
	// Get products from current category
	menu, err := b.Repo.GetMenu(ctx)
	if err != nil {
		return fmt.Errorf("failed to get menu: %w", err)
	}

	products := menu[session.CurrentCategory]
	if len(products) == 0 {
		return b.WhatsApp.SendText(ctx, phone, "No products available. Please select another category.")
	}

	// Sort products alphabetically (same order as displayed in handleBrowsing)
	sortedProducts := sortProductsAlphabetically(products)

	// Try to match by UUID first, then number or name
	var selectedProduct *core.Product
	messageTrimmed := strings.TrimSpace(message)
	messageLower := strings.ToLower(messageTrimmed)

	// Try UUID first (from interactive list reply - backward compatibility)
	if productID, err := uuid.Parse(messageTrimmed); err == nil {
		// Valid UUID - fetch product by ID
		product, err := b.Repo.GetByID(ctx, productID.String())
		if err == nil && product != nil {
			// Verify product is in current category
			if product.Category == session.CurrentCategory {
				selectedProduct = product
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

	// Confirm addition with interactive buttons
	confirmMsg := fmt.Sprintf("✅ Added to cart:\n%s x%d = KES %.0f\n\nCart total: KES %.0f",
		product.Name, quantity, product.Price*float64(quantity), total)

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
		// Checkout flow: Create order -> Initiate STK Push
		if len(session.Cart) == 0 {
			return b.WhatsApp.SendText(ctx, phone, "Your cart is empty. Please add items first.")
		}

		// Calculate total
		total := 0.0
		for _, item := range session.Cart {
			total += item.Price * float64(item.Quantity)
		}

		// Upsert user (Get or Create)
		user, err := b.UserRepo.GetOrCreateByPhone(ctx, phone)
		if err != nil {
			return fmt.Errorf("failed to get or create user: %w", err)
		}

		// Generate order ID
		orderID := uuid.New().String()

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
		order := &core.Order{
			ID:            orderID,
			UserID:        user.ID,
			CustomerPhone: phone,
			TableNumber:   "", // TODO: Ask for table number or get from session
			TotalAmount:   total,
			Status:        core.OrderStatusPending,
			PaymentMethod: string(core.PaymentMethodMpesa),
			Items:         orderItems,
			CreatedAt:     time.Now(),
		}

		if err := b.OrderRepo.CreateOrder(ctx, order); err != nil {
			return fmt.Errorf("failed to create order: %w", err)
		}

		// Initiate STK Push
		_, err = b.Payment.InitiateSTKPush(ctx, phone, total, orderID)
		if err != nil {
			// If STK push fails, update order status to FAILED
			b.OrderRepo.UpdateStatus(ctx, orderID, core.OrderStatusFailed)
			return fmt.Errorf("failed to initiate STK push: %w", err)
		}

		// Send confirmation message
		cartSummary := "📦 Order Summary:\n\n"
		for _, item := range session.Cart {
			itemTotal := item.Price * float64(item.Quantity)
			cartSummary += fmt.Sprintf("%s x%d = KES %.0f\n", item.Name, item.Quantity, itemTotal)
		}
		cartSummary += fmt.Sprintf("\n💰 Total: KES %.0f\n\n", total)
		cartSummary += "💳 I've sent an M-Pesa prompt to your phone. Please enter your PIN to complete the payment."

		if err := b.WhatsApp.SendText(ctx, phone, cartSummary); err != nil {
			return fmt.Errorf("failed to send confirmation: %w", err)
		}

		// Clear cart and reset state
		session.Cart = []core.CartItem{}
		session.State = "START"
		return b.Session.Set(ctx, phone, session, 7200)
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
