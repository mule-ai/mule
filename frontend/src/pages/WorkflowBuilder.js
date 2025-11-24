import React, { useState, useEffect } from 'react';
import { Card, Row, Col, Button, Form, Modal, ListGroup } from 'react-bootstrap';
import { workflowsAPI, agentsAPI, wasmModulesAPI } from '../services/api';

function WorkflowBuilder() {
  const [workflows, setWorkflows] = useState([]);
  const [agents, setAgents] = useState([]);
  const [wasmModules, setWasmModules] = useState([]);
  const [selectedWorkflow, setSelectedWorkflow] = useState(null);
  const [workflowSteps, setWorkflowSteps] = useState([]);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showStepModal, setShowStepModal] = useState(false);
  const [showEditStepModal, setShowEditStepModal] = useState(false);
  const [editingStep, setEditingStep] = useState(null);
  const [newWorkflow, setNewWorkflow] = useState({ name: '', description: '' });
  const [newStep, setNewStep] = useState({ type: 'agent', agent_id: '', wasm_module_id: '', config: {} });
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    loadWorkflows();
    loadAgents();
    loadWasmModules();
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

  const loadWasmModules = async () => {
    try {
      const response = await wasmModulesAPI.list();
      console.log('WASM modules API response:', response);

      // Handle different response structures
      let modules = [];
      if (Array.isArray(response.data)) {
        modules = response.data;
      } else if (response.data && Array.isArray(response.data.data)) {
        // Some APIs wrap data in {data: [...]}
        modules = response.data.data;
      } else if (response.data && typeof response.data === 'object') {
        // If it's a single object, wrap in array
        modules = [response.data];
      }

      setWasmModules(modules);
      console.log('Loaded WASM modules:', modules);
    } catch (error) {
      console.error('Failed to load WASM modules:', error);
      setWasmModules([]); // Set empty array on error
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
      // Clean up step data based on type to avoid foreign key constraint errors
      const stepData = { ...newStep };

      if (stepData.type === 'wasm_module') {
        // For WASM module steps, remove agent_id to avoid FK constraint
        delete stepData.agent_id;
      } else if (stepData.type === 'agent') {
        // For agent steps, remove wasm_module_id
        delete stepData.wasm_module_id;
      }

      await workflowsAPI.createStep(selectedWorkflow.id, stepData);
      setShowStepModal(false);
      setNewStep({ type: 'agent', agent_id: '', wasm_module_id: '', config: {} });
      loadWorkflowSteps(selectedWorkflow.id);
    } catch (error) {
      console.error('Failed to create step:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleEditStep = (step) => {
    setEditingStep(step);
    setShowEditStepModal(true);
  };

  const handleUpdateStep = async () => {
    if (!editingStep || !selectedWorkflow) return;

    setLoading(true);
    try {
      // Transform step_type to type for backend compatibility
      const stepData = {
        ...editingStep,
        type: editingStep.step_type
      };
      delete stepData.step_type;

      // Clean up the data based on step type
      if (stepData.type === 'wasm_module') {
        delete stepData.agent_id;
      } else if (stepData.type === 'agent') {
        delete stepData.wasm_module_id;
      }

      await workflowsAPI.updateStep(selectedWorkflow.id, editingStep.id, stepData);
      setShowEditStepModal(false);
      setEditingStep(null);
      loadWorkflowSteps(selectedWorkflow.id);
    } catch (error) {
      console.error('Failed to update step:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleDeleteStep = async (stepId) => {
    if (!selectedWorkflow) return;

    if (!window.confirm('Are you sure you want to delete this step?')) {
      return;
    }

    setLoading(true);
    try {
      await workflowsAPI.deleteStep(selectedWorkflow.id, stepId);
      loadWorkflowSteps(selectedWorkflow.id);
    } catch (error) {
      console.error('Failed to delete step:', error);
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
                            {step.wasm_module_id && (
                              <span className="ms-2 badge bg-success">
                                {wasmModules.find(m => m.id === step.wasm_module_id)?.name || 'Unknown WASM Module'}
                              </span>
                            )}
                          </div>
                          <div>
                            <small className="text-muted me-2">Order: {step.step_order}</small>
                            <Button
                              variant="outline-primary"
                              size="sm"
                              className="me-1"
                              onClick={() => handleEditStep(step)}
                            >
                              Edit
                            </Button>
                            <Button
                              variant="outline-danger"
                              size="sm"
                              onClick={() => handleDeleteStep(step.id)}
                            >
                              Delete
                            </Button>
                          </div>
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
                <option value="agent">Agent</option>
                <option value="wasm_module">WASM Module</option>
              </Form.Select>
            </Form.Group>

            {newStep.type === 'agent' && (
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

            {newStep.type === 'wasm_module' && (
              <Form.Group className="mb-3">
                <Form.Label>WASM Module</Form.Label>
                <Form.Select
                  value={newStep.wasm_module_id}
                  onChange={(e) =>
                    setNewStep({ ...newStep, wasm_module_id: e.target.value })
                  }
                >
                  <option value="">Select a WASM module...</option>
                  {Array.isArray(wasmModules) && wasmModules.map((module) => (
                    <option key={module.id} value={module.id}>
                      {module.name}
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

      {/* Edit Step Modal */}
      <Modal show={showEditStepModal} onHide={() => setShowEditStepModal(false)}>
        <Modal.Header closeButton>
          <Modal.Title>Edit Workflow Step</Modal.Title>
        </Modal.Header>
        <Modal.Body>
          {editingStep && (
            <Form>
              <Form.Group className="mb-3">
                <Form.Label>Step Order</Form.Label>
                <Form.Control
                  type="number"
                  value={editingStep.step_order}
                  onChange={(e) =>
                    setEditingStep({ ...editingStep, step_order: parseInt(e.target.value) || 0 })
                  }
                  required
                />
              </Form.Group>
              <Form.Group className="mb-3">
                <Form.Label>Step Type</Form.Label>
                <Form.Select
                  value={editingStep.step_type}
                  onChange={(e) => setEditingStep({ ...editingStep, step_type: e.target.value })}
                >
                  <option value="agent">Agent</option>
                  <option value="wasm_module">WASM Module</option>
                </Form.Select>
              </Form.Group>

              {editingStep.step_type === 'agent' && (
                <Form.Group className="mb-3">
                  <Form.Label>Agent</Form.Label>
                  <Form.Select
                    value={editingStep.agent_id || ''}
                    onChange={(e) =>
                      setEditingStep({ ...editingStep, agent_id: e.target.value })
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

              {editingStep.step_type === 'wasm_module' && (
                <Form.Group className="mb-3">
                  <Form.Label>WASM Module</Form.Label>
                  <Form.Select
                    value={editingStep.wasm_module_id || ''}
                    onChange={(e) =>
                      setEditingStep({ ...editingStep, wasm_module_id: e.target.value })
                    }
                  >
                    <option value="">Select a WASM module...</option>
                    {Array.isArray(wasmModules) && wasmModules.map((module) => (
                      <option key={module.id} value={module.id}>
                        {module.name}
                      </option>
                    ))}
                  </Form.Select>
                </Form.Group>
              )}
            </Form>
          )}
        </Modal.Body>
        <Modal.Footer>
          <Button variant="secondary" onClick={() => setShowEditStepModal(false)}>
            Cancel
          </Button>
          <Button variant="primary" onClick={handleUpdateStep} disabled={loading}>
            {loading ? 'Updating...' : 'Update Step'}
          </Button>
        </Modal.Footer>
      </Modal>
    </div>
  );
}

export default WorkflowBuilder;