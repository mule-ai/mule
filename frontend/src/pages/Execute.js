import React, { useState, useEffect } from 'react';
import { Card, Row, Col, Alert, Button, Form, Modal, ListGroup, Badge, Tabs, Tab } from 'react-bootstrap';
import { chatAPI, jobsAPI } from '../services/api';

function Execute() {
  const [models, setModels] = useState([]);
  const [selectedModel, setSelectedModel] = useState('');
  const [input, setInput] = useState('');
  const [executionHistory, setExecutionHistory] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [showJobModal, setShowJobModal] = useState(false);
  const [currentJob, setCurrentJob] = useState(null);
  const [jobSteps, setJobSteps] = useState([]);
  const [activeTab, setActiveTab] = useState('agents');

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

  const handleExecute = async (e) => {
    e.preventDefault();
    if (!selectedModel || !input.trim()) return;

    setLoading(true);
    setError('');

    try {
      const response = await chatAPI.complete({
        model: selectedModel,
        messages: [{ role: 'user', content: input }],
        stream: false,
      });

      const execution = {
        id: Date.now().toString(),
        model: selectedModel,
        input: input,
        output: response.data.choices?.[0]?.message?.content || 'No response',
        timestamp: new Date(),
        type: 'direct',
        status: 'completed',
      };

      setExecutionHistory([execution, ...executionHistory]);
      setInput('');
    } catch (err) {
      // Check if this is an async job response
      if (err.response?.data?.object === 'async.job') {
        const jobData = err.response.data;
        const execution = {
          id: jobData.id,
          model: selectedModel,
          input: input,
          output: null,
          timestamp: new Date(),
          type: 'workflow',
          status: 'queued',
          jobId: jobData.id,
        };

        setExecutionHistory([execution, ...executionHistory]);
        setInput('');

        // Start polling for job status
        pollJobStatus(jobData.id, execution.id);
      } else {
        setError(err.response?.data?.error || 'Failed to execute');
      }
    } finally {
      setLoading(false);
    }
  };

  const pollJobStatus = async (jobId, executionId) => {
    const poll = async () => {
      try {
        const jobResponse = await jobsAPI.get(jobId);
        const job = jobResponse.data;

        setExecutionHistory(prev => prev.map(exec =>
          exec.id === executionId
            ? { ...exec, status: job.status.toLowerCase(), output: job.output_data }
            : exec
        ));

        if (job.status === 'COMPLETED' || job.status === 'FAILED') {
          return;
        }

        setTimeout(poll, 2000); // Poll every 2 seconds
      } catch (err) {
        console.error('Failed to poll job status:', err);
      }
    };

    poll();
  };

  const viewJobDetails = async (execution) => {
    if (!execution.jobId) return;

    try {
      const jobResponse = await jobsAPI.get(execution.jobId);
      const stepsResponse = await jobsAPI.getSteps(execution.jobId);

      setCurrentJob(jobResponse.data);
      setJobSteps(stepsResponse.data || []);
      setShowJobModal(true);
    } catch (err) {
      setError('Failed to load job details');
    }
  };

  const getStatusVariant = (status) => {
    switch (status) {
      case 'queued':
        return 'warning';
      case 'running':
        return 'info';
      case 'completed':
        return 'success';
      case 'failed':
        return 'danger';
      default:
        return 'secondary';
    }
  };

  const getModelType = (modelId) => {
    if (modelId.startsWith('agent/')) return 'Agent';
    if (modelId.startsWith('workflow/')) return 'Workflow';
    return 'Unknown';
  };

  const handleRefresh = () => {
    loadModels();
  };

  return (
    <div>
      <div className="d-flex justify-content-between align-items-center mb-4">
        <div>
          <h1>Execution Interface</h1>
          <p className="text-muted">Execute agents and workflows with real-time monitoring</p>
        </div>
        <Button variant="outline-primary" onClick={handleRefresh}>
          Refresh
        </Button>
      </div>

      {error && <Alert variant="danger" dismissible onClose={() => setError('')}>{error}</Alert>}

      <Tabs
        activeKey={activeTab}
        onSelect={(k) => setActiveTab(k)}
        className="mb-4"
      >
        <Tab eventKey="agents" title="Agents & Workflows">
          <Row>
        <Col md={6}>
          <Card>
            <Card.Header>
              <Card.Title>Execute Model</Card.Title>
            </Card.Header>
            <Card.Body>
              <Form onSubmit={handleExecute}>
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
                        {model.id} ({getModelType(model.id)})
                      </option>
                    ))}
                  </Form.Select>
                </Form.Group>

                <Form.Group className="mb-3">
                  <Form.Label>Input</Form.Label>
                  <Form.Control
                    as="textarea"
                    rows={4}
                    value={input}
                    onChange={(e) => setInput(e.target.value)}
                    placeholder="Enter your input or prompt..."
                    required
                  />
                </Form.Group>

                <Button
                  type="submit"
                  variant="primary"
                  disabled={loading || !selectedModel || !input.trim()}
                  className="w-100"
                >
                  {loading ? 'Executing...' : 'Execute'}
                </Button>
              </Form>
            </Card.Body>
          </Card>
        </Col>

        <Col md={6}>
          <Card>
            <Card.Header>
              <Card.Title>Execution History</Card.Title>
            </Card.Header>
            <Card.Body style={{ height: '500px', overflowY: 'auto' }}
