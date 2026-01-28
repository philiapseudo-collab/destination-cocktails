-- Migration: 003_add_pickup_code_and_completed_status.sql
-- Description: Add pickup_code column to orders table for bar staff workflow
-- Created: 2026-01-23

-- Add pickup_code column to orders table
ALTER TABLE orders ADD COLUMN IF NOT EXISTS pickup_code VARCHAR(4);

-- Create index for pickup code lookups
CREATE INDEX IF NOT EXISTS idx_orders_pickup_code ON orders(pickup_code);

-- Note: PostgreSQL doesn't require ALTER TYPE for VARCHAR status column
-- The COMPLETED status will be handled at application level
-- Existing status column already accepts VARCHAR(20), so 'COMPLETED' will work
