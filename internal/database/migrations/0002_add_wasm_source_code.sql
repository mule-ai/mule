-- Migration to add WASM source code storage and compilation tracking

-- Create table for storing WASM module source code
CREATE TABLE IF NOT EXISTS wasm_module_sources (
    id VARCHAR(255) PRIMARY KEY,
    wasm_module_id VARCHAR(255) NOT NULL REFERENCES wasm_modules(id) ON DELETE CASCADE,
    language TEXT NOT NULL CHECK (language IN ('go', 'rust', 'javascript', 'python')),
    source_code TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    compilation_status TEXT NOT NULL CHECK (compilation_status IN ('pending', 'compiling', 'success', 'failed')) DEFAULT 'pending',
    compilation_error TEXT,
    compiled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Add index for efficient lookup by WASM module ID
CREATE INDEX IF NOT EXISTS idx_wasm_module_sources_wasm_module_id ON wasm_module_sources(wasm_module_id);
CREATE INDEX IF NOT EXISTS idx_wasm_module_sources_language ON wasm_module_sources(language);
CREATE INDEX IF NOT EXISTS idx_wasm_module_sources_status ON wasm_module_sources(compilation_status);

-- Add foreign key constraint to wasm_modules table to ensure referential integrity
-- (This should already exist from the initial schema, but we'll ensure it's properly indexed)

-- Create a view that combines WASM modules with their latest source code
CREATE OR REPLACE VIEW wasm_modules_with_source AS
SELECT 
    wm.id,
    wm.name,
    wm.description,
    wm.module_data,
    wm.created_at as module_created_at,
    wm.updated_at as module_updated_at,
    wms.id as source_id,
    wms.language,
    wms.source_code,
    wms.version,
    wms.compilation_status,
    wms.compilation_error,
    wms.compiled_at,
    wms.created_at as source_created_at,
    wms.updated_at as source_updated_at
FROM wasm_modules wm
LEFT JOIN wasm_module_sources wms ON wm.id = wms.wasm_module_id
WHERE wms.id IS NULL OR wms.id = (
    SELECT id FROM wasm_module_sources 
    WHERE wasm_module_id = wm.id 
    ORDER BY version DESC, updated_at DESC 
    LIMIT 1
);