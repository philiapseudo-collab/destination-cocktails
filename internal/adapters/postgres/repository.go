package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dumu-tech/destination-cocktails/internal/core"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Repository implements ProductRepository, OrderRepository, and UserRepository using GORM with pgx driver
type Repository struct {
	db                *gorm.DB
	productRepository *productRepository
	orderRepository   *orderRepository
	userRepository    *userRepository
}

// productRepository implements ProductRepository methods
type productRepository struct {
	*Repository
}

// orderRepository implements OrderRepository methods  
type orderRepository struct {
	*Repository
}

// userRepository implements UserRepository methods
type userRepository struct {
	*Repository
}

// NewRepository creates a new Postgres repository instance
func NewRepository(dbURL string) (*Repository, error) {
	// GORM with pgx driver (postgres driver uses pgx under the hood)
	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	repo := &Repository{db: db}
	// Set up embedded types
	repo.productRepository = &productRepository{Repository: repo}
	repo.orderRepository = &orderRepository{Repository: repo}
	repo.userRepository = &userRepository{Repository: repo}
	return repo, nil
}

// ProductRepository returns the ProductRepository interface implementation
func (r *Repository) ProductRepository() core.ProductRepository {
	return r.productRepository
}

// OrderRepository returns the OrderRepository interface implementation
func (r *Repository) OrderRepository() core.OrderRepository {
	return r.orderRepository
}

// UserRepository returns the UserRepository interface implementation
func (r *Repository) UserRepository() core.UserRepository {
	return r.userRepository
}

// ProductRepository implementation

// GetByID retrieves a product by its ID
func (r *productRepository) GetByID(ctx context.Context, id string) (*core.Product, error) {
	var productModel ProductModel
	if err := r.db.WithContext(ctx).Table("products").Where("id = ?", id).First(&productModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("product not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get product: %w", err)
	}
	return productModel.ToDomain(), nil
}

// GetByCategory retrieves all products in a specific category
func (r *productRepository) GetByCategory(ctx context.Context, category string) ([]*core.Product, error) {
	var productModels []ProductModel
	if err := r.db.WithContext(ctx).Table("products").
		Where("category = ? AND is_active = ?", category, true).
		Find(&productModels).Error; err != nil {
		return nil, fmt.Errorf("failed to get products by category: %w", err)
	}

	products := make([]*core.Product, len(productModels))
	for i, pm := range productModels {
		products[i] = pm.ToDomain()
	}
	return products, nil
}

// GetAll retrieves all active products
func (r *productRepository) GetAll(ctx context.Context) ([]*core.Product, error) {
	var productModels []ProductModel
	if err := r.db.WithContext(ctx).Table("products").
		Where("is_active = ?", true).
		Find(&productModels).Error; err != nil {
		return nil, fmt.Errorf("failed to get all products: %w", err)
	}

	products := make([]*core.Product, len(productModels))
	for i, pm := range productModels {
		products[i] = pm.ToDomain()
	}
	return products, nil
}

// GetMenu retrieves all active products grouped by category
func (r *productRepository) GetMenu(ctx context.Context) (map[string][]*core.Product, error) {
	var productModels []ProductModel
	if err := r.db.WithContext(ctx).Table("products").
		Where("is_active = ?", true).
		Order("category, name").
		Find(&productModels).Error; err != nil {
		return nil, fmt.Errorf("failed to get menu: %w", err)
	}

	menu := make(map[string][]*core.Product)
	for _, pm := range productModels {
		product := pm.ToDomain()
		category := product.Category
		if menu[category] == nil {
			menu[category] = make([]*core.Product, 0)
		}
		menu[category] = append(menu[category], product)
	}

	return menu, nil
}

// UpdateStock updates the stock quantity for a product
func (r *productRepository) UpdateStock(ctx context.Context, id string, quantity int) error {
	result := r.db.WithContext(ctx).Table("products").
		Where("id = ?", id).
		Update("stock_quantity", quantity)
	
	if result.Error != nil {
		return fmt.Errorf("failed to update stock: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("product not found")
	}
	return nil
}

// OrderRepository implementation

// CreateOrder creates a new order with its items in a transaction
func (r *orderRepository) CreateOrder(ctx context.Context, order *core.Order) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Create order
		orderModel := OrderModelFromDomain(order)
		if err := tx.Table("orders").Create(&orderModel).Error; err != nil {
			return fmt.Errorf("failed to create order: %w", err)
		}

		// Create order items
		for _, item := range order.Items {
			itemModel := OrderItemModelFromDomain(&item)
			itemModel.OrderID = orderModel.ID
			if err := tx.Table("order_items").Create(&itemModel).Error; err != nil {
				return fmt.Errorf("failed to create order item: %w", err)
			}
		}

		return nil
	})
}

