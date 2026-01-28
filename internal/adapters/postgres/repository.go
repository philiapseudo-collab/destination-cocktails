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
	db                  *gorm.DB
	productRepository   *productRepository
	orderRepository     *orderRepository
	userRepository      *userRepository
	adminUserRepository *adminUserRepository
	otpRepository       *otpRepository
	analyticsRepository *analyticsRepository
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

// adminUserRepository implements AdminUserRepository methods
type adminUserRepository struct {
	*Repository
}

// otpRepository implements OTPRepository methods
type otpRepository struct {
	*Repository
}

// analyticsRepository implements AnalyticsRepository methods
type analyticsRepository struct {
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
	repo.adminUserRepository = &adminUserRepository{Repository: repo}
	repo.otpRepository = &otpRepository{Repository: repo}
	repo.analyticsRepository = &analyticsRepository{Repository: repo}
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

// AdminUserRepository returns the AdminUserRepository interface implementation
func (r *Repository) AdminUserRepository() core.AdminUserRepository {
	return r.adminUserRepository
}

// OTPRepository returns the OTPRepository interface implementation
func (r *Repository) OTPRepository() core.OTPRepository {
	return r.otpRepository
}

// AnalyticsRepository returns the AnalyticsRepository interface implementation
func (r *Repository) AnalyticsRepository() core.AnalyticsRepository {
	return r.analyticsRepository
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

// SearchProducts searches for products by name (case-insensitive partial match)
func (r *productRepository) SearchProducts(ctx context.Context, query string) ([]*core.Product, error) {
	var productModels []ProductModel
	searchPattern := "%" + query + "%"
	if err := r.db.WithContext(ctx).Table("products").
		Where("LOWER(name) LIKE LOWER(?) AND is_active = ?", searchPattern, true).
		Order("name").
		Find(&productModels).Error; err != nil {
		return nil, fmt.Errorf("failed to search products: %w", err)
	}

	products := make([]*core.Product, len(productModels))
	for i, pm := range productModels {
		products[i] = pm.ToDomain()
	}
	return products, nil
}

// UpdatePrice updates the price for a product
func (r *productRepository) UpdatePrice(ctx context.Context, id string, price float64) error {
	result := r.db.WithContext(ctx).Table("products").
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"price":      price,
			"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update price: %w", result.Error)
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

// fetchOrderItemsWithProductNames is a helper method that retrieves order items with product names via JOIN
// This ensures consistent OrderItem shape across all retrieval methods
func (r *orderRepository) fetchOrderItemsWithProductNames(ctx context.Context, orderID string) ([]core.OrderItem, error) {
	type OrderItemWithProduct struct {
		OrderItemModel
		ProductName string `gorm:"column:product_name"`
	}

	var itemsWithProducts []OrderItemWithProduct
	if err := r.db.WithContext(ctx).Table("order_items").
		Select("order_items.*, products.name as product_name").
		Joins("LEFT JOIN products ON order_items.product_id = products.id").
		Where("order_items.order_id = ?", orderID).
		Find(&itemsWithProducts).Error; err != nil {
		return nil, fmt.Errorf("failed to get order items: %w", err)
	}

	items := make([]core.OrderItem, len(itemsWithProducts))
	for i, iwp := range itemsWithProducts {
		item := iwp.OrderItemModel.ToDomain()
		items[i] = core.OrderItem{
			ID:          item.ID,
			OrderID:     item.OrderID,
			ProductID:   item.ProductID,
			Quantity:    item.Quantity,
			PriceAtTime: item.PriceAtTime,
			ProductName: iwp.ProductName, // Populated from JOIN
		}
	}

	return items, nil
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

	// Get order items with product names using helper method
	items, err := r.fetchOrderItemsWithProductNames(ctx, id)
	if err != nil {
		return nil, err
	}

	order := orderModel.ToDomain()
	order.Items = items

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

		// Get order items with product names using helper method
		items, err := r.fetchOrderItemsWithProductNames(ctx, om.ID)
		if err != nil {
			return nil, err
		}
		order.Items = items

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

		// Get order items with product names using helper method
		items, err := r.fetchOrderItemsWithProductNames(ctx, om.ID)
		if err != nil {
			return nil, err
		}
		order.Items = items

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

// GetAllWithFilters retrieves orders with optional status filter and limit
func (r *orderRepository) GetAllWithFilters(ctx context.Context, status string, limit int) ([]*core.Order, error) {
	query := r.db.WithContext(ctx).Table("orders").Order("created_at DESC")

	// Apply status filter if provided
	if status != "" {
		query = query.Where("status = ?", status)
	}

	// Apply limit if provided
	if limit > 0 {
		query = query.Limit(limit)
	}

	var orderModels []OrderModel
	if err := query.Find(&orderModels).Error; err != nil {
		return nil, fmt.Errorf("failed to get orders: %w", err)
	}

	orders := make([]*core.Order, len(orderModels))
	for i, om := range orderModels {
		order := om.ToDomain()

		// Get order items with product names using helper method
		items, err := r.fetchOrderItemsWithProductNames(ctx, om.ID)
		if err != nil {
			return nil, err
		}
		order.Items = items

		orders[i] = order
	}

	return orders, nil
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
	ID            string    `gorm:"column:id;type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID        string    `gorm:"column:user_id;type:uuid;not null"`
	CustomerPhone string    `gorm:"column:customer_phone;type:varchar(20);not null;index"`
	TableNumber   string    `gorm:"column:table_number;type:varchar(20)"`
	TotalAmount   float64   `gorm:"column:total_amount;type:decimal(10,2);not null"`
	Status        string    `gorm:"column:status;type:varchar(20);not null;default:'PENDING';index"`
	PaymentMethod string    `gorm:"column:payment_method;type:varchar(20)"`
	PaymentRef    string    `gorm:"column:payment_reference;type:varchar(255)"`
	PickupCode    string    `gorm:"column:pickup_code;type:varchar(4);index"` // 4-digit pickup code for bar staff
	CreatedAt     time.Time `gorm:"column:created_at;type:timestamp;not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt     time.Time `gorm:"column:updated_at;type:timestamp;not null;default:CURRENT_TIMESTAMP"`
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
		PickupCode:    order.PickupCode,
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
		PickupCode:    o.PickupCode,
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

// AdminUserRepository implementation

// AdminUserModel represents the admin_users table structure
type AdminUserModel struct {
	ID          string    `gorm:"column:id;type:uuid;primaryKey;default:uuid_generate_v4()"`
	PhoneNumber string    `gorm:"column:phone_number;type:varchar(20);not null;uniqueIndex"`
	Name        string    `gorm:"column:name;type:varchar(255);not null"`
	Role        string    `gorm:"column:role;type:varchar(20);not null;default:'MANAGER'"`
	IsActive    bool      `gorm:"column:is_active;type:boolean;not null;default:true"`
	CreatedAt   time.Time `gorm:"column:created_at;type:timestamp;not null;default:CURRENT_TIMESTAMP"`
}

func (AdminUserModel) TableName() string {
	return "admin_users"
}

// ToDomain converts AdminUserModel to core.AdminUser
func (a *AdminUserModel) ToDomain() *core.AdminUser {
	return &core.AdminUser{
		ID:          a.ID,
		PhoneNumber: a.PhoneNumber,
		Name:        a.Name,
		Role:        a.Role,
		IsActive:    a.IsActive,
		CreatedAt:   a.CreatedAt,
	}
}

// GetByPhone retrieves an admin user by phone number
func (r *adminUserRepository) GetByPhone(ctx context.Context, phone string) (*core.AdminUser, error) {
	var adminModel AdminUserModel
	if err := r.db.WithContext(ctx).Table("admin_users").Where("phone_number = ?", phone).First(&adminModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("admin user not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get admin user: %w", err)
	}
	return adminModel.ToDomain(), nil
}

// Create creates a new admin user
func (r *adminUserRepository) Create(ctx context.Context, user *core.AdminUser) error {
	adminModel := &AdminUserModel{
		ID:          user.ID,
		PhoneNumber: user.PhoneNumber,
		Name:        user.Name,
		Role:        user.Role,
		IsActive:    user.IsActive,
		CreatedAt:   user.CreatedAt,
	}
	if err := r.db.WithContext(ctx).Table("admin_users").Create(adminModel).Error; err != nil {
		return fmt.Errorf("failed to create admin user: %w", err)
	}
	return nil
}

// IsActive checks if an admin user is active
func (r *adminUserRepository) IsActive(ctx context.Context, phone string) (bool, error) {
	var adminModel AdminUserModel
	if err := r.db.WithContext(ctx).Table("admin_users").
		Select("is_active").
		Where("phone_number = ?", phone).
		First(&adminModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check admin status: %w", err)
	}
	return adminModel.IsActive, nil
}

// OTPRepository implementation

// OTPCodeModel represents the otp_codes table structure
type OTPCodeModel struct {
	ID          string    `gorm:"column:id;type:uuid;primaryKey;default:uuid_generate_v4()"`
	PhoneNumber string    `gorm:"column:phone_number;type:varchar(20);not null;index"`
	Code        string    `gorm:"column:code;type:varchar(6);not null"`
	ExpiresAt   time.Time `gorm:"column:expires_at;type:timestamp;not null"`
	Verified    bool      `gorm:"column:verified;type:boolean;not null;default:false"`
	CreatedAt   time.Time `gorm:"column:created_at;type:timestamp;not null;default:CURRENT_TIMESTAMP"`
}

func (OTPCodeModel) TableName() string {
	return "otp_codes"
}

// ToDomain converts OTPCodeModel to core.OTPCode
func (o *OTPCodeModel) ToDomain() *core.OTPCode {
	return &core.OTPCode{
		ID:          o.ID,
		PhoneNumber: o.PhoneNumber,
		Code:        o.Code,
		ExpiresAt:   o.ExpiresAt,
		Verified:    o.Verified,
		CreatedAt:   o.CreatedAt,
	}
}

// Create creates a new OTP code
func (r *otpRepository) Create(ctx context.Context, otp *core.OTPCode) error {
	otpModel := &OTPCodeModel{
		ID:          otp.ID,
		PhoneNumber: otp.PhoneNumber,
		Code:        otp.Code,
		ExpiresAt:   otp.ExpiresAt,
		Verified:    otp.Verified,
		CreatedAt:   otp.CreatedAt,
	}
	if err := r.db.WithContext(ctx).Table("otp_codes").Create(otpModel).Error; err != nil {
		return fmt.Errorf("failed to create OTP code: %w", err)
	}
	return nil
}

// GetLatestByPhone retrieves the latest unverified OTP code for a phone number
func (r *otpRepository) GetLatestByPhone(ctx context.Context, phone string) (*core.OTPCode, error) {
	var otpModel OTPCodeModel
	if err := r.db.WithContext(ctx).Table("otp_codes").
		Where("phone_number = ? AND verified = ?", phone, false).
		Order("created_at DESC").
		First(&otpModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("OTP code not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get OTP code: %w", err)
	}
	return otpModel.ToDomain(), nil
}

// MarkAsVerified marks an OTP code as verified
func (r *otpRepository) MarkAsVerified(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Table("otp_codes").
		Where("id = ?", id).
		Update("verified", true)

	if result.Error != nil {
		return fmt.Errorf("failed to mark OTP as verified: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("OTP code not found")
	}
	return nil
}

// CleanupExpired deletes expired OTP codes
func (r *otpRepository) CleanupExpired(ctx context.Context) error {
	result := r.db.WithContext(ctx).Table("otp_codes").
		Where("expires_at < ?", time.Now()).
		Delete(&OTPCodeModel{})

	if result.Error != nil {
		return fmt.Errorf("failed to cleanup expired OTP codes: %w", result.Error)
	}
	return nil
}

// AnalyticsRepository implementation

// GetOverview retrieves dashboard overview metrics for today
func (r *analyticsRepository) GetOverview(ctx context.Context) (*core.Analytics, error) {
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	var analytics core.Analytics

	// Get today's revenue and order count
	type TodayStats struct {
		Revenue    float64
		OrderCount int
	}
	var todayStats TodayStats
	if err := r.db.WithContext(ctx).Table("orders").
		Select("COALESCE(SUM(total_amount), 0) as revenue, COUNT(*) as order_count").
		Where("status = ? AND created_at >= ?", "PAID", startOfDay).
		Scan(&todayStats).Error; err != nil {
		return nil, fmt.Errorf("failed to get today's stats: %w", err)
	}

	analytics.TodayRevenue = todayStats.Revenue
	analytics.TodayOrders = todayStats.OrderCount

	// Calculate average order value
	if todayStats.OrderCount > 0 {
		analytics.AverageOrderValue = todayStats.Revenue / float64(todayStats.OrderCount)
	}

	// Get best seller for today
	type BestSellerResult struct {
		ProductName string
		Quantity    int
	}
	var bestSeller BestSellerResult
	if err := r.db.WithContext(ctx).Table("order_items").
		Select("products.name as product_name, SUM(order_items.quantity) as quantity").
		Joins("JOIN orders ON order_items.order_id = orders.id").
		Joins("JOIN products ON order_items.product_id = products.id").
		Where("orders.status = ? AND orders.created_at >= ?", "PAID", startOfDay).
		Group("products.name").
		Order("quantity DESC").
		Limit(1).
		Scan(&bestSeller).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to get best seller: %w", err)
	}

	analytics.BestSeller = core.BestSeller{
		Name:     bestSeller.ProductName,
		Quantity: bestSeller.Quantity,
	}

	return &analytics, nil
}

// GetRevenueTrend retrieves daily revenue data for the specified number of days
func (r *analyticsRepository) GetRevenueTrend(ctx context.Context, days int) ([]*core.RevenueTrend, error) {
	startDate := time.Now().AddDate(0, 0, -days)

	type TrendResult struct {
		Date       string
		Revenue    float64
		OrderCount int
	}

	var results []TrendResult
	if err := r.db.WithContext(ctx).Table("orders").
		Select("DATE(created_at) as date, COALESCE(SUM(total_amount), 0) as revenue, COUNT(*) as order_count").
		Where("status = ? AND created_at >= ?", "PAID", startDate).
		Group("DATE(created_at)").
		Order("date ASC").
		Scan(&results).Error; err != nil {
		return nil, fmt.Errorf("failed to get revenue trend: %w", err)
	}

	trends := make([]*core.RevenueTrend, len(results))
	for i, r := range results {
		trends[i] = &core.RevenueTrend{
			Date:       r.Date,
			Revenue:    r.Revenue,
			OrderCount: r.OrderCount,
		}
	}

	return trends, nil
}

// GetTopProducts retrieves top-selling products by revenue
func (r *analyticsRepository) GetTopProducts(ctx context.Context, limit int) ([]*core.TopProduct, error) {
	// Get data for last 30 days
	startDate := time.Now().AddDate(0, 0, -30)

	type ProductResult struct {
		ProductName  string
		QuantitySold int
		Revenue      float64
	}

	var results []ProductResult
	if err := r.db.WithContext(ctx).Table("order_items").
		Select("products.name as product_name, SUM(order_items.quantity) as quantity_sold, SUM(order_items.quantity * order_items.price_at_time) as revenue").
		Joins("JOIN orders ON order_items.order_id = orders.id").
		Joins("JOIN products ON order_items.product_id = products.id").
		Where("orders.status = ? AND orders.created_at >= ?", "PAID", startDate).
		Group("products.name").
		Order("revenue DESC").
		Limit(limit).
		Scan(&results).Error; err != nil {
		return nil, fmt.Errorf("failed to get top products: %w", err)
	}

	products := make([]*core.TopProduct, len(results))
	for i, r := range results {
		products[i] = &core.TopProduct{
			ProductName:  r.ProductName,
			QuantitySold: r.QuantitySold,
			Revenue:      r.Revenue,
		}
	}

	return products, nil
}
