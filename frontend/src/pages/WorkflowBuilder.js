import React, { useState, useEffect } from 'react';
import { Card, Row, Col, Button, Form, Modal, ListGroup } from 'react-bootstrap';
import { workflowsAPI, agentsAPI } from '../services/api';

function WorkflowBuilder() {
  const [workflows, setWorkflows] = useState([]);
  const [agents, setAgents] = useState([]);
  const [selectedWorkflow, setSelectedWorkflow] = useState(null);
  const [workflowSteps, setWorkflowSteps] = useState([]);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showStepModal, setShowStepModal] = useState(false);
  const [newWorkflow, setNewWorkflow] = useState({ name: '', description: '' });
  const [newStep, setNewStep] = useState({ type: 'AGENT', agent_id: '', config: {} });
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    loadWorkflows();
    loadAgents();
  }, []);

  const loadWorkflows = async () => {
    try {
      const response = await workflowsAPI.list();
      setWorkflows(response.data || []);
    } catch (error) {
      console.error('Failed to load workflows:', error);
    }
  };

  const loadAgents = async () => {
    try {
      const response = await agentsAPI.list();
      setAgents(response.data || []);
    } catch (error) {
      console.error('Failed to load agents:', error);
    }
  };

  const loadWorkflowSteps = async (workflowId) => {
    try {
      const response = await workflowsAPI.getSteps(workflowId);
      setWorkflowSteps(response.data || []);
    } catch (error) {
      console.error('Failed to load workflow steps:', error);
    }
  };

  const handleCreateWorkflow = async () => {
    setLoading(true);
    try {
      await workflowsAPI.create(newWorkflow);
      setShowCreateModal(false);
      setNewWorkflow({ name: '', description: '' });
      loadWorkflows();
    } catch (error) {
      console.error('Failed to create workflow:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleCreateStep = async () => {
    setLoading(true);
    try {
      await workflowsAPI.createStep(selectedWorkflow.id, newStep);
      setShowStepModal(false);
      setNewStep({ type: 'AGENT', agent_id: '', config: {} });
      loadWorkflowSteps(selectedWorkflow.id);
    } catch (error) {
      console.error('Failed to create step:', error);
    } finally {
      setLoading(false);
    }
  };

  const selectWorkflow = (workflow) => {
    setSelectedWorkflow(workflow);
    loadWorkflowSteps(workflow.id);
  };

  return (
    <div>
      <div className="d-flex justify-content-between align-items-center mb-4">
        <h1>Workflow Builder</h1>
        <Button
          variant="primary"
          onClick={() => setShowCreateModal(true)}
        >
          Create Workflow
        </Button>
      </div>

      <Row>
        <Col md={4}>
          <Card>
            <Card.Header>
              <Card.Title>Workflows</Card.Title>
            </Card.Header>
            <Card.Body>
              <ListGroup>
                {workflows.map((workflow) => (
                  <ListGroup.Item
                    key={workflow.id}
                    action
                    active={selectedWorkflow?.id === workflow.id}
                    onClick={() => selectWorkflow(workflow)}
                  >
                    <h6>{workflow.name}</h6>
                    <small className="text-muted">{workflow.description}</small>
                  </ListGroup.Item>
                ))}
              </ListGroup>
            </Card.Body>
          </Card>
        </Col>

        <Col md={8}>
          {selectedWorkflow ? (
            <Card>
              <Card.Header className="d-flex justify-content-between align-items-center">
                <Card.Title>{selectedWorkflow.name}</Card.Title>
                <Button
                  variant="outline-primary"
                  size="sm"
                  onClick={() => setShowStepModal(true)}
                >
                  Add Step
                </Button>
              </Card.Header>
              <Card.Body>
                <h5>Steps</h5>
                {workflowSteps.length === 0 ? (
                  <p className="text-muted">No steps defined yet</p>
                ) : (
                  <ListGroup>
                    {workflowSteps.map((step, index) => (
                      <ListGroup.Item key={step.id}>
                        <div className="d-flex justify-content-between align-items-center">
                          <div>
                            <strong>Step {index + 1}:</strong> {step.type}
                            {step.agent_id && (
                              <span className="ms-2 badge bg-primary">
                                {agents.find(a => a.id === step.agent_id)?.name || 'Unknown Agent'}
                              </span>
                            )}
                          </div>
                          <small className="text-muted">Order: {step.step_order}</small>
                        </div>
                      </ListGroup.Item>
                    ))}
                  </ListGroup>
                )}
              </Card.Body>
            </Card>
          ) : (
            <Card>
              <Card.Body className="text-center text-muted">
                <h4>Select a workflow to view details</h4>
                <p>Choose a workflow from the list to see and edit its steps</p>
              </Card.Body>
            </Card>
          )}
        </Col>
      </Row>

      {/* Create Workflow Modal */}
      <Modal show={showCreateModal} onHide={() => setShowCreateModal(false)}>
        <Modal.Header closeButton>
          <Modal.Title>Create New Workflow</Modal.Title>
        </Modal.Header>
        <Modal.Body>
          <Form>
            <Form.Group className="mb-3">
              <Form.Label>Name</Form.Label>
              <Form.Control
                type="text"
                value={newWorkflow.name}
                onChange={(e) =>
                  setNewWorkflow({ ...newWorkflow, name: e.target.value })
                }
                required
              />
            </Form.Group>
            <Form.Group className="mb-3">
              <Form.Label>Description</Form.Label>
              <Form.Control
                as="textarea"
                rows={3}
                value={newWorkflow.description}
                onChange={(e) =>
                  setNewWorkflow({ ...newWorkflow, description: e.target.value })
                }
              />
            </Form.Group>
          </Form>
        </Modal.Body>
        <Modal.Footer>
          <Button variant="secondary" onClick={() => setShowCreateModal(false)}>
            Cancel
          </Button>
          <Button variant="primary" onClick={handleCreateWorkflow} disabled={loading}>
            {loading ? 'Creating...' : 'Create'}
          </Button>
        </Modal.Footer>
      </Modal>

      {/* Add Step Modal */}
      <Modal show={showStepModal} onHide={() => setShowStepModal(false)}>
        <Modal.Header closeButton>
          <Modal.Title>Add Workflow Step</Modal.Title>
        </Modal.Header>
        <Modal.Body>
          <Form>
            <Form.Group className="mb-3">
              <Form.Label>Step Type</Form.Label>
              <Form.Select
                value={newStep.type}
                onChange={(e) => setNewStep({ ...newStep, type: e.target.value })}
              >
                <option value="AGENT">Agent</option>
                <option value="WASM">WASM Module</option>
              </Form.Select>
            </Form.Group>

            {newStep.type === 'AGENT' && (
              <Form.Group className="mb-3">
                <Form.Label>Agent</Form.Label>
                <Form.Select
                  value={newStep.agent_id}
                  onChange={(e) =>
                    setNewStep({ ...newStep, agent_id: e.target.value })
                  }
                >
                  <option value="">Select an agent...</option>
                  {agents.map((agent) => (
                    <option key={agent.id} value={agent.id}>
                      {agent.name}
                    </option>
                  ))}
                </Form.Select>
              </Form.Group>
            )}
          </Form>
        </Modal.Body>
        <Modal.Footer>
          <Button variant="secondary" onClick={() => setShowStepModal(false)}>
            Cancel
          </Button>
          <Button variant="primary" onClick={handleCreateStep} disabled={loading}>
            {loading ? 'Adding...' : 'Add Step'}
          </Button>
        </Modal.Footer>
      </Modal>
    </div>
  );
}

export default WorkflowBuilder;