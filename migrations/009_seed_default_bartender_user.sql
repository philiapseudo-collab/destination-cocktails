-- Migration: 009_seed_default_bartender_user.sql
-- Description: Seed one active bartender account for PIN login
-- Created: 2026-02-20

BEGIN;

-- Default tablet bartender account.
-- PIN (plaintext for initial setup): 1234
-- Rotate this PIN immediately after first successful login.
DELETE FROM admin_users
WHERE phone_number = '254700000001';

INSERT INTO admin_users (phone_number, name, role, pin_hash, is_active)
VALUES (
    '254735537873',
    'Bar Tablet',
    'BARTENDER',
    '$2a$10$GcgAM0/eLUzDWNUAjfccHuCCjyXgM2ufgtyvAbS0EApiwLfxTGCDO',
    true
)
ON CONFLICT (phone_number) DO UPDATE SET
    name = EXCLUDED.name,
    role = 'BARTENDER',
    pin_hash = EXCLUDED.pin_hash,
    is_active = true;

COMMIT;
