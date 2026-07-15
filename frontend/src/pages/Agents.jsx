import React, { useState, useEffect } from 'react';
import { Card, Row, Col, Button, Form, Modal, ListGroup } from 'react-bootstrap';
import { agentsAPI, providersAPI, skillsAPI } from '../services/api';
import FilterableDropdown from '../components/FilterableDropdown';

function Agents() {
  const [agents, setAgents] = useState([]);
  const [providers, setProviders] = useState([]);
  const [models, setModels] = useState([]);
  const [skills, setSkills] = useState([]);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showEditModal, setShowEditModal] = useState(false);
  const [showSkillsModal, setShowSkillsModal] = useState(false);
  const [selectedAgent, setSelectedAgent] = useState(null);
  const [selectedAgentSkills, setSelectedAgentSkills] = useState([]);
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
    loadSkills();
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

  const loadSkills = async () => {
    try {
      const response = await skillsAPI.list();
      setSkills(response.data || []);
    } catch (error) {
      console.error('Failed to load skills:', error);
    }
  };

  const loadAgentSkills = async (agentId) => {
    try {
      const response = await agentsAPI.getSkills(agentId);
      setSelectedAgentSkills(response.data || []);
    } catch (error) {
      console.error('Failed to load agent skills:', error);
      setSelectedAgentSkills([]);
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
      console.log('Creating agent with data:', newAgent);
      const createData = {
        name: newAgent.name,
        description: newAgent.description,
        provider_id: newAgent.provider_id,
        model_id: newAgent.model_id,
        system_prompt: newAgent.system_prompt,
      };
      await agentsAPI.create(createData);
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
      console.error('Error response:', error.response?.data);
      alert('Failed to create agent: ' + (error.response?.data?.error || error.message));
    } finally {
      setLoading(false);
    }
  };

  const handleUpdateAgent = async () => {
    if (!selectedAgent?.id) {
      alert('Error: No agent selected');
      return;
    }
    setLoading(true);
    try {
      console.log('Updating agent with data:', selectedAgent);
      const updateData = {
        name: selectedAgent.name,
        description: selectedAgent.description,
        provider_id: selectedAgent.provider_id,
        model_id: selectedAgent.model_id,
        system_prompt: selectedAgent.system_prompt,
      };
      await agentsAPI.update(selectedAgent.id, updateData);
      setShowEditModal(false);
      setSelectedAgent(null);
      loadAgents();
    } catch (error) {
      console.error('Failed to update agent:', error);
      console.error('Error response:', error.response?.data);
      alert('Failed to update agent: ' + (error.response?.data?.error || error.message));
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

  const openSkillsModal = async (agent) => {
    setSelectedAgent(agent);
    await loadAgentSkills(agent.id);
    setShowSkillsModal(true);
  };

  const handleAssignSkill = async (skillId) => {
    if (!selectedAgent) return;

    setLoading(true);
    try {
      await agentsAPI.assignSkill(selectedAgent.id, skillId);
      await loadAgentSkills(selectedAgent.id);
    } catch (error) {
      console.error('Failed to assign skill:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleRemoveSkill = async (skillId) => {
    if (!selectedAgent) return;

    setLoading(true);
    try {
      await agentsAPI.removeSkill(selectedAgent.id, skillId);
      await loadAgentSkills(selectedAgent.id);
    } catch (error) {
      console.error('Failed to remove skill:', error);
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
                    onClick={() => openSkillsModal(agent)}
                  >
                    Skills
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

      {/* Skills Management Modal */}
      <Modal show={showSkillsModal} onHide={() => setShowSkillsModal(false)} size="lg">
        <Modal.Header closeButton>
          <Modal.Title>Manage Skills for {selectedAgent?.name}</Modal.Title>
        </Modal.Header>
        <Modal.Body>
          {selectedAgent && (
            <>
              <h6>Assigned Skills</h6>
              {selectedAgentSkills.length > 0 ? (
                <ListGroup className="mb-4">
                  {selectedAgentSkills.map((skill) => (
                    <ListGroup.Item key={skill.id} className="d-flex justify-content-between align-items-center">
                      <div>
                        <strong>{skill.name}</strong>
                        <div className="small text-muted">{skill.description}</div>
                        <div className="small text-muted">
                          Path: {skill.path}
                        </div>
                      </div>
                      <Button
                        variant="outline-danger"
                        size="sm"
                        onClick={() => handleRemoveSkill(skill.id)}
                        disabled={loading}
                      >
                        Remove
                      </Button>
                    </ListGroup.Item>
                  ))}
                </ListGroup>
              ) : (
                <p className="text-muted mb-4">No skills assigned to this agent.</p>
              )}

              <h6>Available Skills</h6>
              <ListGroup>
                {skills
                  .filter((skill) => !selectedAgentSkills.find((s) => s.id === skill.id))
                  .map((skill) => (
                    <ListGroup.Item key={skill.id} className="d-flex justify-content-between align-items-center">
                      <div>
                        <strong>{skill.name}</strong>
                        <div className="small text-muted">{skill.description}</div>
                        <div className="small text-muted">
                          Path: {skill.path}
                        </div>
                      </div>
                      <Button
                        variant="outline-success"
                        size="sm"
                        onClick={() => handleAssignSkill(skill.id)}
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
          <Button variant="secondary" onClick={() => setShowSkillsModal(false)}>
            Close
          </Button>
        </Modal.Footer>
      </Modal>
    </div>
  );
}

export default Agents;