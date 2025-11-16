import React, { useState, useEffect } from 'react';
import { Card, Row, Col, Alert, Button, Form } from 'react-bootstrap';
import { chatAPI } from '../services/api';

function Dashboard() {
  const [models, setModels] = useState([]);
  const [selectedModel, setSelectedModel] = useState('');
  const [message, setMessage] = useState('');
  const [chatHistory, setChatHistory] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    loadModels();
  }, []);

  const loadModels = async () => {
    try {
      const response = await chatAPI.models();
      setModels(response.data.data || []);
    } catch (err) {
      setError('Failed to load models');
    }
  };

  const handleSendMessage = async (e) => {
    e.preventDefault();
    if (!selectedModel || !message.trim()) return;

    setLoading(true);
    setError('');

    try {
      const response = await chatAPI.complete({
        model: selectedModel,
        messages: [{ role: 'user', content: message }],
        stream: false,
      });

      const newMessage = {
        role: 'user',
        content: message,
        timestamp: new Date(),
      };

      const responseMessage = {
        role: 'assistant',
        content: response.data.choices?.[0]?.message?.content || 'No response',
        timestamp: new Date(),
      };

      setChatHistory([...chatHistory, newMessage, responseMessage]);
      setMessage('');
    } catch (err) {
      setError(err.response?.data?.error || 'Failed to send message');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <h1>Dashboard</h1>
      <p>Test your AI agents and workflows</p>

      {error && <Alert variant="danger">{error}</Alert>}

      <Row>
        <Col md={6}>
          <Card>
            <Card.Header>
              <Card.Title>Chat Interface</Card.Title>
            </Card.Header>
            <Card.Body>
              <Form onSubmit={handleSendMessage}>
                <Form.Group className="mb-3">
                  <Form.Label>Model</Form.Label>
                  <Form.Select
                    value={selectedModel}
                    onChange={(e) => setSelectedModel(e.target.value)}
                    required
                  >
                    <option value="">Select a model...</option>
                    {models.map((model) => (
                      <option key={model.id} value={model.id}>
                        {model.id}
                      </option>
                    ))}
                  </Form.Select>
                </Form.Group>

                <Form.Group className="mb-3">
                  <Form.Label>Message</Form.Label>
                  <Form.Control
                    as="textarea"
                    rows={3}
                    value={message}
                    onChange={(e) => setMessage(e.target.value)}
                    placeholder="Enter your message..."
                    required
                  />
                </Form.Group>

                <Button
                  type="submit"
                  variant="primary"
                  disabled={loading || !selectedModel || !message.trim()}
                >
                  {loading ? 'Sending...' : 'Send Message'}
                </Button>
              </Form>
            </Card.Body>
          </Card>
        </Col>

        <Col md={6}>
          <Card>
            <Card.Header>
              <Card.Title>Chat History</Card.Title>
            </Card.Header>
            <Card.Body style={{ height: '400px', overflowY: 'auto' }}>
              {chatHistory.length === 0 ? (
                <p className="text-muted">No messages yet</p>
              ) : (
                chatHistory.map((msg, index) => (
                  <div
                    key={index}
                    className={`mb-3 p-2 rounded ${
                      msg.role === 'user'
                        ? 'bg-primary text-white'
                        : 'bg-light text-dark'
                    }`}
                  >
                    <strong>{msg.role}:</strong> {msg.content}
                    <div className="small">
                      {msg.timestamp.toLocaleTimeString()}
                    </div>
                  </div>
                ))
              )}
            </Card.Body>
          </Card>
        </Col>
      </Row>
    </div>
  );
}

export default Dashboard;