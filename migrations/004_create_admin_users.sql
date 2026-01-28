-- Migration: 004_create_admin_users.sql
-- Description: Create admin_users table for manager dashboard authentication
-- Created: 2026-01-23

-- Admin users table
CREATE TABLE IF NOT EXISTS admin_users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    phone_number VARCHAR(20) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'MANAGER',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create index on phone_number for fast lookups
CREATE INDEX IF NOT EXISTS idx_admin_users_phone_number ON admin_users(phone_number);
CREATE INDEX IF NOT EXISTS idx_admin_users_is_active ON admin_users(is_active);

-- Seed test admin user (phone: 254700000000, OTP will be hardcoded to 123456)
INSERT INTO admin_users (phone_number, name, role, is_active)
VALUES ('254700000000', 'Test Admin', 'OWNER', true)
ON CONFLICT (phone_number) DO NOTHING;
