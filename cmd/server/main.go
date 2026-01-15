package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/dumu-tech/destination-cocktails/internal/adapters/http"
	"github.com/dumu-tech/destination-cocktails/internal/adapters/payment"
	"github.com/dumu-tech/destination-cocktails/internal/adapters/postgres"
	redisRepo "github.com/dumu-tech/destination-cocktails/internal/adapters/redis"
	"github.com/dumu-tech/destination-cocktails/internal/adapters/whatsapp"
	"github.com/dumu-tech/destination-cocktails/internal/config"
	"github.com/dumu-tech/destination-cocktails/internal/service"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/dumu-tech/destination-cocktails/internal/core"
)

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

	// Connect to Redis
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("Failed to parse Redis URL: %v", err)
	}

	if cfg.RedisPassword != "" {
		redisOpts.Password = cfg.RedisPassword
	}

	rdb := redis.NewClient(redisOpts)
	defer rdb.Close()

	// Ping Redis to verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Println("✓ Redis connection established")

	// Connect to PostgreSQL
	dbpool, err := pgxpool.New(ctx, cfg.DBURL)
	if err != nil {
		log.Fatalf("Failed to create connection pool: %v", err)
	}
	defer dbpool.Close()

	// Ping PostgreSQL to verify connection
	if err := dbpool.Ping(ctx); err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}
	log.Println("✓ PostgreSQL connection established")

	// Initialize repositories using GORM (we still need pgxpool for some operations)
	postgresRepo, err := postgres.NewRepository(cfg.DBURL)
	if err != nil {
		log.Fatalf("Failed to initialize postgres repository: %v", err)
	}

	// Initialize Redis repository
	redisRepository := redisRepo.NewRepository(rdb)

	// Initialize WhatsApp client
	whatsappClient := whatsapp.NewClient(cfg.WhatsAppPhoneNumberID, cfg.WhatsAppToken)

	// Initialize Payment client
	paymentClient, err := payment.NewClient()
	if err != nil {
		log.Fatalf("Failed to initialize payment client: %v", err)
	}

	// Initialize Bot Service
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

	// Initialize Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "Destination Cocktails API",
		ServerHeader: "Fiber",
	})

	// Middleware
	app.Use(recover.New())
	app.Use(logger.New())

	// Health check route
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"project": "destination-cocktails",
		})
	})

	// WhatsApp Webhook routes
	// GET for webhook verification (WhatsApp requires this)
	app.Get("/webhook", httpHandler.VerifyWebhook)
	// POST for receiving messages
	app.Post("/webhook", httpHandler.ReceiveMessage)

	// Payment webhook route
	app.Post("/api/webhooks/payment", httpHandler.HandlePaymentWebhook)

	log.Println("✓ Routes registered:")
	log.Println("  GET  /webhook - WhatsApp webhook verification")
	log.Println("  POST /webhook - WhatsApp message webhook")
	log.Println("  POST /api/webhooks/payment - Payment webhook")

	// Start server
	addr := fmt.Sprintf(":%s", cfg.AppPort)
	log.Printf("🚀 Server starting on %s", addr)
	if err := app.Listen(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
