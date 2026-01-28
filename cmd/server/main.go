package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/dumu-tech/destination-cocktails/internal/adapters/http"
	"github.com/dumu-tech/destination-cocktails/internal/adapters/payment"
	"github.com/dumu-tech/destination-cocktails/internal/adapters/postgres"
	redisRepo "github.com/dumu-tech/destination-cocktails/internal/adapters/redis"
	"github.com/dumu-tech/destination-cocktails/internal/adapters/whatsapp"
	"github.com/dumu-tech/destination-cocktails/internal/config"
	"github.com/dumu-tech/destination-cocktails/internal/core"
	"github.com/dumu-tech/destination-cocktails/internal/events"
	"github.com/dumu-tech/destination-cocktails/internal/middleware"
	"github.com/dumu-tech/destination-cocktails/internal/service"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// maskURL masks sensitive parts of a URL for logging
func maskURL(url string) string {
	if url == "" {
		return "<empty>"
	}
	// Just show the protocol and host, mask the rest
	if len(url) < 20 {
		return url[:min(10, len(url))] + "***"
	}
	return url[:20] + "***"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// orderRepoAdapter adapts core.OrderRepository to http.OrderRepositoryHandler
type orderRepoAdapter struct {
	repo core.OrderRepository
}

func (a *orderRepoAdapter) UpdateStatus(ctx context.Context, id string, status core.OrderStatus) error {
	return a.repo.UpdateStatus(ctx, id, status)
}

func (a *orderRepoAdapter) GetOrderByID(ctx context.Context, id string) (*core.Order, error) {
	return a.repo.GetByID(ctx, id)
}

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Println("Initializing connections...")

	// Create context for initialization
	initCtx, initCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer initCancel()

	// Connect to Redis
	var rdb *redis.Client
	var redisRepository *redisRepo.Repository
	log.Printf("Connecting to Redis...")
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Printf("WARNING: Failed to parse Redis URL: %v", err)
		log.Println("Server will continue without Redis (some features may be unavailable)")
	} else {
		if cfg.RedisPassword != "" {
			redisOpts.Password = cfg.RedisPassword
		}

		rdb = redis.NewClient(redisOpts)

		// Ping Redis to verify connection (with timeout)
		pingCtx, pingCancel := context.WithTimeout(initCtx, 5*time.Second)
		if err := rdb.Ping(pingCtx).Err(); err != nil {
			log.Printf("WARNING: Failed to connect to Redis: %v", err)
			log.Println("Server will continue without Redis (some features may be unavailable)")
			rdb.Close()
			rdb = nil
		} else {
			log.Println("✓ Redis connection established")
			redisRepository = redisRepo.NewRepository(rdb)
			defer rdb.Close()
		}
		pingCancel()
	}

	// Connect to PostgreSQL
	log.Printf("Connecting to PostgreSQL...")
	dbpool, err := pgxpool.New(initCtx, cfg.DBURL)
	if err != nil {
		log.Fatalf("CRITICAL: Failed to create PostgreSQL connection pool: %v", err)
	}
	defer dbpool.Close()

	// Ping PostgreSQL to verify connection
	pingCtx, pingCancel := context.WithTimeout(initCtx, 10*time.Second)
	if err := dbpool.Ping(pingCtx); err != nil {
		log.Fatalf("CRITICAL: Failed to connect to PostgreSQL: %v", err)
	}
	pingCancel()
	log.Println("✓ PostgreSQL connection established")

	// Initialize repositories using GORM (we still need pgxpool for some operations)
	log.Println("Initializing PostgreSQL repository...")
	postgresRepo, err := postgres.NewRepository(cfg.DBURL)
	if err != nil {
		log.Fatalf("CRITICAL: Failed to initialize postgres repository: %v", err)
	}

	// Initialize WhatsApp client
	log.Printf("Initializing WhatsApp client (Phone Number ID length: %d, Token length: %d)",
		len(cfg.WhatsAppPhoneNumberID), len(cfg.WhatsAppToken))
	if cfg.WhatsAppPhoneNumberID == "" {
		log.Fatalf("CRITICAL: WHATSAPP_PHONE_NUMBER_ID environment variable is not set")
	}
	if cfg.WhatsAppToken == "" {
		log.Fatalf("CRITICAL: WHATSAPP_TOKEN environment variable is not set")
	}
	whatsappClient := whatsapp.NewClient(cfg.WhatsAppPhoneNumberID, cfg.WhatsAppToken)
	log.Printf("WhatsApp client initialized with Phone Number ID: %s", cfg.WhatsAppPhoneNumberID)

	// Initialize Payment client
	paymentClient, err := payment.NewClient()
	if err != nil {
		log.Fatalf("Failed to initialize payment client: %v", err)
	}

	// Initialize Bot Service (Redis is optional but required for sessions)
	log.Println("Initializing services...")
	if redisRepository == nil {
		log.Fatalf("CRITICAL: Redis is required for bot service. Please configure REDIS_URL environment variable.")
	}

	botService := service.NewBotService(
		postgresRepo.ProductRepository(),
		redisRepository,
		whatsappClient,
		paymentClient,
		postgresRepo.OrderRepository(),
		postgresRepo.UserRepository(),
	)

	// Create adapter for OrderRepository to match OrderRepositoryHandler interface
	orderRepoAdapter := &orderRepoAdapter{
		repo: postgresRepo.OrderRepository(),
	}

	// Initialize HTTP Handler
	httpHandler := http.NewHandler(
		botService,
		paymentClient,
		orderRepoAdapter,
		whatsappClient,
	)

	// Initialize Event Bus for SSE
	log.Println("Initializing Event Bus...")
	eventBus := events.NewEventBus()

	// Wire EventBus to HTTP Handler for real-time order updates
	httpHandler.SetEventBus(eventBus)

	// Initialize Dashboard Service
	log.Println("Initializing Dashboard Service...")
	dashboardService := service.NewDashboardService(
		postgresRepo.AdminUserRepository(),
		postgresRepo.OTPRepository(),
		postgresRepo.ProductRepository(),
		postgresRepo.OrderRepository(),
		postgresRepo.AnalyticsRepository(),
		whatsappClient,
		eventBus,
		cfg.JWTSecret,
	)

	// Initialize Dashboard Handler
	dashboardHandler := http.NewDashboardHandler(dashboardService)

	// Initialize Fiber app early so health checks work
	log.Println("Initializing HTTP server...")
	app := fiber.New(fiber.Config{
		AppName:      "Destination Cocktails API",
		ServerHeader: "Fiber",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	})

	// Middleware
	app.Use(recover.New())
	app.Use(logger.New())

	// CORS middleware for dashboard
	app.Use(cors.New(cors.Config{
		AllowOriginsFunc: func(origin string) bool {
			// Allow localhost for development
			if origin == "http://localhost:3000" {
				return true
			}
			// Allow any Railway subdomain (e.g. dashboard-production.up.railway.app)
			if strings.HasSuffix(origin, ".railway.app") {
				return true
			}
			return false
		},
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization",
		AllowCredentials: true,
		ExposeHeaders:    "Content-Length",
		MaxAge:           3600,
	}))

	// Health check route - responds immediately
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"project": "destination-cocktails",
		})
	})

	// Early startup route for debugging
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"message": "Destination Cocktails API",
			"status":  "running",
		})
	})

	// WhatsApp Webhook routes
	// GET for webhook verification (WhatsApp requires this)
	app.Get("/webhook", httpHandler.VerifyWebhook)
	// POST for receiving messages
	app.Post("/webhook", httpHandler.ReceiveMessage)

	// Payment webhook route
	app.Post("/api/webhooks/payment", httpHandler.HandlePaymentWebhook)

	// Dashboard routes (authentication - no auth required)
	app.Post("/api/admin/auth/request-otp", dashboardHandler.RequestOTP)
	app.Post("/api/admin/auth/verify-otp", dashboardHandler.VerifyOTP)

	// Dashboard routes (protected with JWT middleware)
	authMiddleware := middleware.AuthMiddleware(dashboardService)
	app.Post("/api/admin/auth/logout", authMiddleware, dashboardHandler.Logout)
	app.Get("/api/admin/auth/me", authMiddleware, dashboardHandler.GetMe)

	// Products
	app.Get("/api/admin/products", authMiddleware, dashboardHandler.GetProducts)
	app.Patch("/api/admin/products/:id/stock", authMiddleware, dashboardHandler.UpdateStock)
	app.Patch("/api/admin/products/:id/price", authMiddleware, dashboardHandler.UpdatePrice)

	// Orders
	app.Get("/api/admin/orders", authMiddleware, dashboardHandler.GetOrders)

	// Analytics
	app.Get("/api/admin/analytics/overview", authMiddleware, dashboardHandler.GetAnalyticsOverview)
	app.Get("/api/admin/analytics/revenue", authMiddleware, dashboardHandler.GetRevenueTrend)
	app.Get("/api/admin/analytics/top-products", authMiddleware, dashboardHandler.GetTopProducts)

	// SSE (Server-Sent Events)
	app.Get("/api/admin/events", authMiddleware, dashboardHandler.SSEEvents)

	log.Println("✓ Routes registered:")
	log.Printf("  GET  /webhook - WhatsApp webhook verification (verify token configured: %v)", cfg.WhatsAppVerifyToken != "")
	log.Println("  POST /webhook - WhatsApp message webhook")
	log.Println("  POST /api/webhooks/payment - Payment webhook")
	log.Println("  POST /api/admin/auth/request-otp - Request OTP")
	log.Println("  POST /api/admin/auth/verify-otp - Verify OTP")
	log.Println("  GET  /api/admin/products - Get products (protected)")
	log.Println("  PATCH /api/admin/products/:id/stock - Update stock (protected)")
	log.Println("  PATCH /api/admin/products/:id/price - Update price (protected)")
	log.Println("  GET  /api/admin/orders - Get orders (protected)")
	log.Println("  GET  /api/admin/analytics/* - Analytics endpoints (protected)")
	log.Println("  GET  /api/admin/events - SSE stream (protected)")
	log.Printf("✓ Server ready on port %s", cfg.AppPort)

	// Start server
	addr := fmt.Sprintf(":%s", cfg.AppPort)
	log.Printf("🚀 Server starting on %s", addr)
	if err := app.Listen(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
