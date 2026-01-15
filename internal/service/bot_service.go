package service

import (
	"context"
	"fmt"
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

// HandleIncomingMessage processes incoming WhatsApp messages
func (b *BotService) HandleIncomingMessage(phone string, message string, messageType string) error {
	ctx := context.Background()

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
	welcomeMsg := "🍹 Welcome to Destination Cocktails!\n\n" +
		"Order drinks directly from your table. What would you like to do?\n\n" +
		"Reply with: *Order Drinks* to browse our menu"

	if err := b.WhatsApp.SendText(ctx, phone, welcomeMsg); err != nil {
		return fmt.Errorf("failed to send welcome message: %w", err)
	}

	// Set state to MENU
	session.State = "MENU"
	return b.Session.Set(ctx, phone, session, 7200)
}

// handleMenu handles the MENU state - shows categories
func (b *BotService) handleMenu(ctx context.Context, phone string, session *core.Session, message string) error {
	message = strings.ToLower(strings.TrimSpace(message))
	
	if message != "order drinks" && !strings.Contains(message, "order") {
		// Invalid input, remind user
		reminderMsg := "Please reply with *Order Drinks* to browse our menu"
		return b.WhatsApp.SendText(ctx, phone, reminderMsg)
	}

	// Get menu (grouped by category)
	menu, err := b.Repo.GetMenu(ctx)
	if err != nil {
		return fmt.Errorf("failed to get menu: %w", err)
	}

	// Build category list
	categories := make([]struct {
		ID          string
		Title       string
		Description string
	}, 0, len(menu))

	for category := range menu {
		categories = append(categories, struct {
			ID          string
			Title       string
			Description string
		}{
			ID:    category,
			Title: category,
		})
	}

	// Limit to 10 categories (WhatsApp list limit)
	if len(categories) > 10 {
		categories = categories[:10]
	}

	// Send category list
	items := make([]struct {
		ID          string
		Title       string
		Description string
	}, len(categories))
	for i, cat := range categories {
		items[i] = struct {
			ID          string
			Title       string
			Description string
		}{
			ID:    cat.ID,
			Title: cat.Title,
		}
	}

	// Use WhatsApp client's SendProductList (we'll need to cast or create a helper)
	// For now, send as text with categories
	categoryList := "Select a category:\n\n"
	for i, cat := range categories {
		categoryList += fmt.Sprintf("%d. %s\n", i+1, cat.Title)
	}
	categoryList += "\nReply with the category name to view products."

	if err := b.WhatsApp.SendText(ctx, phone, categoryList); err != nil {
		return fmt.Errorf("failed to send categories: %w", err)
	}

	// Set state to BROWSING
	session.State = "BROWSING"
	return b.Session.Set(ctx, phone, session, 7200)
}

// handleBrowsing handles the BROWSING state - shows products in a category
func (b *BotService) handleBrowsing(ctx context.Context, phone string, session *core.Session, message string) error {
	// Check if message matches a category
	menu, err := b.Repo.GetMenu(ctx)
	if err != nil {
		return fmt.Errorf("failed to get menu: %w", err)
	}

	// Find matching category (case-insensitive)
	var selectedCategory string
	messageLower := strings.ToLower(strings.TrimSpace(message))
	for category := range menu {
		if strings.ToLower(category) == messageLower {
			selectedCategory = category
			break
		}
	}

	if selectedCategory == "" {
		// Invalid category, show available categories
		categoryList := "Please select a valid category:\n\n"
		count := 0
		for category := range menu {
			if count >= 10 {
				break
			}
			categoryList += fmt.Sprintf("• %s\n", category)
			count++
		}
		return b.WhatsApp.SendText(ctx, phone, categoryList)
	}

	// Get products for this category
	products := menu[selectedCategory]
	if len(products) == 0 {
		return b.WhatsApp.SendText(ctx, phone, "No products available in this category.")
	}

	// Limit to 10 products
	if len(products) > 10 {
		products = products[:10]
	}

	// Build product list message
	productList := fmt.Sprintf("Products in *%s*:\n\n", selectedCategory)
	for i, product := range products {
		productList += fmt.Sprintf("%d. %s - KES %.0f\n", i+1, product.Name, product.Price)
	}
	productList += "\nReply with the product name or number to add to cart."

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

	// Try to match by number or name
	var selectedProduct *core.Product
	messageLower := strings.ToLower(strings.TrimSpace(message))

	// Try number first
	if num, err := strconv.Atoi(message); err == nil && num > 0 && num <= len(products) {
		selectedProduct = products[num-1]
	} else {
		// Try name match
		for _, product := range products {
			if strings.ToLower(product.Name) == messageLower || 
			   strings.Contains(strings.ToLower(product.Name), messageLower) {
				selectedProduct = product
				break
			}
		}
	}

	if selectedProduct == nil {
		// Invalid selection
		productList := "Please select a valid product:\n\n"
		for i, product := range products {
			productList += fmt.Sprintf("%d. %s - KES %.0f\n", i+1, product.Name, product.Price)
		}
		return b.WhatsApp.SendText(ctx, phone, productList)
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

	// Confirm addition
	confirmMsg := fmt.Sprintf("✅ Added to cart:\n%s x%d = KES %.0f\n\nCart total: KES %.0f\n\n"+
		"Reply with:\n• *Add More* to continue shopping\n• *Checkout* to complete order",
		product.Name, quantity, product.Price*float64(quantity), total)

	if err := b.WhatsApp.SendText(ctx, phone, confirmMsg); err != nil {
		return fmt.Errorf("failed to send confirmation: %w", err)
	}

	// Set state to CONFIRM_ORDER
	session.State = "CONFIRM_ORDER"
	return b.Session.Set(ctx, phone, session, 7200)
}

// handleConfirmOrder handles the CONFIRM_ORDER state - user can add more or checkout
func (b *BotService) handleConfirmOrder(ctx context.Context, phone string, session *core.Session, message string) error {
	messageLower := strings.ToLower(strings.TrimSpace(message))

	if strings.Contains(messageLower, "add more") || strings.Contains(messageLower, "continue") {
		// Go back to categories
		return b.handleMenu(ctx, phone, session, "Order Drinks")
	}

	if strings.Contains(messageLower, "checkout") {
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

	// Invalid input
	return b.WhatsApp.SendText(ctx, phone, "Please reply with:\n• *Add More* to continue shopping\n• *Checkout* to complete order")
}
