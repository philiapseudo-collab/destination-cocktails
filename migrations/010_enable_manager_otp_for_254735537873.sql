-- Migration: 010_enable_manager_otp_for_254735537873.sql
-- Description: Ensure 254735537873 has MANAGER role (OTP) while retaining PIN for bartender login
-- Created: 2026-02-20

BEGIN;

UPDATE admin_users
SET
    role = 'MANAGER',
    is_active = true,
    pin_hash = COALESCE(
        pin_hash,
        '$2a$10$GcgAM0/eLUzDWNUAjfccHuCCjyXgM2ufgtyvAbS0EApiwLfxTGCDO'
    )
WHERE phone_number = '254735537873';

COMMIT;
