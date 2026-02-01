-- Migration: 005_add_admin_254708116809.sql
-- Description: Add authorized admin user with phone 254708116809
-- Created: 2026-01-28

-- Add the authorized admin user
INSERT INTO admin_users (phone_number, name, role, is_active)
VALUES ('254708116809', 'Manager', 'OWNER', true)
ON CONFLICT (phone_number) DO UPDATE SET
    is_active = true,
    role = 'OWNER';
