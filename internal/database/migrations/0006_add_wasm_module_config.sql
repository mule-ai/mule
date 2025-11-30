-- Add config column to wasm_modules table
ALTER TABLE wasm_modules ADD COLUMN IF NOT EXISTS config JSONB;