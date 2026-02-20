-- Migration: 011_add_order_audit_fields_for_history.sql
-- Description: Add order lifecycle audit columns for bartender dispute history
-- Created: 2026-02-20

BEGIN;

ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS ready_at TIMESTAMP,
    ADD COLUMN IF NOT EXISTS ready_by_admin_user_id UUID,
    ADD COLUMN IF NOT EXISTS completed_at TIMESTAMP,
    ADD COLUMN IF NOT EXISTS completed_by_admin_user_id UUID;

CREATE INDEX IF NOT EXISTS idx_orders_ready_at ON orders(ready_at);
CREATE INDEX IF NOT EXISTS idx_orders_completed_at ON orders(completed_at);
CREATE INDEX IF NOT EXISTS idx_orders_completed_by_admin_user_id ON orders(completed_by_admin_user_id);

-- Backfill lifecycle timestamps for already-processed orders.
UPDATE orders
SET ready_at = COALESCE(ready_at, updated_at)
WHERE status = 'READY';

UPDATE orders
SET completed_at = COALESCE(completed_at, updated_at)
WHERE status = 'COMPLETED';

COMMIT;
