import React, { useState, useEffect } from 'react';
import { Card, Row, Col, Alert, Button, Form, Tabs, Tab, Badge, ListGroup } from 'react-bootstrap';
import { chatAPI, wasmModulesAPI, jobsAPI } from '../services/api';

function Dashboard() {
  const [models, setModels] = useState([]);
  const [wasmModules, setWasmModules] = useState([]);
  const [selectedAgent, setSelectedAgent] = useState('');
  const [selectedWasmModule, setSelectedWasmModule] = useState('');
  const [message, setMessage] = useState('');
  const [wasmInput, setWasmInput] = useState('');
  const [executionHistory, setExecutionHistory] = useState([]);
  const [loading, setLoading] = useState(false);
  const [wasmLoading, setWasmLoading] = useState(false);
  const [error, setError] = useState('');
  const [activeTab, setActiveTab] = useState('agents');

  useEffect(() => {
    loadModels();
    loadWasmModules();
  }, []);

  const loadModels = async () => {
    try {
      const response = await chatAPI.models();
      setModels(response.data.data || []);
    } catch (err) {
      setError('Failed to load models');
    }
  };

  const loadWasmModules = async () => {
    try {
      const response = await wasmModulesAPI.list();
      setWasmModules(response.data.data || []);
    } catch (err) {
      setError('Failed to load WASM modules');
    }
  };

  const handleExecuteAgent = async (e) => {
    e.preventDefault();
    if (!selectedAgent || !message.trim()) return;

    setLoading(true);
    setError('');

    // Add pending execution to history
    const pendingExecution = {
      id: Date.now().toString(),
      agent: selectedAgent,
      input: message,
      output: null,
      timestamp: new Date(),
      type: 'agent',
      status: 'executing',
    };
    setExecutionHistory(prev => [pendingExecution, ...prev]);

    try {
      // Call chat completions API with the selected model
      const response = await chatAPI.complete({
        model: selectedAgent,
        messages: [{ role: 'user', content: message }],
        stream: false,
      });

      // Update execution in history
      const completedExecution = {
        ...pendingExecution,
        output: response.data.choices?.[0]?.message?.content || 'No response',
        status: 'completed',
      };
      setExecutionHistory(prev => prev.map(exec =>
        exec.id === pendingExecution.id ? completedExecution : exec
      ));

      // Reset form
      setSelectedAgent('');
      setMessage('');
    } catch (err) {
      const errorMessage = err.response?.data?.error || err.message || 'Unknown error occurred';
      setError(`Failed to execute agent: ${errorMessage}`);

      // Update execution in history with error
      const failedExecution = {
        ...pendingExecution,
        output: errorMessage,
        status: 'failed',
      };
      setExecutionHistory(prev => prev.map(exec =>
        exec.id === pendingExecution.id ? failedExecution : exec
      ));
    } finally {
      setLoading(false);
    }
  };

  const handleExecuteWasmModule = async (e) => {
    e.preventDefault();
    if (!selectedWasmModule || !wasmInput.trim()) return;

    setWasmLoading(true);
    setError('');

    // Find the selected WASM module
    const wasmModule = wasmModules.find(m => m.id === selectedWasmModule);
    if (!wasmModule) {
      setError('Selected WASM module not found');
      setWasmLoading(false);
      return;
    }

    // Add pending execution to history
    const pendingExecution = {
      id: Date.now().toString(),
      wasm_module: selectedWasmModule,
      input: wasmInput,
      output: null,
      timestamp: new Date(),
      type: 'wasm',
      status: 'executing',
    };
    setExecutionHistory(prev => [pendingExecution, ...prev]);

    try {
      // Create a job for the WASM module execution
      const jobResponse = await jobsAPI.create({
        workflow_id: selectedWasmModule,
        input_data: { input: wasmInput },
      });

      const jobId = jobResponse.data?.data?.id || jobResponse.data?.id;

      // Update execution in history with job ID
      const queuedExecution = {
        ...pendingExecution,
        jobId: jobId,
        status: 'queued',
      };
      setExecutionHistory(prev => prev.map(exec =>
        exec.id === pendingExecution.id ? queuedExecution : exec
      ));

      // Start monitoring the job
      monitorJob(jobId, pendingExecution.id);

      // Reset form
      setSelectedWasmModule('');
      setWasmInput('');
    } catch (err) {
      const errorMessage = err.response?.data?.error || err.message || 'Unknown error occurred';
      setError(`Failed to execute WASM module: ${errorMessage}`);

      // Update execution in history with error
      const failedExecution = {
        ...pendingExecution,
        output: errorMessage,
        status: 'failed',
      };
      setExecutionHistory(prev => prev.map(exec =>
        exec.id === pendingExecution.id ? failedExecution : exec
      ));
    } finally {
      setWasmLoading(false);
    }
  };

  // Monitor job status and update execution history
  const monitorJob = async (jobId, executionId) => {
    const checkJobStatus = async () => {
      try {
        const jobResponse = await jobsAPI.get(jobId);
        const job = jobResponse.data?.data || jobResponse.data;

        // Update execution history with job status
        setExecutionHistory(prev => prev.map(execution => {
          if (execution.id === executionId) {
            return {
              ...execution,
              status: job.status.toLowerCase(),
              output: job.output || execution.output,
            };
          }
          return execution;
        }));

        // Continue monitoring if job is not completed
        if (job.status.toLowerCase() !== 'completed' && job.status.toLowerCase() !== 'failed') {
          setTimeout(checkJobStatus, 2000); // Check every 2 seconds
        }
      } catch (err) {
        console.error('Failed to monitor job:', err);
      }
    };

    // Start monitoring
    setTimeout(checkJobStatus, 2000);
  };

  const getStatusVariant = (status) => {
    switch (status) {
      case 'queued':
        return 'warning';
      case 'running':
        return 'info';
      case 'executing':
        return 'primary';
      case 'completed':
        return 'success';
      case 'failed':
        return 'danger';
      default:
        return 'secondary';
    }
  };

  const getModelType = (identifier) => {
    if (!identifier) return 'Unknown';
    if (identifier.startsWith('agent/')) return 'Agent';
    if (identifier.startsWith('workflow/')) return 'Workflow';
    if (identifier.includes('agent')) return 'Agent';
    if (identifier.includes('wasm')) return 'WASM Module';
    return 'Model';
  };

  return (
    <div>
      <h1>Dashboard</h1>
      <p>Test your AI agents, workflows, and WASM modules</p>

      {error && <Alert variant="danger">{error}</Alert>}

      <Tabs
        activeKey={activeTab}
        onSelect={(k) => setActiveTab(k)}
        className="mb-4"
      >
        <Tab eventKey="agents" title="Agents & Workflows">
          <Row>
            <Col md={6}>
              <Card className="mb-4">
                <Card.Header>
                  <Card.Title>Execute Agent / Workflow</Card.Title>
                </Card.Header>
                <Card.Body>
                  <Form onSubmit={handleExecuteAgent}>
                    <Form.Group className="mb-3">
                      <Form.Label>Agent / Workflow</Form.Label>
                      <Form.Select
                        value={selectedAgent}
                        onChange={(e) => setSelectedAgent(e.target.value)}
                        required
                      >
                        <option value="">Select an agent or workflow...</option>
                        {models.map((model) => (
                          <option key={model.id} value={model.id}>
                            {model.id}
                          </option>
                        ))}
                      </Form.Select>
                    </Form.Group>

                    <Form.Group className="mb-3">
                      <Form.Label>Input</Form.Label>
                      <Form.Control
                        as="textarea"
                        rows={4}
                        value={message}
                        onChange={(e) => setMessage(e.target.value)}
                        placeholder="Enter your input or prompt..."
                        required
                      />
                    </Form.Group>

                    <Button
                      type="submit"
                      variant="primary"
                      disabled={loading || !selectedAgent || !message.trim()}
                      className="w-100"
                    >
                      {loading ? (
                        <>
                          <span className="spinner-border spinner-border-sm me-2" role="status" aria-hidden="true"></span>
                          Executing...
                        </>
                      ) : 'Execute'}
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
                <Card.Body style={{ height: '500px', overflowY: 'auto' }}>
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
                                  {getModelType(execution.model || execution.agent || execution.wasm_module)}
                                </Badge>
                                <small className="text-muted">
                                  {execution.timestamp.toLocaleTimeString()}
                                </small>
                              </div>
                              <div className="mb-1">
                                <strong>{execution.type === 'model' ? 'Model' : execution.type === 'agent' ? 'Agent' : 'WASM Module'}:</strong> {execution.model || execution.agent || execution.wasm_module}
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
                                  <Form.Control
                                    as="textarea"
                                    rows={3}
                                    readOnly
                                    value={typeof execution.output === 'object'
                                      ? JSON.stringify(execution.output, null, 2)
                                      : execution.output}
                                    className="mt-1 small"
                                    style={{ fontFamily: 'monospace', fontSize: '12px' }}
                                  />
                                </div>
                              )}
                            </div>
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
        <Tab eventKey="wasm" title="WASM Modules">
          <Row>
            <Col md={6}>
              <Card className="mb-4">
                <Card.Header>
                  <Card.Title>Execute WASM Module</Card.Title>
                </Card.Header>
                <Card.Body>
                  <Form onSubmit={handleExecuteWasmModule}>
                    <Form.Group className="mb-3">
                      <Form.Label>WASM Module</Form.Label>
                      <Form.Select
                        value={selectedWasmModule}
                        onChange={(e) => setSelectedWasmModule(e.target.value)}
                        required
                      >
                        <option value="">Select a WASM module...</option>
                        {wasmModules.map((module) => (
                          <option key={module.id} value={module.id}>
                            {module.name}
                          </option>
                        ))}
                      </Form.Select>
                    </Form.Group>

                    <Form.Group className="mb-3">
                      <Form.Label>Input</Form.Label>
                      <Form.Control
                        as="textarea"
                        rows={4}
                        value={wasmInput}
                        onChange={(e) => setWasmInput(e.target.value)}
                        placeholder="Enter your input or prompt..."
                        required
                      />
                    </Form.Group>

                    <Button
                      type="submit"
                      variant="primary"
                      disabled={wasmLoading || !selectedWasmModule || !wasmInput.trim()}
                      className="w-100"
                    >
                      {wasmLoading ? (
                        <>
                          <span className="spinner-border spinner-border-sm me-2" role="status" aria-hidden="true"></span>
                          Executing...
                        </>
                      ) : 'Execute'}
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
                <Card.Body style={{ height: '500px', overflowY: 'auto' }}>
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
                                  {getModelType(execution.model || execution.agent || execution.wasm_module)}
                                </Badge>
                                <small className="text-muted">
                                  {execution.timestamp.toLocaleTimeString()}
                                </small>
                              </div>
                              <div className="mb-1">
                                <strong>{execution.type === 'model' ? 'Model' : execution.type === 'agent' ? 'Agent' : 'WASM Module'}:</strong> {execution.model || execution.agent || execution.wasm_module}
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
                                  <Form.Control
                                    as="textarea"
                                    rows={3}
                                    readOnly
                                    value={typeof execution.output === 'object'
                                      ? JSON.stringify(execution.output, null, 2)
                                      : execution.output}
                                    className="mt-1 small"
                                    style={{ fontFamily: 'monospace', fontSize: '12px' }}
                                  />
                                </div>
                              )}
                            </div>
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
    </div>
  );
}

export default Dashboard;