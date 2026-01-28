package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/dumu-tech/destination-cocktails/internal/core"
	"github.com/dumu-tech/destination-cocktails/internal/events"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// DashboardService handles dashboard business logic
type DashboardService struct {
	adminUserRepo   core.AdminUserRepository
	otpRepo         core.OTPRepository
	productRepo     core.ProductRepository
	orderRepo       core.OrderRepository
	analyticsRepo   core.AnalyticsRepository
	whatsappGateway core.WhatsAppGateway
	eventBus        *events.EventBus
	jwtSecret       string
}

// NewDashboardService creates a new dashboard service
func NewDashboardService(
	adminUserRepo core.AdminUserRepository,
	otpRepo core.OTPRepository,
	productRepo core.ProductRepository,
	orderRepo core.OrderRepository,
	analyticsRepo core.AnalyticsRepository,
	whatsappGateway core.WhatsAppGateway,
	eventBus *events.EventBus,
	jwtSecret string,
) *DashboardService {
	return &DashboardService{
		adminUserRepo:   adminUserRepo,
		otpRepo:         otpRepo,
		productRepo:     productRepo,
		orderRepo:       orderRepo,
		analyticsRepo:   analyticsRepo,
		whatsappGateway: whatsappGateway,
		eventBus:        eventBus,
		jwtSecret:       jwtSecret,
	}
}

// RequestOTP generates and sends an OTP code via WhatsApp
func (s *DashboardService) RequestOTP(ctx context.Context, phone string) error {
	// Check if admin user exists and is active
	isActive, err := s.adminUserRepo.IsActive(ctx, phone)
	if err != nil || !isActive {
		return fmt.Errorf("unauthorized: admin user not found or inactive")
	}

	// Generate OTP code (hardcoded for test admin, random for others)
	var code string
	if phone == "254700000000" {
		code = "123456" // Hardcoded for test admin
	} else {
		code, err = generateOTP()
		if err != nil {
			return fmt.Errorf("failed to generate OTP: %w", err)
		}
	}

	// Create OTP record
	otp := &core.OTPCode{
		ID:          uuid.New().String(),
		PhoneNumber: phone,
		Code:        code,
		ExpiresAt:   time.Now().Add(5 * time.Minute),
		Verified:    false,
		CreatedAt:   time.Now(),
	}

	if err := s.otpRepo.Create(ctx, otp); err != nil {
		return fmt.Errorf("failed to save OTP: %w", err)
	}

	// Send OTP via WhatsApp
	message := fmt.Sprintf("Your Destination Cocktails Dashboard login code is: *%s*\n\nThis code expires in 5 minutes.", code)
	if err := s.whatsappGateway.SendText(ctx, phone, message); err != nil {
		return fmt.Errorf("failed to send OTP via WhatsApp: %w", err)
	}

	return nil
}

// VerifyOTP verifies an OTP code and returns a JWT token
func (s *DashboardService) VerifyOTP(ctx context.Context, phone string, code string) (string, error) {
	// Get latest OTP for phone
	otp, err := s.otpRepo.GetLatestByPhone(ctx, phone)
	if err != nil {
		return "", fmt.Errorf("invalid or expired OTP")
	}

	// Check if OTP is expired
	if time.Now().After(otp.ExpiresAt) {
		return "", fmt.Errorf("OTP has expired")
	}

	// Check if OTP code matches
	if otp.Code != code {
		return "", fmt.Errorf("invalid OTP code")
	}

	// Mark OTP as verified
	if err := s.otpRepo.MarkAsVerified(ctx, otp.ID); err != nil {
		return "", fmt.Errorf("failed to verify OTP: %w", err)
	}

	// Get admin user details
	adminUser, err := s.adminUserRepo.GetByPhone(ctx, phone)
	if err != nil {
		return "", fmt.Errorf("admin user not found: %w", err)
	}

	// Generate JWT token
	token, err := s.generateJWT(adminUser)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	return token, nil
}

// GetProducts retrieves all products
func (s *DashboardService) GetProducts(ctx context.Context) ([]*core.Product, error) {
	return s.productRepo.GetAll(ctx)
}

// UpdateStock updates product stock and emits event
func (s *DashboardService) UpdateStock(ctx context.Context, productID string, stock int) error {
	if err := s.productRepo.UpdateStock(ctx, productID, stock); err != nil {
		return err
	}

	// Emit stock updated event
	s.eventBus.PublishStockUpdated(productID, stock)

	return nil
}

// UpdatePrice updates product price and emits event
func (s *DashboardService) UpdatePrice(ctx context.Context, productID string, price float64) error {
	if err := s.productRepo.UpdatePrice(ctx, productID, price); err != nil {
		return err
	}

	// Emit price updated event
	s.eventBus.PublishPriceUpdated(productID, price)

	return nil
}

// GetOrders retrieves orders with optional filters
func (s *DashboardService) GetOrders(ctx context.Context, status string, limit int) ([]*core.Order, error) {
	return s.orderRepo.GetAllWithFilters(ctx, status, limit)
}

// GetAnalyticsOverview retrieves dashboard overview metrics
func (s *DashboardService) GetAnalyticsOverview(ctx context.Context) (*core.Analytics, error) {
	return s.analyticsRepo.GetOverview(ctx)
}

// GetRevenueTrend retrieves revenue trend data
func (s *DashboardService) GetRevenueTrend(ctx context.Context, days int) ([]*core.RevenueTrend, error) {
	return s.analyticsRepo.GetRevenueTrend(ctx, days)
}

// GetTopProducts retrieves top-selling products
func (s *DashboardService) GetTopProducts(ctx context.Context, limit int) ([]*core.TopProduct, error) {
	return s.analyticsRepo.GetTopProducts(ctx, limit)
}

// GetEventBus returns the event bus for SSE subscriptions
func (s *DashboardService) GetEventBus() *events.EventBus {
	return s.eventBus
}

// GetAdminUserByPhone retrieves an admin user by phone number
func (s *DashboardService) GetAdminUserByPhone(ctx context.Context, phone string) (*core.AdminUser, error) {
	return s.adminUserRepo.GetByPhone(ctx, phone)
}

// generateOTP generates a random 6-digit OTP code
func generateOTP() (string, error) {
	max := big.NewInt(1000000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// generateJWT generates a JWT token for an admin user
func (s *DashboardService) generateJWT(user *core.AdminUser) (string, error) {
	claims := jwt.MapClaims{
		"user_id": user.ID,
		"phone":   user.PhoneNumber,
		"name":    user.Name,
		"role":    user.Role,
		"exp":     time.Now().Add(7 * 24 * time.Hour).Unix(), // 7 days
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtSecret))
}

// ValidateJWT validates a JWT token and returns the claims
func (s *DashboardService) ValidateJWT(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.jwtSecret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}
