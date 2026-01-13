package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/dumu-tech/destination-cocktails/internal/config"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// MenuItem represents a product in the seed data JSON
type MenuItem struct {
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Category string  `json:"category"`
	Stock    int     `json:"stock"`
}

// MenuData holds the menu items to be seeded
var MenuData = []byte(`[
  { "name": "Destination Island Tea", "price": 800, "category": "Cocktails", "stock": 100 },
  { "name": "Dawa Daktar", "price": 500, "category": "Cocktails", "stock": 100 },
  { "name": "Blue Lagoon", "price": 500, "category": "Cocktails", "stock": 100 },
  { "name": "Tequila Sunrise", "price": 550, "category": "Cocktails", "stock": 100 },
  { "name": "Gin & Juice", "price": 450, "category": "Cocktails", "stock": 100 },
  { "name": "Classic Mojito", "price": 600, "category": "Cocktails", "stock": 100 },
  { "name": "Screwdriver", "price": 450, "category": "Cocktails", "stock": 100 },
  { "name": "Whisky Sour", "price": 550, "category": "Cocktails", "stock": 100 },
  { "name": "The Rum Punch", "price": 500, "category": "Cocktails", "stock": 100 },
  { "name": "Black Russian", "price": 500, "category": "Cocktails", "stock": 100 },
  { "name": "Gilbey's Special Dry Gin (750ml)", "price": 3000, "category": "Gin", "stock": 50 },
  { "name": "Gilbey's Mixed Berry (750ml)", "price": 3200, "category": "Gin", "stock": 50 },
  { "name": "Chrome Gin Original (750ml)", "price": 1500, "category": "Gin", "stock": 50 },
  { "name": "Chrome Gin Original (250ml)", "price": 450, "category": "Gin", "stock": 50 },
  { "name": "Best Gin (750ml)", "price": 1800, "category": "Gin", "stock": 50 },
  { "name": "Gordon's Dry Gin (750ml)", "price": 3500, "category": "Gin", "stock": 50 },
  { "name": "Tanqueray London Dry (750ml)", "price": 4500, "category": "Gin", "stock": 50 },
  { "name": "Tanqueray Sevilla (750ml)", "price": 5000, "category": "Gin", "stock": 50 },
  { "name": "Beefeater Gin (750ml)", "price": 4200, "category": "Gin", "stock": 50 },
  { "name": "Bombay Sapphire (750ml)", "price": 4800, "category": "Gin", "stock": 50 },
  { "name": "Kenya Cane Original (750ml)", "price": 1500, "category": "Spirits", "stock": 50 },
  { "name": "Kenya Cane Original (250ml)", "price": 450, "category": "Spirits", "stock": 50 },
  { "name": "Kenya Cane Coconut (750ml)", "price": 1500, "category": "Spirits", "stock": 50 },
  { "name": "Kenya Cane Coconut (250ml)", "price": 450, "category": "Spirits", "stock": 50 },
  { "name": "Kenya Cane Pineapple (750ml)", "price": 1500, "category": "Spirits", "stock": 50 },
  { "name": "Kenya Cane Pineapple (250ml)", "price": 450, "category": "Spirits", "stock": 50 },
  { "name": "Kenya Cane Citrus (750ml)", "price": 1500, "category": "Spirits", "stock": 50 },
  { "name": "Kenya Cane Citrus (250ml)", "price": 450, "category": "Spirits", "stock": 50 },
  { "name": "Konyagi (750ml)", "price": 1600, "category": "Spirits", "stock": 50 },
  { "name": "Konyagi (250ml)", "price": 500, "category": "Spirits", "stock": 50 },
  { "name": "Captain Morgan Spiced Gold (750ml)", "price": 3000, "category": "Rum", "stock": 50 },
  { "name": "Captain Morgan Dark Rum (750ml)", "price": 3000, "category": "Rum", "stock": 50 },
  { "name": "Myers Original Dark Rum (750ml)", "price": 3500, "category": "Rum", "stock": 50 },
  { "name": "Malibu Coconut Rum (750ml)", "price": 2800, "category": "Rum", "stock": 50 },
  { "name": "Bacardi White Rum (750ml)", "price": 3200, "category": "Rum", "stock": 50 },
  { "name": "Chrome Vodka (750ml)", "price": 1500, "category": "Vodka", "stock": 50 },
  { "name": "Chrome Vodka (250ml)", "price": 450, "category": "Vodka", "stock": 50 },
  { "name": "Kibao Vodka (750ml)", "price": 1400, "category": "Vodka", "stock": 50 },
  { "name": "Kibao Vodka (250ml)", "price": 400, "category": "Vodka", "stock": 50 },
  { "name": "Smirnoff Red Label (750ml)", "price": 2500, "category": "Vodka", "stock": 50 },
  { "name": "Skyy Vodka (750ml)", "price": 3000, "category": "Vodka", "stock": 50 },
  { "name": "Absolut Blue (750ml)", "price": 3500, "category": "Vodka", "stock": 50 },
  { "name": "Cîroc Vodka (750ml)", "price": 6500, "category": "Vodka", "stock": 50 },
  { "name": "Johnnie Walker Red Label (750ml)", "price": 3000, "category": "Whisky", "stock": 50 },
  { "name": "Johnnie Walker Black Label (750ml)", "price": 5500, "category": "Whisky", "stock": 50 },
  { "name": "Johnnie Walker Double Black (750ml)", "price": 6500, "category": "Whisky", "stock": 50 },
  { "name": "Bond 7 Whisky (750ml)", "price": 2200, "category": "Whisky", "stock": 50 },
  { "name": "Hunter's Choice (750ml)", "price": 2200, "category": "Whisky", "stock": 50 },
  { "name": "William Lawson (750ml)", "price": 2800, "category": "Whisky", "stock": 50 },
  { "name": "VAT 69 (750ml)", "price": 2500, "category": "Whisky", "stock": 50 },
  { "name": "Ballantine's Finest (750ml)", "price": 3000, "category": "Whisky", "stock": 50 },
  { "name": "Jameson Irish Whiskey (750ml)", "price": 5000, "category": "Whisky", "stock": 50 },
  { "name": "Black & White (750ml)", "price": 2200, "category": "Whisky", "stock": 50 },
  { "name": "County Brandy (750ml)", "price": 1200, "category": "Brandy", "stock": 50 },
  { "name": "Richot Brandy (750ml)", "price": 2500, "category": "Brandy", "stock": 50 },
  { "name": "Viceroy Brandy (750ml)", "price": 2800, "category": "Brandy", "stock": 50 },
  { "name": "Hennessy VS (750ml)", "price": 8500, "category": "Cognac", "stock": 20 },
  { "name": "Jose Cuervo Tequila (Shot)", "price": 250, "category": "Shots", "stock": 100 },
  { "name": "Amarula Cream (Shot)", "price": 250, "category": "Shots", "stock": 100 },
  { "name": "Baileys Delight (Shot)", "price": 200, "category": "Shots", "stock": 100 },
  { "name": "Jagermeister (Shot)", "price": 300, "category": "Shots", "stock": 100 }
]`)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Connect to database
	db, err := gorm.Open(postgres.Open(cfg.DBURL), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Parse menu data
	var menuItems []MenuItem
	if err := json.Unmarshal(MenuData, &menuItems); err != nil {
		log.Fatalf("Failed to parse menu data: %v", err)
	}

	if len(menuItems) == 0 {
		log.Println("MenuData is empty. No products to seed.")
		return
	}

	ctx := context.Background()
	upserted := 0
	inserted := 0
	updated := 0

	// Upsert products (update if exists by name, insert if not)
	for _, item := range menuItems {
		// Generate UUID for the product
		productID := uuid.New().String()

		// Check if product with this name already exists
		var existingID string
		result := db.WithContext(ctx).Table("products").
			Select("id").
			Where("name = ?", item.Name).
			Limit(1).
			Scan(&existingID)

		if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
			log.Fatalf("Failed to check existing product %s: %v", item.Name, result.Error)
		}

		productMap := map[string]interface{}{
			"name":           item.Name,
			"description":    "", // Default empty string
			"price":          item.Price,
			"category":       item.Category,
			"stock_quantity": item.Stock, // Map "stock" to "stock_quantity"
			"image_url":      "",          // Default empty string
			"is_active":     true,        // Default true
		}

		if existingID != "" {
			// Update existing product
			productMap["id"] = existingID
			if err := db.WithContext(ctx).Table("products").
				Where("id = ?", existingID).
				Updates(map[string]interface{}{
					"price":          item.Price,
					"stock_quantity": item.Stock,
					"updated_at":     gorm.Expr("CURRENT_TIMESTAMP"),
				}).Error; err != nil {
				log.Fatalf("Failed to update product %s: %v", item.Name, err)
			}
			updated++
		} else {
			// Insert new product
			productMap["id"] = productID
			if err := db.WithContext(ctx).Table("products").Create(productMap).Error; err != nil {
				log.Fatalf("Failed to insert product %s: %v", item.Name, err)
			}
			inserted++
		}
		upserted++
	}

	log.Printf("Seeder completed: %d products processed (%d inserted, %d updated)", upserted, inserted, updated)
}
