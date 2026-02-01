-- Quick SQL to add authorized admin user
-- Run this directly in your PostgreSQL database or via Railway CLI

INSERT INTO admin_users (phone_number, name, role, is_active)
VALUES ('254708116809', 'Manager', 'OWNER', true)
ON CONFLICT (phone_number) DO UPDATE SET
    is_active = true,
    role = 'OWNER';

-- Verify the admin was added
SELECT * FROM admin_users WHERE phone_number = '254708116809';
