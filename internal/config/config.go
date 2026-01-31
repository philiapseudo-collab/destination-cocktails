package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

// DATABASE_URL is Railway's standard environment variable name

// Config holds all application configuration
type Config struct {
	AppPort string `envconfig:"APP_PORT" default:"8080"`
	AppEnv  string `envconfig:"APP_ENV" default:"development"`

	// Database
	DBHost     string `envconfig:"DB_HOST" default:"localhost"`
	DBPort     string `envconfig:"DB_PORT" default:"5432"`
	DBUser     string `envconfig:"DB_USER" default:"postgres"`
	DBPassword string `envconfig:"DB_PASSWORD" default:"postgres"`
	DBName     string `envconfig:"DB_NAME" default:"destination_cocktails"`
	DBURL      string `envconfig:"DB_URL"`

	// Redis
	RedisURL      string `envconfig:"REDIS_URL" default:"redis://localhost:6379"`
	RedisPassword string `envconfig:"REDIS_PASSWORD" default:""`

	// WhatsApp
	WhatsAppToken         string `envconfig:"WHATSAPP_TOKEN"`
	WhatsAppPhoneNumberID string `envconfig:"WHATSAPP_PHONE_NUMBER_ID"`
	WhatsAppVerifyToken   string `envconfig:"WHATSAPP_VERIFY_TOKEN"`

	// Bar Staff
	BarStaffPhone string `envconfig:"BAR_STAFF_PHONE"` // Phone number for bar staff notifications

	// Dashboard
	JWTSecret string `envconfig:"JWT_SECRET" default:"change-this-secret-in-production"`

	// Kopo Kopo (use Client ID + Secret for OAuth; or set Access Token for sandbox manual token)
	KopoKopoClientID        string `envconfig:"KOPOKOPO_CLIENT_ID"`
	KopoKopoClientSecret    string `envconfig:"KOPOKOPO_CLIENT_SECRET"`
	KopoKopoWebhookSecret   string `envconfig:"KOPOKOPO_WEBHOOK_SECRET"`    // Used to verify X-KopoKopo-Signature header
	KopoKopoBaseURL         string `envconfig:"KOPOKOPO_BASE_URL" default:"https://api.kopokopo.com"`
	KopoKopoTillNumber      string `envconfig:"KOPOKOPO_TILL_NUMBER"`
	KopoKopoAccessToken     string `envconfig:"KOPOKOPO_ACCESS_TOKEN"`      // Optional: manual token (e.g. sandbox); else we use Client ID/Secret OAuth
	KopoKopoCallbackURL     string `envconfig:"KOPOKOPO_CALLBACK_URL"`      // Full callback URL (e.g., https://your-app.railway.app/api/webhooks/payment)

	// Pesapal
	PesapalClientID     string `envconfig:"PESAPAL_CLIENT_ID"`
	PesapalClientSecret string `envconfig:"PESAPAL_CLIENT_SECRET"`
	PesapalEnvironment  string `envconfig:"PESAPAL_ENVIRONMENT" default:"sandbox"`
}

var instance *Config

// Load initializes and returns the singleton Config instance
func Load() (*Config, error) {
	if instance != nil {
		return instance, nil
	}

	// Load .env file if it exists (for local development)
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(); err != nil {
			return nil, fmt.Errorf("error loading .env file: %w", err)
		}
	}

	cfg := &Config{}
	if err := envconfig.Process("", cfg); err != nil {
		return nil, fmt.Errorf("error processing environment variables: %w", err)
	}

	// Check for Railway's DATABASE_URL if DB_URL is not set
	if cfg.DBURL == "" {
		if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
			cfg.DBURL = databaseURL
		}
	}

	// Build DBURL if still not provided
	if cfg.DBURL == "" {
		cfg.DBURL = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
			cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName)
	}

	instance = cfg
	return instance, nil
}

// Get returns the singleton Config instance (must call Load first)
func Get() *Config {
	if instance == nil {
		panic("config not loaded: call config.Load() first")
	}
	return instance
}
