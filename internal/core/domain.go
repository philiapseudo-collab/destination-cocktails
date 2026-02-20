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
	ID                string      `json:"id"`
	UserID            string      `json:"user_id"`        // FK to users.id
	CustomerPhone     string      `json:"customer_phone"` // Denormalized for performance
	TableNumber       string      `json:"table_number"`
	TotalAmount       float64     `json:"total_amount"`
	Status            OrderStatus `json:"status"`
	PaymentMethod     string      `json:"payment_method"`
	PaymentRef        string      `json:"payment_reference"`
	PickupCode        string      `json:"pickup_code"` // 4-digit code for bar staff
	ReadyAt           *time.Time  `json:"ready_at,omitempty"`
	ReadyByUserID     string      `json:"ready_by_user_id,omitempty"`
	CompletedAt       *time.Time  `json:"completed_at,omitempty"`
	CompletedByUserID string      `json:"completed_by_user_id,omitempty"`
	Items             []OrderItem `json:"items"`
	CreatedAt         time.Time   `json:"created_at"`
}

// OrderItem represents a single item in an order
type OrderItem struct {
	ID          string  `json:"id"`
	OrderID     string  `json:"order_id"`
	ProductID   string  `json:"product_id"`
	Quantity    int     `json:"quantity"`
	PriceAtTime float64 `json:"price_at_time"`
	ProductName string  `json:"product_name" gorm:"-"` // Not stored in DB, populated via JOIN
}

// OrderStatus represents the state of an order
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "PENDING"
	OrderStatusPaid      OrderStatus = "PAID"
	OrderStatusFailed    OrderStatus = "FAILED"
	OrderStatusReady     OrderStatus = "READY"
	OrderStatusCompleted OrderStatus = "COMPLETED"
	OrderStatusCancelled OrderStatus = "CANCELLED"
)

// PaymentMethod represents the payment method used
type PaymentMethod string

const (
	PaymentMethodMpesa PaymentMethod = "MPESA"
	PaymentMethodCard  PaymentMethod = "CARD"
	PaymentMethodCash  PaymentMethod = "CASH"
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
	PendingOrderID   string     `json:"pending_order_id"`   // Order ID with pending payment (prevents duplicate checkout)
}

// CartItem represents an item in the user's shopping cart
type CartItem struct {
	ProductID string  `json:"product_id"`
	Quantity  int     `json:"quantity"`
	Name      string  `json:"name"`  // Denormalized for quick display
	Price     float64 `json:"price"` // Denormalized for quick calculation
}

// AdminUser represents a manager/owner who can access the dashboard
type AdminUser struct {
	ID          string    `json:"id"`
	PhoneNumber string    `json:"phone_number"`
	Name        string    `json:"name"`
	Role        string    `json:"role"` // MANAGER, BARTENDER
	PinHash     string    `json:"-"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
}

const (
	AdminRoleManager   = "MANAGER"
	AdminRoleBartender = "BARTENDER"
)

// OTPCode represents a one-time password for authentication
type OTPCode struct {
	ID          string    `json:"id"`
	PhoneNumber string    `json:"phone_number"`
	Code        string    `json:"code"`
	ExpiresAt   time.Time `json:"expires_at"`
	Verified    bool      `json:"verified"`
	CreatedAt   time.Time `json:"created_at"`
}

// Analytics represents dashboard overview metrics
type Analytics struct {
	TodayRevenue      float64    `json:"today_revenue"`
	TodayOrders       int        `json:"today_orders"`
	BestSeller        BestSeller `json:"best_seller"`
	AverageOrderValue float64    `json:"average_order_value"`
}

// BestSeller represents the top-selling product
type BestSeller struct {
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
}

// RevenueTrend represents daily revenue data
type RevenueTrend struct {
	Date       string  `json:"date"`
	Revenue    float64 `json:"revenue"`
	OrderCount int     `json:"order_count"`
}

// TopProduct represents a top-selling product with stats
type TopProduct struct {
	ProductName  string  `json:"product_name"`
	QuantitySold int     `json:"quantity_sold"`
	Revenue      float64 `json:"revenue"`
}

// SalesReport represents an exportable sales report for a time range.
type SalesReport struct {
	Title               string    `json:"title"`
	DateLabel           string    `json:"date_label"`
	Timezone            string    `json:"timezone"`
	BusinessDayStart    string    `json:"business_day_start"`
	StartAt             time.Time `json:"start_at"`
	EndAt               time.Time `json:"end_at"`
	GeneratedAt         time.Time `json:"generated_at"`
	TotalRevenue        float64   `json:"total_revenue"`
	OrderCount          int       `json:"order_count"`
	AverageOrderValue   float64   `json:"average_order_value"`
	SettledStatusFilter []string  `json:"settled_status_filter"`
	Orders              []Order   `json:"orders"`
}
