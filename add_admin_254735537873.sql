-- Quick SQL to add admin user 254735537873
-- Run this directly via Railway Query tab or psql

INSERT INTO admin_users (phone_number, name, role, is_active)
VALUES ('254735537873', 'Barman / Admin', 'OWNER', true)
ON CONFLICT (phone_number) DO UPDATE SET
    is_active = true,
    role = 'OWNER',
    name = 'Barman / Admin';

-- Verify the admin was added
SELECT * FROM admin_users WHERE phone_number = '254735537873';