// GetByID retrieves an order by its ID with all items (implements OrderRepository)
func (r *orderRepository) GetByID(ctx context.Context, id string) (*core.Order, error) {
	var orderModel OrderModel
	if err := r.db.WithContext(ctx).Table("orders").Where("id = ?", id).First(&orderModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("order not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	// Get order items
	var itemModels []OrderItemModel
	if err := r.db.WithContext(ctx).Table("order_items").
		Where("order_id = ?", id).
		Find(&itemModels).Error; err != nil {
		return nil, fmt.Errorf("failed to get order items: %w", err)
	}

	order := orderModel.ToDomain()
	order.Items = make([]core.OrderItem, len(itemModels))
	for i, im := range itemModels {
		order.Items[i] = *im.ToDomain()
	}

	return order, nil
}

// GetByUserID retrieves all orders for a specific user
func (r *orderRepository) GetByUserID(ctx context.Context, userID string) ([]*core.Order, error) {
	var orderModels []OrderModel
	if err := r.db.WithContext(ctx).Table("orders").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&orderModels).Error; err != nil {
		return nil, fmt.Errorf("failed to get orders by user ID: %w", err)
	}

	orders := make([]*core.Order, len(orderModels))
	for i, om := range orderModels {
		order := om.ToDomain()
		
		// Get order items for each order
		var itemModels []OrderItemModel
		if err := r.db.WithContext(ctx).Table("order_items").
			Where("order_id = ?", om.ID).
			Find(&itemModels).Error; err != nil {
			return nil, fmt.Errorf("failed to get order items: %w", err)
		}

		order.Items = make([]core.OrderItem, len(itemModels))
		for j, im := range itemModels {
			order.Items[j] = *im.ToDomain()
		}

		orders[i] = order
	}

	return orders, nil
}

// GetByPhone retrieves all orders for a specific phone number
func (r *orderRepository) GetByPhone(ctx context.Context, phone string) ([]*core.Order, error) {
	var orderModels []OrderModel
	if err := r.db.WithContext(ctx).Table("orders").
		Where("customer_phone = ?", phone).
		Order("created_at DESC").
		Find(&orderModels).Error; err != nil {
		return nil, fmt.Errorf("failed to get orders by phone: %w", err)
	}

	orders := make([]*core.Order, len(orderModels))
	for i, om := range orderModels {
		order := om.ToDomain()
		
		// Get order items for each order
		var itemModels []OrderItemModel
		if err := r.db.WithContext(ctx).Table("order_items").
			Where("order_id = ?", om.ID).
			Find(&itemModels).Error; err != nil {
			return nil, fmt.Errorf("failed to get order items: %w", err)
		}

		order.Items = make([]core.OrderItem, len(itemModels))
		for j, im := range itemModels {
			order.Items[j] = *im.ToDomain()
		}

		orders[i] = order
	}

	return orders, nil
}

// UpdateStatus updates the status of an order
func (r *orderRepository) UpdateStatus(ctx context.Context, id string, status core.OrderStatus) error {
	result := r.db.WithContext(ctx).Table("orders").
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     string(status),
			"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update order status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("order not found")
	}
	return nil
}

// Database Models (with GORM tags)

// ProductModel represents the product table structure
type ProductModel struct {
	ID            string         `gorm:"column:id;type:uuid;primaryKey;default:uuid_generate_v4()"`
	Name          string         `gorm:"column:name;type:varchar(255);not null"`
	Description   sql.NullString `gorm:"column:description;type:text"`
	Price         float64        `gorm:"column:price;type:decimal(10,2);not null"`
	Category      string         `gorm:"column:category;type:varchar(100);not null"`
	StockQuantity int            `gorm:"column:stock_quantity;type:integer;not null;default:0"`
	ImageURL      sql.NullString `gorm:"column:image_url;type:varchar(500)"`
	IsActive      bool           `gorm:"column:is_active;type:boolean;not null;default:true"`
}

func (ProductModel) TableName() string {
	return "products"
}

// ToDomain converts ProductModel to core.Product
func (p *ProductModel) ToDomain() *core.Product {
	product := &core.Product{
		ID:            p.ID,
		Name:          p.Name,
		Price:         p.Price,
		Category:      p.Category,
		StockQuantity: p.StockQuantity,
		IsActive:      p.IsActive,
	}

	if p.Description.Valid {
		product.Description = p.Description.String
	}
	if p.ImageURL.Valid {
		product.ImageURL = p.ImageURL.String
	}

	return product
}

// OrderModel represents the order table structure
type OrderModel struct {
	ID             string    `gorm:"column:id;type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID         string    `gorm:"column:user_id;type:uuid;not null"`
	CustomerPhone  string    `gorm:"column:customer_phone;type:varchar(20);not null"`
	TableNumber    string    `gorm:"column:table_number;type:varchar(20)"`
	TotalAmount    float64   `gorm:"column:total_amount;type:decimal(10,2);not null"`
	Status         string    `gorm:"column:status;type:varchar(20);not null;default:'PENDING'"`
	PaymentMethod  string    `gorm:"column:payment_method;type:varchar(20)"`
	PaymentRef     string    `gorm:"column:payment_reference;type:varchar(255)"`
	CreatedAt      time.Time `gorm:"column:created_at;type:timestamp;not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt      time.Time `gorm:"column:updated_at;type:timestamp;not null;default:CURRENT_TIMESTAMP"`
}

