package core

import "context"

// ProductRepository defines the interface for product data access
type ProductRepository interface {
	GetByID(ctx context.Context, id string) (*Product, error)
	GetByCategory(ctx context.Context, category string) ([]*Product, error)
	GetAll(ctx context.Context) ([]*Product, error)
	GetMenu(ctx context.Context) (map[string][]*Product, error)
	UpdateStock(ctx context.Context, id string, quantity int) error
	UpdatePrice(ctx context.Context, id string, price float64) error
	SearchProducts(ctx context.Context, query string) ([]*Product, error)
}

// OrderRepository defines the interface for order data access
type OrderRepository interface {
	CreateOrder(ctx context.Context, order *Order) error
	GetByID(ctx context.Context, id string) (*Order, error)
	GetByUserID(ctx context.Context, userID string) ([]*Order, error)
	GetByPhone(ctx context.Context, phone string) ([]*Order, error)
	UpdateStatus(ctx context.Context, id string, status OrderStatus) error
	GetAllWithFilters(ctx context.Context, status string, limit int) ([]*Order, error)
	FindPendingByPhoneAndAmount(ctx context.Context, phone string, amount float64) (*Order, error)
	FindPendingByHashedPhoneAndAmount(ctx context.Context, hashedPhone string, amount float64) (*Order, error) // Match by hashed phone from buygoods webhooks
	FindPendingByAmount(ctx context.Context, amount float64) (*Order, error)                                   // Fallback when phone unavailable
}

// UserRepository defines the interface for user data access
type UserRepository interface {
	GetByPhone(ctx context.Context, phone string) (*User, error)
	Create(ctx context.Context, user *User) error
	GetOrCreateByPhone(ctx context.Context, phone string) (*User, error)
}

// SessionRepository defines the interface for session state management in Redis
type SessionRepository interface {
	Get(ctx context.Context, phone string) (*Session, error)
	Set(ctx context.Context, phone string, session *Session, ttl int) error
	Delete(ctx context.Context, phone string) error
	UpdateStep(ctx context.Context, phone string, step string) error
	UpdateCart(ctx context.Context, phone string, cartItems string) error
}

// Button represents a quick reply button
type Button struct {
	ID    string
	Title string
}

// WhatsAppGateway defines the interface for WhatsApp messaging
type WhatsAppGateway interface {
	SendText(ctx context.Context, phone string, message string) error
	SendMenu(ctx context.Context, phone string, products []*Product) error
	SendCategoryList(ctx context.Context, phone string, categories []string) error
	SendProductList(ctx context.Context, phone string, category string, products []*Product) error
	SendMenuButtons(ctx context.Context, phone string, text string, buttons []Button) error
}

// PaymentGateway defines the interface for payment processing
type PaymentGateway interface {
	InitiateSTKPush(ctx context.Context, orderID string, phone string, amount float64) error
	VerifyWebhook(ctx context.Context, signature string, payload []byte) bool
	ProcessWebhook(ctx context.Context, payload []byte) (*PaymentWebhook, error)
}

// PaymentWebhook represents the structure of a payment webhook result
type PaymentWebhook struct {
	OrderID     string
	Status      string
	Reference   string
	Amount      float64
	Phone       string // Sender phone number from webhook (may be empty for buygoods)
	HashedPhone string // SHA256 hashed phone from buygoods webhooks
	Success     bool
}

// AdminUserRepository defines the interface for admin user data access
type AdminUserRepository interface {
	GetByPhone(ctx context.Context, phone string) (*AdminUser, error)
	GetActiveByRole(ctx context.Context, role string) ([]*AdminUser, error)
	Create(ctx context.Context, user *AdminUser) error
	IsActive(ctx context.Context, phone string) (bool, error)
}

// OTPRepository defines the interface for OTP code management
type OTPRepository interface {
	Create(ctx context.Context, otp *OTPCode) error
	GetLatestByPhone(ctx context.Context, phone string) (*OTPCode, error)
	MarkAsVerified(ctx context.Context, id string) error
	CleanupExpired(ctx context.Context) error
}

// AnalyticsRepository defines the interface for analytics data access
type AnalyticsRepository interface {
	GetOverview(ctx context.Context) (*Analytics, error)
	GetRevenueTrend(ctx context.Context, days int) ([]*RevenueTrend, error)
	GetTopProducts(ctx context.Context, limit int) ([]*TopProduct, error)
}
