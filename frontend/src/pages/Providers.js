import React, { useState, useEffect } from 'react';
import { Card, Row, Col, Button, Form, Modal } from 'react-bootstrap';
import { providersAPI } from '../services/api';

function Providers() {
  const [providers, setProviders] = useState([]);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showEditModal, setShowEditModal] = useState(false);
  const [selectedProvider, setSelectedProvider] = useState(null);
  const [newProvider, setNewProvider] = useState({
    name: '',
    api_base_url: '',
    api_key_encrypted: '',
  });
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    loadProviders();
  }, []);

  const loadProviders = async () => {
    try {
      const response = await providersAPI.list();
      setProviders(response.data || []);
    } catch (error) {
      console.error('Failed to load providers:', error);
    }
  };

  const handleCreateProvider = async () => {
    setLoading(true);
    try {
      await providersAPI.create(newProvider);
      setShowCreateModal(false);
      setNewProvider({
        name: '',
        api_base_url: '',
        api_key_encrypted: '',
      });
      loadProviders();
    } catch (error) {
      console.error('Failed to create provider:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleUpdateProvider = async () => {
    setLoading(true);
    try {
      await providersAPI.update(selectedProvider.id, selectedProvider);
      setShowEditModal(false);
      setSelectedProvider(null);
      loadProviders();
    } catch (error) {
      console.error('Failed to update provider:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleDeleteProvider = async (providerId) => {
    if (window.confirm('Are you sure you want to delete this provider?')) {
      try {
        await providersAPI.delete(providerId);
        loadProviders();
      } catch (error) {
        console.error('Failed to delete provider:', error);
      }
    }
  };

  const openEditModal = (provider) => {
    setSelectedProvider(provider);
    setShowEditModal(true);
  };

  return (
    <div>
      <div className="d-flex justify-content-between align-items-center mb-4">
        <h1>Providers</h1>
        <Button variant="primary" onClick={() => setShowCreateModal(true)}>
          Add Provider
        </Button>
      </div>

      <Row>
        {providers.map((provider) => (
          <Col md={6} lg={4} className="mb-4" key={provider.id}>
            <Card>
              <Card.Header>
                <Card.Title className="mb-0">{provider.name}</Card.Title>
              </Card.Header>
              <Card.Body>
                <div className="mb-2">
                  <strong>API Base URL:</strong>
                  <div className="small text-muted">{provider.api_base_url}</div>
                </div>
                <div className="mb-3">
                  <strong>API Key:</strong>
                  <div className="small text-muted">
                    {provider.api_key_encrypted ? '••••••••' : 'Not set'}
                  </div>
                </div>
                <div className="d-flex gap-2">
                  <Button
                    variant="outline-primary"
                    size="sm"
                    onClick={() => openEditModal(provider)}
                  >
                    Edit
                  </Button>
                  <Button
                    variant="outline-danger"
                    size="sm"
                    onClick={() => handleDeleteProvider(provider.id)}
                  >
                    Delete
                  </Button>
                </div>
              </Card.Body>
            </Card>
          </Col>
        ))}
      </Row>

      {/* Create Provider Modal */}
      <Modal show={showCreateModal} onHide={() => setShowCreateModal(false)}>
        <Modal.Header closeButton>
          <Modal.Title>Add New Provider</Modal.Title>
        </Modal.Header>
        <Modal.Body>
          <Form>
            <Form.Group className="mb-3">
              <Form.Label>Name</Form.Label>
              <Form.Control
                type="text"
                value={newProvider.name}
                onChange={(e) =>
                  setNewProvider({ ...newProvider, name: e.target.value })
                }
                placeholder="e.g., OpenAI, Google"
                required
              />
            </Form.Group>
            <Form.Group className="mb-3">
              <Form.Label>API Base URL</Form.Label>
              <Form.Control
                type="url"
                value={newProvider.api_base_url}
                onChange={(e) =>
                  setNewProvider({ ...newProvider, api_base_url: e.target.value })
                }
                placeholder="e.g., https://api.openai.com/v1"
                required
              />
            </Form.Group>
            <Form.Group className="mb-3">
              <Form.Label>API Key</Form.Label>
              <Form.Control
                type="password"
                value={newProvider.api_key_encrypted}
                onChange={(e) =>
                  setNewProvider({ ...newProvider, api_key_encrypted: e.target.value })
                }
                placeholder="Enter your API key"
                required
              />
            </Form.Group>
          </Form>
        </Modal.Body>
        <Modal.Footer>
          <Button variant="secondary" onClick={() => setShowCreateModal(false)}>
            Cancel
          </Button>
          <Button variant="primary" onClick={handleCreateProvider} disabled={loading}>
            {loading ? 'Creating...' : 'Create'}
          </Button>
        </Modal.Footer>
      </Modal>

      {/* Edit Provider Modal */}
      <Modal show={showEditModal} onHide={() => setShowEditModal(false)}>
        <Modal.Header closeButton>
          <Modal.Title>Edit Provider</Modal.Title>
        </Modal.Header>
        <Modal.Body>
          {selectedProvider && (
            <Form>
              <Form.Group className="mb-3">
                <Form.Label>Name</Form.Label>
                <Form.Control
                  type="text"
                  value={selectedProvider.name}
                  onChange={(e) =>
                    setSelectedProvider({ ...selectedProvider, name: e.target.value })
                  }
                  required
                />
              </Form.Group>
              <Form.Group className="mb-3">
                <Form.Label>API Base URL</Form.Label>
                <Form.Control
                  type="url"
                  value={selectedProvider.api_base_url}
                  onChange={(e) =>
                    setSelectedProvider({ ...selectedProvider, api_base_url: e.target.value })
                  }
                  required
                />
              </Form.Group>
              <Form.Group className="mb-3">
                <Form.Label>API Key</Form.Label>
                <Form.Control
                  type="password"
                  value={selectedProvider.api_key_encrypted}
                  onChange={(e) =>
                    setSelectedProvider({ ...selectedProvider, api_key_encrypted: e.target.value })
                  }
                  placeholder="Enter new API key (leave blank to keep current)"
                />
              </Form.Group>
            </Form>
          )}
        </Modal.Body>
        <Modal.Footer>
          <Button variant="secondary" onClick={() => setShowEditModal(false)}>
            Cancel
          </Button>
          <Button variant="primary" onClick={handleUpdateProvider} disabled={loading}>
            {loading ? 'Updating...' : 'Update'}
          </Button>
        </Modal.Footer>
      </Modal>
    </div>
  );
}

export default Providers;