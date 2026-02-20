-- Migration: 008_add_bartender_pin_and_ready_status.sql
-- Description: Add bartender PIN support and normalize order/admin statuses for RBAC workflow
-- Created: 2026-02-20

BEGIN;

-- Add nullable bcrypt hash field for bartender PIN authentication.
ALTER TABLE admin_users
    ADD COLUMN IF NOT EXISTS pin_hash VARCHAR(255);

-- Normalize role values to the new RBAC contract.
UPDATE admin_users
SET role = UPPER(role)
WHERE role IS NOT NULL;

UPDATE admin_users
SET role = 'MANAGER'
WHERE role IS NULL OR role NOT IN ('MANAGER', 'BARTENDER');

ALTER TABLE admin_users
    ALTER COLUMN role SET DEFAULT 'MANAGER';

ALTER TABLE admin_users
    ALTER COLUMN role SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_admin_users_role ON admin_users(role);

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_admin_users_role'
    ) THEN
        ALTER TABLE admin_users DROP CONSTRAINT chk_admin_users_role;
    END IF;
END $$;

ALTER TABLE admin_users
    ADD CONSTRAINT chk_admin_users_role CHECK (role IN ('MANAGER', 'BARTENDER'));

-- Replace legacy SERVED status with READY for the new 3-stage flow.
UPDATE orders
SET status = 'READY'
WHERE status = 'SERVED';

COMMIT;
