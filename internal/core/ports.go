package core

import "context"

// ProductRepository defines the interface for product data access
type ProductRepository interface {
	GetByID(ctx context.Context, id string) (*Product, error)
	GetByCategory(ctx context.Context, category string) ([]*Product, error)
	GetAll(ctx context.Context) ([]*Product, error)
	GetMenu(ctx context.Context) (map[string][]*Product, error)
	UpdateStock(ctx context.Context, id string, quantity int) error
}

// OrderRepository defines the interface for order data access
type OrderRepository interface {
	CreateOrder(ctx context.Context, order *Order) error
	GetByID(ctx context.Context, id string) (*Order, error)
	GetByUserID(ctx context.Context, userID string) ([]*Order, error)
	GetByPhone(ctx context.Context, phone string) ([]*Order, error)
	UpdateStatus(ctx context.Context, id string, status OrderStatus) error
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
	InitiateSTKPush(ctx context.Context, phone string, amount float64, orderID string) (string, error)
	VerifyWebhook(ctx context.Context, signature string, payload []byte) bool
	ProcessWebhook(ctx context.Context, payload []byte) (*PaymentWebhook, error)
}

// PaymentWebhook represents the structure of a payment webhook result
type PaymentWebhook struct {
	OrderID   string
	Status    string
	Reference string
	Amount    float64
	Success   bool
}
