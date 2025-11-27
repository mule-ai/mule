import React, { useState, useEffect } from 'react';
import { Card, Form, Button, Alert, Spinner } from 'react-bootstrap';
import api from '../services/api';

function ToolExecutionConfig() {
  const [maxToolCalls, setMaxToolCalls] = useState(10);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);
  const [success, setSuccess] = useState(null);

  useEffect(() => {
    loadConfig();
  }, []);

  const loadConfig = async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await api.get('/api/v1/settings/max_tool_calls');
      const value = parseInt(response.data.value) || 10;
      setMaxToolCalls(value);
    } catch (err) {
      // If setting doesn't exist, use default
      if (err.response && err.response.status === 404) {
        setMaxToolCalls(10);
      } else {
        setError('Failed to load tool execution configuration: ' + err.message);
      }
    } finally {
      setLoading(false);
    }
  };

  const handleChange = (e) => {
    setMaxToolCalls(parseInt(e.target.value) || 10);
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setSaving(true);
    setError(null);
    setSuccess(null);

    try {
      const settingData = {
        key: 'max_tool_calls',
        value: maxToolCalls.toString(),
        description: 'Maximum number of tool calls allowed in a single agent execution',
        category: 'agent'
      };

      await api.put('/api/v1/settings/max_tool_calls', settingData);
      setSuccess('Tool execution configuration updated successfully!');
    } catch (err) {
      setError('Failed to update tool execution configuration: ' + err.message);
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <div className="text-center">
        <Spinner animation="border" />
      </div>
    );
  }

  return (
    <Card>
      <Card.Header>
        <h5>Tool Execution Configuration</h5>
      </Card.Header>
      <Card.Body>
        {error && <Alert variant="danger">{error}</Alert>}
        {success && <Alert variant="success">{success}</Alert>}

        <Form onSubmit={handleSubmit}>
          <Form.Group className="mb-3">
            <Form.Label>Maximum Tool Calls</Form.Label>
            <Form.Control
              type="number"
              name="max_tool_calls"
              value={maxToolCalls}
              onChange={handleChange}
              min="1"
              max="100"
              required
            />
            <Form.Text className="text-muted">
              Maximum number of tool calls allowed in a single agent execution (1-100)
            </Form.Text>
          </Form.Group>

          <Button variant="primary" type="submit" disabled={saving}>
            {saving ? 'Saving...' : 'Save Configuration'}
          </Button>
        </Form>
      </Card.Body>
    </Card>
  );
}

export default ToolExecutionConfig;