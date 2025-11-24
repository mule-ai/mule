import React, { useState, useEffect } from 'react';
import { Card, Form, Button, Alert, Spinner } from 'react-bootstrap';
import { memoryConfigAPI, providersAPI } from '../services/api';

function MemoryConfig() {
  const [config, setConfig] = useState({
    database_url: '',
    embedding_provider: 'openai',
    embedding_model: 'text-embedding-ada-002',
    embedding_dims: 1536,
    default_ttl_seconds: 0,
    default_top_k: 5,
  });
  const [providers, setProviders] = useState([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);
  const [success, setSuccess] = useState(null);

  useEffect(() => {
    loadConfig();
    loadProviders();
  }, []);

  const loadProviders = async () => {
    try {
      const response = await providersAPI.list();
      setProviders(response.data || []);
    } catch (err) {
      console.error('Failed to load providers:', err);
    }
  };

  const loadConfig = async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await memoryConfigAPI.get();
      setConfig({
        database_url: response.database_url || '',
        embedding_provider: response.embedding_provider || 'openai',
        embedding_model: response.embedding_model || 'text-embedding-ada-002',
        embedding_dims: response.embedding_dims || 1536,
        default_ttl_seconds: response.default_ttl_seconds || 0,
        default_top_k: response.default_top_k || 5,
      });
    } catch (err) {
      setError('Failed to load memory configuration: ' + err.message);
    } finally {
      setLoading(false);
    }
  };

  const handleSave = async () => {
    setSaving(true);
    setError(null);
    setSuccess(null);
    try {
      await memoryConfigAPI.update(config);
      setSuccess('Memory configuration saved successfully!');
      setTimeout(() => setSuccess(null), 3000);
    } catch (err) {
      setError('Failed to save memory configuration: ' + err.message);
    } finally {
      setSaving(false);
    }
  };

  const handleChange = (field, value) => {
    setConfig(prev => ({ ...prev, [field]: value }));
  };

  if (loading) {
    return (
      <div className="text-center p-4">
        <Spinner animation="border" />
        <p className="mt-2">Loading memory configuration...</p>
      </div>
    );
  }

  return (
    <Card className="mb-4">
      <Card.Header>
        <Card.Title className="mb-0">Memory Tool Configuration</Card.Title>
      </Card.Header>
      <Card.Body>
        {error && <Alert variant="danger">{error}</Alert>}
        {success && <Alert variant="success">{success}</Alert>}
        
        <Form>
          <Form.Group className="mb-3">
            <Form.Label>Database URL</Form.Label>
            <Form.Control
              type="text"
              value={config.database_url}
              onChange={(e) => handleChange('database_url', e.target.value)}
              placeholder="postgres://user:pass@host:5432/dbname?sslmode=disable"
            />
            <Form.Text className="text-muted">
              PostgreSQL connection string for memory storage
            </Form.Text>
          </Form.Group>

          <Form.Group className="mb-3">
            <Form.Label>Embedding Provider</Form.Label>
            <Form.Select
              value={config.embedding_provider}
              onChange={(e) => handleChange('embedding_provider', e.target.value)}
            >
              <option value="">Select a provider...</option>
              {providers.map((provider) => (
                <option key={provider.id} value={provider.id}>
                  {provider.name}
                </option>
              ))}
            </Form.Select>
            {providers.length === 0 && (
              <Form.Text className="text-muted">
                No providers configured. Please add providers in the Providers section.
              </Form.Text>
            )}
          </Form.Group>

          <Form.Group className="mb-3">
            <Form.Label>Embedding Model</Form.Label>
            <Form.Control
              type="text"
              value={config.embedding_model}
              onChange={(e) => handleChange('embedding_model', e.target.value)}
              placeholder="text-embedding-ada-002"
            />
          </Form.Group>

          <Form.Group className="mb-3">
            <Form.Label>Embedding Dimensions</Form.Label>
            <Form.Control
              type="number"
              value={config.embedding_dims}
              onChange={(e) => handleChange('embedding_dims', parseInt(e.target.value) || 1536)}
              min="1"
              max="4096"
            />
          </Form.Group>

          <Form.Group className="mb-3">
            <Form.Label>Default TTL (seconds)</Form.Label>
            <Form.Control
              type="number"
              value={config.default_ttl_seconds}
              onChange={(e) => handleChange('default_ttl_seconds', parseInt(e.target.value) || 0)}
              min="0"
            />
            <Form.Text className="text-muted">
              Time to live for stored memories (0 = no expiration)
            </Form.Text>
          </Form.Group>

          <Form.Group className="mb-3">
            <Form.Label>Default Top K</Form.Label>
            <Form.Control
              type="number"
              value={config.default_top_k}
              onChange={(e) => handleChange('default_top_k', parseInt(e.target.value) || 5)}
              min="1"
              max="100"
            />
            <Form.Text className="text-muted">
              Number of results to return for memory retrieval
            </Form.Text>
          </Form.Group>

          <Button
            variant="primary"
            onClick={handleSave}
            disabled={saving}
          >
            {saving ? 'Saving...' : 'Save Configuration'}
          </Button>
        </Form>
      </Card.Body>
    </Card>
  );
}

export default MemoryConfig;