import React, { useState, useEffect } from 'react';
import { Card, Row, Col, Button, Form, Modal, ListGroup } from 'react-bootstrap';
import { agentsAPI, providersAPI } from '../services/api';

function Agents() {
  const [agents, setAgents] = useState([]);
  const [providers, setProviders] = useState([]);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showEditModal, setShowEditModal] = useState(false);
  const [selectedAgent, setSelectedAgent] = useState(null);
  const [newAgent, setNewAgent] = useState({
    name: '',
    description: '',
    provider_id: '',
    model_id: '',
    system_prompt: '',
  });
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    loadAgents();
    loadProviders();
  }, []);

  const loadAgents = async () => {
    try {
      const response = await agentsAPI.list();
      setAgents(response.data || []);
    } catch (error) {
      console.error('Failed to load agents:', error);
    }
  };

  const loadProviders = async () => {
    try {
      const response = await providersAPI.list();
      setProviders(response.data || []);
    } catch (error) {
      console.error('Failed to load providers:', error);
    }
  };

  const handleCreateAgent = async () => {
    setLoading(true);
    try {
      await agentsAPI.create(newAgent);
      setShowCreateModal(false);
      setNewAgent({
        name: '',
        description: '',
        provider_id: '',
        model_id: '',
        system_prompt: '',
      });
      loadAgents();
    } catch (error) {
      console.error('Failed to create agent:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleUpdateAgent = async () => {
    setLoading(true);
    try {
      await agentsAPI.update(selectedAgent.id, selectedAgent);
      setShowEditModal(false);
      setSelectedAgent(null);
      loadAgents();
    } catch (error) {
      console.error('Failed to update agent:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleDeleteAgent = async (agentId) => {
    if (window.confirm('Are you sure you want to delete this agent?')) {
      try {
        await agentsAPI.delete(agentId);
        loadAgents();
      } catch (error) {
        console.error('Failed to delete agent:', error);
      }
    }
  };

  const openEditModal = (agent) => {
    setSelectedAgent(agent);
    setShowEditModal(true);
  };

  return (
    <div>
      <div className="d-flex justify-content-between align-items-center mb-4">
        <h1>Agents</h1>
        <Button variant="primary" onClick={() => setShowCreateModal(true)}>
          Create Agent
        </Button>
      </div>

      <Row>
        {agents.map((agent) => (
          <Col md={6} lg={4} className="mb-4" key={agent.id}>
            <Card>
              <Card.Header>
                <Card.Title className="mb-0">{agent.name}</Card.Title>
              </Card.Header>
              <Card.Body>
                <p className="text-muted">{agent.description}</p>
                <div className="mb-2">
                  <strong>Provider:</strong>{' '}
                  {providers.find((p) => p.id === agent.provider_id)?.name || 'Unknown'}
                </div>
                <div className="mb-2">
                  <strong>Model:</strong> {agent.model_id}
                </div>
                <div className="mb-3">
                  <strong>System Prompt:</strong>
                  <div className="small text-muted mt-1">
                    {agent.system_prompt?.substring(0, 100)}
                    {agent.system_prompt?.length > 100 && '...'}
                  </div>
                </div>
                <div className="d-flex gap-2">
                  <Button
                    variant="outline-primary"
                    size="sm"
                    onClick={() => openEditModal(agent)}
                  >
                    Edit
                  </Button>
                  <Button
                    variant="outline-danger"
                    size="sm"
                    onClick={() => handleDeleteAgent(agent.id)}
                  >
                    Delete
                  </Button>
                </div>
              </Card.Body>
            </Card>
          </Col>
        ))}
      </Row>

      {/* Create Agent Modal */}
      <Modal show={showCreateModal} onHide={() => setShowCreateModal(false)}>
        <Modal.Header closeButton>
          <Modal.Title>Create New Agent</Modal.Title>
        </Modal.Header>
        <Modal.Body>
          <Form>
            <Form.Group className="mb-3">
              <Form.Label>Name</Form.Label>
              <Form.Control
                type="text"
                value={newAgent.name}
                onChange={(e) =>
                  setNewAgent({ ...newAgent, name: e.target.value })
                }
                required
              />
            </Form.Group>
            <Form.Group className="mb-3">
              <Form.Label>Description</Form.Label>
              <Form.Control
                as="textarea"
                rows={2}
                value={newAgent.description}
                onChange={(e) =>
                  setNewAgent({ ...newAgent, description: e.target.value })
                }
              />
            </Form.Group>
            <Form.Group className="mb-3">
              <Form.Label>Provider</Form.Label>
              <Form.Select
                value={newAgent.provider_id}
                onChange={(e) =>
                  setNewAgent({ ...newAgent, provider_id: e.target.value })
                }
                required
              >
                <option value="">Select a provider...</option>
                {providers.map((provider) => (
                  <option key={provider.id} value={provider.id}>
                    {provider.name}
                  </option>
                ))}
              </Form.Select>
            </Form.Group>
            <Form.Group className="mb-3">
              <Form.Label>Model ID</Form.Label>
              <Form.Control
                type="text"
                value={newAgent.model_id}
                onChange={(e) =>
                  setNewAgent({ ...newAgent, model_id: e.target.value })
                }
                placeholder="e.g., gpt-4, gemini-pro"
                required
              />
            </Form.Group>
            <Form.Group className="mb-3">
              <Form.Label>System Prompt</Form.Label>
              <Form.Control
                as="textarea"
                rows={4}
                value={newAgent.system_prompt}
                onChange={(e) =>
                  setNewAgent({ ...newAgent, system_prompt: e.target.value })
                }
                placeholder="Enter the system prompt that defines the agent's behavior..."
              />
            </Form.Group>
          </Form>
        </Modal.Body>
        <Modal.Footer>
          <Button variant="secondary" onClick={() => setShowCreateModal(false)}>
            Cancel
          </Button>
          <Button variant="primary" onClick={handleCreateAgent} disabled={loading}>
            {loading ? 'Creating...' : 'Create'}
          </Button>
        </Modal.Footer>
      </Modal>

      {/* Edit Agent Modal */}
      <Modal show={showEditModal} onHide={() => setShowEditModal(false)}>
        <Modal.Header closeButton>
          <Modal.Title>Edit Agent</Modal.Title>
        </Modal.Header>
        <Modal.Body>
          {selectedAgent && (
            <Form>
              <Form.Group className="mb-3">
                <Form.Label>Name</Form.Label>
                <Form.Control
                  type="text"
                  value={selectedAgent.name}
                  onChange={(e) =>
                    setSelectedAgent({ ...selectedAgent, name: e.target.value })
                  }
                  required
                />
              </Form.Group>
              <Form.Group className="mb-3">
                <Form.Label>Description</Form.Label>
                <Form.Control
                  as="textarea"
                  rows={2}
                  value={selectedAgent.description}
                  onChange={(e) =>
                    setSelectedAgent({ ...selectedAgent, description: e.target.value })
                  }
                />
              </Form.Group>
              <Form.Group className="mb-3">
                <Form.Label>Provider</Form.Label>
                <Form.Select
                  value={selectedAgent.provider_id}
                  onChange={(e) =>
                    setSelectedAgent({ ...selectedAgent, provider_id: e.target.value })
                  }
                  required
                >
                  <option value="">Select a provider...</option>
                  {providers.map((provider) => (
                    <option key={provider.id} value={provider.id}>
                      {provider.name}
                    </option>
                  ))}
                </Form.Select>
              </Form.Group>
              <Form.Group className="mb-3">
                <Form.Label>Model ID</Form.Label>
                <Form.Control
                  type="text"
                  value={selectedAgent.model_id}
                  onChange={(e) =>
                    setSelectedAgent({ ...selectedAgent, model_id: e.target.value })
                  }
                  required
                />
              </Form.Group>
              <Form.Group className="mb-3">
                <Form.Label>System Prompt</Form.Label>
                <Form.Control
                  as="textarea"
                  rows={4}
                  value={selectedAgent.system_prompt}
                  onChange={(e) =>
                    setSelectedAgent({ ...selectedAgent, system_prompt: e.target.value })
                  }
                />
              </Form.Group>
            </Form>
          )}
        </Modal.Body>
        <Modal.Footer>
          <Button variant="secondary" onClick={() => setShowEditModal(false)}>
            Cancel
          </Button>
          <Button variant="primary" onClick={handleUpdateAgent} disabled={loading}>
            {loading ? 'Updating...' : 'Update'}
          </Button>
        </Modal.Footer>
      </Modal>
    </div>
  );
}

export default Agents;