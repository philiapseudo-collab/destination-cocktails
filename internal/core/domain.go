package core

import "time"

// Product represents a menu item (drink/food) in the system
type Product struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Price         float64 `json:"price"`
	Category      string  `json:"category"`
	StockQuantity int     `json:"stock_quantity"`
	ImageURL      string  `json:"image_url"`
	IsActive      bool    `json:"is_active"`
}

// Order represents a customer order
type Order struct {
	ID             string      `json:"id"`
	UserID         string      `json:"user_id"`         // FK to users.id
	CustomerPhone  string      `json:"customer_phone"`  // Denormalized for performance
	TableNumber    string      `json:"table_number"`
	TotalAmount    float64     `json:"total_amount"`
	Status         OrderStatus `json:"status"`
	PaymentMethod  string      `json:"payment_method"`
	PaymentRef     string      `json:"payment_reference"`
	Items          []OrderItem `json:"items"`
	CreatedAt      time.Time   `json:"created_at"`
}

// OrderItem represents a single item in an order
type OrderItem struct {
	ID           string  `json:"id"`
	OrderID      string  `json:"order_id"`
	ProductID    string  `json:"product_id"`
	Quantity     int     `json:"quantity"`
	PriceAtTime  float64 `json:"price_at_time"`
}

// OrderStatus represents the state of an order
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "PENDING"
	OrderStatusPaid      OrderStatus = "PAID"
	OrderStatusFailed    OrderStatus = "FAILED"
	OrderStatusServed    OrderStatus = "SERVED"
	OrderStatusCancelled OrderStatus = "CANCELLED"
)

// PaymentMethod represents the payment method used
type PaymentMethod string

const (
	PaymentMethodMpesa PaymentMethod = "MPESA"
	PaymentMethodCard   PaymentMethod = "CARD"
	PaymentMethodCash   PaymentMethod = "CASH"
)

// User represents a customer in the system
type User struct {
	ID          string    `json:"id"`
	PhoneNumber string    `json:"phone_number"`
	Name        string    `json:"name"`
	CreatedAt   time.Time `json:"created_at"`
}

// Session represents a user's current state in Redis
type Session struct {
	State            string     `json:"state"`              // START, MENU, BROWSING, SELECTING_PRODUCT, QUANTITY, CONFIRMATION
	CurrentCategory  string     `json:"current_category"`   // Current category being browsed
	CurrentProductID string     `json:"current_product_id"` // Product being selected
	Cart             []CartItem `json:"cart"`               // Array of cart items
}

// CartItem represents an item in the user's shopping cart
type CartItem struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
	Name      string `json:"name"`      // Denormalized for quick display
	Price     float64 `json:"price"`    // Denormalized for quick calculation
}
