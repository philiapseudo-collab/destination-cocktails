package main

import (
	"context"
	"fmt"
	"log"

	"github.com/dumu-tech/destination-cocktails/internal/adapters/http"
	"github.com/dumu-tech/destination-cocktails/internal/adapters/payment"
	"github.com/dumu-tech/destination-cocktails/internal/adapters/postgres"
	"github.com/dumu-tech/destination-cocktails/internal/adapters/redis"
	"github.com/dumu-tech/destination-cocktails/internal/adapters/whatsapp"
	"github.com/dumu-tech/destination-cocktails/internal/config"
	"github.com/dumu-tech/destination-cocktails/internal/service"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	goredis "github.com/redis/go-redis/v9"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database connection
	db, err := postgres.NewRepository(cfg.DBURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	log.Println("✓ Database connected")

	// Initialize Redis client
	redisOpts, err := goredis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("Failed to parse Redis URL: %v", err)
	}
	
	// Override password if specified separately
	if cfg.RedisPassword != "" {
		redisOpts.Password = cfg.RedisPassword
	}
	
	redisClient := goredis.NewClient(redisOpts)
	
	// Test Redis connection
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Println("✓ Redis connected")

	// Initialize Redis session repository
	sessionRepo := redis.NewRepository(redisClient)

	// Initialize WhatsApp client
	whatsappClient := whatsapp.NewClient(
		cfg.WhatsAppToken,
		cfg.WhatsAppPhoneNumberID,
	)
	log.Println("✓ WhatsApp client initialized")

	// Initialize Kopo Kopo payment gateway
	paymentGateway, err := payment.NewClient()
	if err != nil {
		log.Fatalf("Failed to initialize payment gateway: %v", err)
	}
	log.Println("✓ Payment gateway initialized")

	// Initialize repositories
	productRepo := db.ProductRepository()
	orderRepo := db.OrderRepository()
	userRepo := db.UserRepository()

	// Initialize bot service
	botService := service.NewBotService(
		productRepo,
		sessionRepo,
		whatsappClient,
		paymentGateway,
		orderRepo,
		userRepo,
	)
	log.Println("✓ Bot service initialized")

	// Initialize HTTP handler
	httpHandler := http.NewHandler(
		botService,
		paymentGateway,
		orderRepo,
		whatsappClient,
	)
	log.Println("✓ HTTP handler initialized")

	// Create Fiber app
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{
				"error": err.Error(),
			})
		},
	})

	// Middleware
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     "*",
		AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization",
		AllowCredentials: false,
	}))

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"service": "destination-cocktails",
		})
	})

	// WhatsApp webhook routes
	app.Get("/api/webhooks/whatsapp", httpHandler.VerifyWebhook)
	app.Post("/api/webhooks/whatsapp", httpHandler.ReceiveMessage)

	// Payment webhook routes (Kopo Kopo)
	app.Post("/api/webhooks/payment", httpHandler.HandlePaymentWebhook)

	// Start server
	port := cfg.AppPort
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 Server starting on port %s...", port)
	log.Printf("   WhatsApp Webhook: http://localhost:%s/api/webhooks/whatsapp", port)
	log.Printf("   Payment Webhook:  http://localhost:%s/api/webhooks/payment", port)
	log.Printf("   Health Check:     http://localhost:%s/health", port)

	if err := app.Listen(fmt.Sprintf(":%s", port)); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
