-- Migration: 007_add_admin_254735537873.sql
-- Description: Add admin user for Barman/Admin (phone: 254735537873)
-- Created: 2026-02-06

-- Add admin user with OWNER role
INSERT INTO admin_users (phone_number, name, role, is_active)
VALUES ('254735537873', 'Barman / Admin', 'OWNER', true)
ON CONFLICT (phone_number) DO UPDATE SET
    is_active = true,
    role = 'OWNER',
    name = 'Barman / Admin';

-- Verify the admin was added
SELECT * FROM admin_users WHERE phone_number = '254735537873';
