-- Memory configuration table for genai memory tool integration
CREATE TABLE IF NOT EXISTS memory_config (
    id VARCHAR(255) PRIMARY KEY DEFAULT 'default',
    database_url TEXT NOT NULL,
    embedding_provider TEXT NOT NULL DEFAULT 'openai',
    embedding_model TEXT NOT NULL DEFAULT 'text-embedding-ada-002',
    embedding_dims INT NOT NULL DEFAULT 1536,
    default_ttl_seconds INT DEFAULT 0,
    default_top_k INT DEFAULT 5,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Create index for faster lookups
CREATE INDEX IF NOT EXISTS idx_memory_config_updated_at ON memory_config (updated_at);

-- Insert default configuration
INSERT INTO memory_config (id, database_url, embedding_provider, embedding_model, embedding_dims, default_ttl_seconds, default_top_k)
VALUES ('default', 'postgres://mule:mule@localhost:5432/mulev2?sslmode=disable', 'openai', 'text-embedding-ada-002', 1536, 0, 5)
ON CONFLICT (id) DO NOTHING;