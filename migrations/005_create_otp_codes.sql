-- Migration: 005_create_otp_codes.sql
-- Description: Create otp_codes table for WhatsApp OTP authentication
-- Created: 2026-01-23

-- OTP codes table
CREATE TABLE IF NOT EXISTS otp_codes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    phone_number VARCHAR(20) NOT NULL,
    code VARCHAR(6) NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    verified BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for efficient lookups
CREATE INDEX IF NOT EXISTS idx_otp_codes_phone_number ON otp_codes(phone_number);
CREATE INDEX IF NOT EXISTS idx_otp_codes_expires_at ON otp_codes(expires_at);
CREATE INDEX IF NOT EXISTS idx_otp_codes_verified ON otp_codes(verified);

-- Composite index for common query pattern (phone + not verified + not expired)
CREATE INDEX IF NOT EXISTS idx_otp_codes_phone_verified ON otp_codes(phone_number, verified);
