import React, { useState, useEffect } from 'react';
import { Card, Row, Col, Button, Form, Modal, ListGroup } from 'react-bootstrap';
import { agentsAPI, providersAPI, toolsAPI } from '../services/api';
import FilterableDropdown from '../components/FilterableDropdown';

function Agents() {
  const [agents, setAgents] = useState([]);
  const [providers, setProviders] = useState([]);
  const [models, setModels] = useState([]);
  const [tools, setTools] = useState([]);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showEditModal, setShowEditModal] = useState(false);
  const [showToolsModal, setShowToolsModal] = useState(false);
  const [selectedAgent, setSelectedAgent] = useState(null);
  const [selectedAgentTools, setSelectedAgentTools] = useState([]);
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
    loadTools();
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

  const loadTools = async () => {
    try {
      const response = await toolsAPI.list();
      setTools(response.data || []);
    } catch (error) {
      console.error('Failed to load tools:', error);
    }
  };

  const loadAgentTools = async (agentId) => {
    try {
      const response = await agentsAPI.getTools(agentId);
      setSelectedAgentTools(response.data || []);
    } catch (error) {
      console.error('Failed to load agent tools:', error);
      setSelectedAgentTools([]);
    }
  };

  const loadModels = async (providerId) => {
    if (!providerId) {
      setModels([]);
      return;
    }

    try {
      const response = await providersAPI.getModels(providerId);
      console.log('Provider models API raw response:', response);

      // The API returns { data: [...models] }
      // Axios wraps this, so response.data is { data: [...models] }
      const modelsData = response.data?.data || [];

      console.log('Extracted models array:', modelsData);
      console.log('Is array?', Array.isArray(modelsData));

      // Ensure we always set an array
      setModels(Array.isArray(modelsData) ? modelsData : []);
    } catch (error) {
      console.error('Failed to load models from provider:', error);
      setModels([]);
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
    // Load models for the agent's provider
    if (agent.provider_id) {
      loadModels(agent.provider_id);
    }
  };

  const openToolsModal = async (agent) => {
    setSelectedAgent(agent);
    await loadAgentTools(agent.id);
    setShowToolsModal(true);
  };

  const handleAssignTool = async (toolId) => {
    if (!selectedAgent) return;

    setLoading(true);
    try {
      await agentsAPI.assignTool(selectedAgent.id, toolId);
      await loadAgentTools(selectedAgent.id);
    } catch (error) {
      console.error('Failed to assign tool:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleRemoveTool = async (toolId) => {
    if (!selectedAgent) return;

    setLoading(true);
    try {
      await agentsAPI.removeTool(selectedAgent.id, toolId);
      await loadAgentTools(selectedAgent.id);
    } catch (error) {
      console.error('Failed to remove tool:', error);
    } finally {
      setLoading(false);
    }
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
                    variant="outline-info"
                    size="sm"
                    onClick={() => openToolsModal(agent)}
                  >
                    Tools
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
                onChange={(e) => {
                  const providerId = e.target.value;
                  setNewAgent({ ...newAgent, provider_id: providerId, model_id: '' });
                  loadModels(providerId);
                }}
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
              <FilterableDropdown
                options={(Array.isArray(models) ? models : []).map((model) => ({
                  value: model.id,
                  label: `${model.id} ${model.owned_by ? `(${model.owned_by})` : ''}`
                }))}
                value={newAgent.model_id}
                onChange={(value) => setNewAgent({ ...newAgent, model_id: value })}
                placeholder="Type to search models..."
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
      <Modal show={showEditModal} onHide={() => setShowEditModal(false)} size="lg" dialogClassName="modal-80w">
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
                  onChange={(e) => {
                    const providerId = e.target.value;
                    setSelectedAgent({ ...selectedAgent, provider_id: providerId, model_id: '' });
                    loadModels(providerId);
                  }}
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
                <FilterableDropdown
                  options={(Array.isArray(models) ? models : []).map((model) => ({
                    value: model.id,
                    label: `${model.id} ${model.owned_by ? `(${model.owned_by})` : ''}`
                  }))}
                  value={selectedAgent.model_id}
                  onChange={(value) => setSelectedAgent({ ...selectedAgent, model_id: value })}
                  placeholder="Type to search models..."
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

      {/* Tools Management Modal */}
      <Modal show={showToolsModal} onHide={() => setShowToolsModal(false)} size="lg">
        <Modal.Header closeButton>
          <Modal.Title>Manage Tools for {selectedAgent?.name}</Modal.Title>
        </Modal.Header>
        <Modal.Body>
          {selectedAgent && (
            <>
              <h6>Assigned Tools</h6>
              {selectedAgentTools.length > 0 ? (
                <ListGroup className="mb-4">
                  {selectedAgentTools.map((tool) => (
                    <ListGroup.Item key={tool.id} className="d-flex justify-content-between align-items-center">
                      <div>
                        <strong>{tool.name}</strong>
                        <div className="small text-muted">{tool.description}</div>
                        <div className="small text-muted">
                          Type: {tool.metadata?.tool_type || 'N/A'}
                        </div>
                      </div>
                      <Button
                        variant="outline-danger"
                        size="sm"
                        onClick={() => handleRemoveTool(tool.id)}
                        disabled={loading}
                      >
                        Remove
                      </Button>
                    </ListGroup.Item>
                  ))}
                </ListGroup>
              ) : (
                <p className="text-muted mb-4">No tools assigned to this agent.</p>
              )}

              <h6>Available Tools</h6>
              <ListGroup>
                {tools
                  .filter((tool) => !selectedAgentTools.find((t) => t.id === tool.id))
                  .map((tool) => (
                    <ListGroup.Item key={tool.id} className="d-flex justify-content-between align-items-center">
                      <div>
                        <strong>{tool.name}</strong>
                        <div className="small text-muted">{tool.description}</div>
                        <div className="small text-muted">
                          Type: {tool.metadata?.tool_type || 'N/A'}
                        </div>
                      </div>
                      <Button
                        variant="outline-success"
                        size="sm"
                        onClick={() => handleAssignTool(tool.id)}
                        disabled={loading}
                      >
                        Assign
                      </Button>
                    </ListGroup.Item>
                  ))}
              </ListGroup>
            </>
          )}
        </Modal.Body>
        <Modal.Footer>
          <Button variant="secondary" onClick={() => setShowToolsModal(false)}>
            Close
          </Button>
        </Modal.Footer>
      </Modal>
    </div>
  );
}

export default Agents;