package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/dumu-tech/destination-cocktails/internal/config"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

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

	// Start server
	addr := fmt.Sprintf(":%s", cfg.AppPort)
	log.Printf("🚀 Server starting on %s", addr)
	if err := app.Listen(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