>
              {executionHistory.length === 0 ? (
                <p className="text-muted">No executions yet</p>
              ) : (
                <ListGroup>
                  {executionHistory.map((execution) => (
                    <ListGroup.Item key={execution.id} className="mb-2">
                      <div className="d-flex justify-content-between align-items-start">
                        <div className="flex-grow-1">
                          <div className="d-flex align-items-center mb-1">
                            <Badge bg={getStatusVariant(execution.status)} className="me-2">
                              {execution.status}
                            </Badge>
                            <Badge bg="outline-secondary" className="me-2">
                              {getModelType(execution.model)}
                            </Badge>
                            <small className="text-muted">
                              {execution.timestamp.toLocaleTimeString()}
                            </small>
                          </div>
                          <div className="mb-1">
                            <strong>Model:</strong> {execution.model}
                          </div>
                          <div className="mb-2">
                            <strong>Input:</strong>
                            <div className="small text-muted mt-1">
                              {execution.input.length > 100
                                ? execution.input.substring(0, 100) + '...'
                                : execution.input
                              }
                            </div>
                          </div>
                          {execution.output && (
                            <div className="mb-2">
                              <strong>Output:</strong>
                              <div className="small text-muted mt-1">
                                {execution.output.length > 100
                                  ? execution.output.substring(0, 100) + '...'
                                  : execution.output
                                }
                              </div>
                            </div>
                          )}
                        </div>
                        {execution.jobId && (
                          <Button
                            variant="outline-primary"
                            size="sm"
                            onClick={() => viewJobDetails(execution)}
                          >
                            Details
                          </Button>
                        )}
                      </div>
                    </ListGroup.Item>
                  ))}
                </ListGroup>
              )}
            </Card.Body>
          </Card>
        </Col>
      </Row>
        </Tab>
      </Tabs>

      {/* Job Details Modal */}
      <Modal
        show={showJobModal}
        onHide={() => setShowJobModal(false)}
        size="lg"
      >
        <Modal.Header closeButton>
          <Modal.Title>Job Details - {currentJob?.id?.substring(0, 8)}...</Modal.Title>
        </Modal.Header>
        <Modal.Body>
          {currentJob && (
            <div>
              <Row className="mb-3">
                <Col md={6}>
                  <strong>Status:</strong>{' '}
                  <Badge bg={getStatusVariant(currentJob.status.toLowerCase())}>
                    {currentJob.status}
                  </Badge>
                </Col>
                <Col md={6}>
                  <strong>Workflow ID:</strong> {currentJob.workflow_id}
                </Col>
              </Row>

              <Row className="mb-3">
                <Col md={6}>
                  <strong>Created:</strong> {new Date(currentJob.created_at).toLocaleString()}
                </Col>
                <Col md={6}>
                  <strong>Started:</strong> {currentJob.started_at ? new Date(currentJob.started_at).toLocaleString() : 'N/A'}
                </Col>
              </Row>

              <Row className="mb-3">
                <Col md={6}>
                  <strong>Completed:</strong> {currentJob.completed_at ? new Date(currentJob.completed_at).toLocaleString() : 'N/A'}
                </Col>
              </Row>

              <h5 className="mt-4">Job Steps</h5>
              {jobSteps.length === 0 ? (
                <p className="text-muted">No steps found for this job</p>
              ) : (
                <ListGroup>
                  {jobSteps.map((step, index) => (
                    <ListGroup.Item key={step.id}>
                      <div className="d-flex justify-content-between align-items-center">
                        <div>
                          <strong>Step {index + 1}:</strong> {step.workflow_step_id}
                        </div>
                        <Badge bg={getStatusVariant(step.status.toLowerCase())}>
                          {step.status}
                        </Badge>
                      </div>

                      <div className="mt-2">
                        <small className="text-muted">
                          Started: {step.started_at ? new Date(step.started_at).toLocaleString() : 'N/A'} |
                          Completed: {step.completed_at ? new Date(step.completed_at).toLocaleString() : 'N/A'}
                        </small>
                      </div>

                      {step.input_data && (
                        <div className="mt-2">
                          <strong>Input:</strong>
                          <pre className="small text-muted mt-1">
                            {JSON.stringify(step.input_data, null, 2)}
                          </pre>
                        </div>
                      )}

                      {step.output_data && (
                        <div className="mt-2">
                          <strong>Output:</strong>
                          <pre className="small text-muted mt-1">
                            {JSON.stringify(step.output_data, null, 2)}
                          </pre>
                        </div>
                      )}
                    </ListGroup.Item>
                  ))}
                </ListGroup>
              )}
            </div>
          )}
        </Modal.Body>
        <Modal.Footer>
          <Button variant="secondary" onClick={() => setShowJobModal(false)}>
            Close
          </Button>
        </Modal.Footer>
      </Modal>
    </div>
  );
}

export default Execute;