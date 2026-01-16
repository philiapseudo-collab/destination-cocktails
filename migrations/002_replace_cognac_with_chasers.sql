-- Migration: 002_replace_cognac_with_chasers.sql
-- Description: Soft delete Cognac products by archiving them
-- Created: 2026-01-16
-- Strategy: Soft delete to preserve order history (foreign key constraints)

-- Soft delete all Cognac products:
-- 1. Change category to 'Archived' to hide from menu
-- 2. Set is_active to FALSE to ensure they don't appear in queries
-- This preserves order history while removing them from the active menu

UPDATE products
SET 
    category = 'Archived',
    is_active = FALSE,
    updated_at = CURRENT_TIMESTAMP
WHERE category = 'Cognac';

-- Log the number of products archived (for verification)
-- Note: This will show in migration runner output
DO $$
DECLARE
    archived_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO archived_count
    FROM products
    WHERE category = 'Archived' AND is_active = FALSE;
    
    RAISE NOTICE 'Archived % Cognac product(s)', archived_count;
END $$;