func (OrderModel) TableName() string {
	return "orders"
}

// OrderModelFromDomain creates OrderModel from core.Order
func OrderModelFromDomain(order *core.Order) *OrderModel {
	return &OrderModel{
		ID:            order.ID,
		UserID:        order.UserID,
		CustomerPhone: order.CustomerPhone,
		TableNumber:   order.TableNumber,
		TotalAmount:   order.TotalAmount,
		Status:        string(order.Status),
		PaymentMethod: order.PaymentMethod,
		PaymentRef:    order.PaymentRef,
		CreatedAt:     order.CreatedAt,
	}
}

// ToDomain converts OrderModel to core.Order
func (o *OrderModel) ToDomain() *core.Order {
	return &core.Order{
		ID:            o.ID,
		UserID:        o.UserID,
		CustomerPhone: o.CustomerPhone,
		TableNumber:   o.TableNumber,
		TotalAmount:   o.TotalAmount,
		Status:        core.OrderStatus(o.Status),
		PaymentMethod: o.PaymentMethod,
		PaymentRef:    o.PaymentRef,
		CreatedAt:     o.CreatedAt,
		Items:         []core.OrderItem{}, // Will be populated separately
	}
}

// OrderItemModel represents the order_items table structure
type OrderItemModel struct {
	ID          string  `gorm:"column:id;type:uuid;primaryKey;default:uuid_generate_v4()"`
	OrderID     string  `gorm:"column:order_id;type:uuid;not null"`
	ProductID   string  `gorm:"column:product_id;type:uuid;not null"`
	Quantity    int     `gorm:"column:quantity;type:integer;not null"`
	PriceAtTime float64 `gorm:"column:price_at_time;type:decimal(10,2);not null"`
}

func (OrderItemModel) TableName() string {
	return "order_items"
}

// OrderItemModelFromDomain creates OrderItemModel from core.OrderItem
func OrderItemModelFromDomain(item *core.OrderItem) *OrderItemModel {
	return &OrderItemModel{
		ID:          item.ID,
		OrderID:     item.OrderID,
		ProductID:   item.ProductID,
		Quantity:    item.Quantity,
		PriceAtTime: item.PriceAtTime,
	}
}

// ToDomain converts OrderItemModel to core.OrderItem
func (oi *OrderItemModel) ToDomain() *core.OrderItem {
	return &core.OrderItem{
		ID:          oi.ID,
		OrderID:     oi.OrderID,
		ProductID:   oi.ProductID,
		Quantity:    oi.Quantity,
		PriceAtTime: oi.PriceAtTime,
	}
}

// UserRepository implementation

// UserModel represents the users table structure
type UserModel struct {
	ID          string    `gorm:"column:id;type:uuid;primaryKey;default:uuid_generate_v4()"`
	PhoneNumber string    `gorm:"column:phone_number;type:varchar(20);not null;uniqueIndex"`
	Name        string    `gorm:"column:name;type:varchar(255)"`
	CreatedAt   time.Time `gorm:"column:created_at;type:timestamp;not null;default:CURRENT_TIMESTAMP"`
}

func (UserModel) TableName() string {
	return "users"
}

// ToDomain converts UserModel to core.User
func (u *UserModel) ToDomain() *core.User {
	return &core.User{
		ID:          u.ID,
		PhoneNumber: u.PhoneNumber,
		Name:        u.Name,
		CreatedAt:   u.CreatedAt,
	}
}

// GetByPhone retrieves a user by phone number
func (r *userRepository) GetByPhone(ctx context.Context, phone string) (*core.User, error) {
	var userModel UserModel
	if err := r.db.WithContext(ctx).Table("users").Where("phone_number = ?", phone).First(&userModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("user not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return userModel.ToDomain(), nil
}

// Create creates a new user
func (r *userRepository) Create(ctx context.Context, user *core.User) error {
	userModel := &UserModel{
		ID:          user.ID,
		PhoneNumber: user.PhoneNumber,
		Name:        user.Name,
		CreatedAt:   user.CreatedAt,
	}
	if err := r.db.WithContext(ctx).Table("users").Create(userModel).Error; err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

// GetOrCreateByPhone retrieves a user by phone or creates one if not found
func (r *userRepository) GetOrCreateByPhone(ctx context.Context, phone string) (*core.User, error) {
	user, err := r.GetByPhone(ctx, phone)
	if err == nil {
		return user, nil
	}

	// User doesn't exist, create new one
	newUser := &core.User{
		ID:          uuid.New().String(),
		PhoneNumber: phone,
		Name:        "",
		CreatedAt:   time.Now(),
	}

	if err := r.Create(ctx, newUser); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return newUser, nil
}
