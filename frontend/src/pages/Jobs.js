import React, { useState, useEffect } from 'react';
import { Card, Row, Col, Badge, Button, Modal, ListGroup, Form } from 'react-bootstrap';
import { jobsAPI } from '../services/api';

function Jobs() {
  const [jobs, setJobs] = useState([]);
  const [selectedJob, setSelectedJob] = useState(null);
  const [jobSteps, setJobSteps] = useState([]);
  const [showDetailsModal, setShowDetailsModal] = useState(false);

  useEffect(() => {
    loadJobs();
    // Poll for updates every 5 seconds
    const interval = setInterval(loadJobs, 5000);
    return () => clearInterval(interval);
  }, []);

  const loadJobs = async () => {
    try {
      const response = await jobsAPI.list();
      setJobs(response.data || []);
    } catch (error) {
      console.error('Failed to load jobs:', error);
    }
  };

  const loadJobSteps = async (jobId) => {
    try {
      const response = await jobsAPI.getSteps(jobId);
      setJobSteps(response.data || []);
    } catch (error) {
      console.error('Failed to load job steps:', error);
    }
  };

  const openJobDetails = async (job) => {
    setSelectedJob(job);
    await loadJobSteps(job.id);
    setShowDetailsModal(true);
  };

  const getStatusVariant = (status) => {
    switch (status.toLowerCase()) {
      case 'queued':
        return 'warning';
      case 'running':
        return 'info';
      case 'completed':
        return 'success';
      case 'failed':
        return 'danger';
      case 'cancelled':
        return 'secondary';
      default:
        return 'secondary';
    }
  };

  const formatDate = (dateString) => {
    if (!dateString) return 'N/A';
    return new Date(dateString).toLocaleString();
  };

  const cancelJob = async (jobId) => {
    if (!window.confirm('Are you sure you want to cancel this job?')) {
      return;
    }

    try {
      await jobsAPI.cancel(jobId);
      // Refresh the job list
      await loadJobs();

      // If the details modal is open for this job, close it
      if (selectedJob && selectedJob.id === jobId) {
        setShowDetailsModal(false);
        setSelectedJob(null);
      }

      // Show success message
      alert('Job cancelled successfully');
    } catch (error) {
      console.error('Failed to cancel job:', error);
      alert('Failed to cancel job: ' + (error.response?.data?.error || error.message));
    }
  };

  return (
    <div>
      <div className="d-flex justify-content-between align-items-center mb-4">
        <h1>Jobs</h1>
        <Button variant="outline-primary" onClick={loadJobs}>
          Refresh
        </Button>
      </div>

      <Row>
        {jobs.map((job) => (
          <Col md={6} lg={4} className="mb-4" key={job.id}>
            <Card>
              <Card.Header className="d-flex justify-content-between align-items-center">
                <Card.Title className="mb-0">Job {job.id.substring(0, 8)}...</Card.Title>
                <Badge bg={getStatusVariant(job.status)}>{job.status}</Badge>
              </Card.Header>
              <Card.Body>
                {job.workflow_name ? (
                  <div className="mb-2">
                    <strong>Workflow:</strong>{' '}
                    <span className="small text-muted">{job.workflow_name}</span>
                  </div>
                ) : job.wasm_module_name ? (
                  <div className="mb-2">
                    <strong>WASM Module:</strong>{' '}
                    <span className="small text-muted">{job.wasm_module_name}</span>
                  </div>
                ) : (
                  <div className="mb-2">
                    <strong>Workflow ID:</strong>{' '}
                    <span className="small text-muted">{job.workflow_id}</span>
                  </div>
                )}
                {job.working_directory && (
                  <div className="mb-2">
                    <strong>Working Directory:</strong>{' '}
                    <span className="small text-muted">{job.working_directory}</span>
                  </div>
                )}
                <div className="mb-2">
                  <strong>Created:</strong>{' '}
                  <span className="small text-muted">{formatDate(job.created_at)}</span>
                </div>
                <div className="mb-2">
                  <strong>Started:</strong>{' '}
                  <span className="small text-muted">{formatDate(job.started_at)}</span>
                </div>
                <div className="mb-3">
                  <strong>Completed:</strong>{' '}
                  <span className="small text-muted">{formatDate(job.completed_at)}</span>
                </div>

                {job.input_data && (
                  <div className="mb-3">
                    <strong>Input:</strong>
                    <Form.Control
                      as="textarea"
                      rows={3}
                      readOnly
                      value={JSON.stringify(job.input_data, null, 2)}
                      className="mt-1 small"
                      style={{ fontFamily: 'monospace', fontSize: '12px' }}
                    />
                  </div>
                )}

                {job.output_data && (
                  <div className="mb-3">
                    <strong>Output:</strong>
                    <Form.Control
                      as="textarea"
                      rows={4}
                      readOnly
                      value={JSON.stringify(job.output_data, null, 2)}
                      className="mt-1 small"
                      style={{ fontFamily: 'monospace', fontSize: '12px' }}
                    />
                  </div>
                )}

                <Button
                  variant="outline-primary"
                  size="sm"
                  onClick={() => openJobDetails(job)}
                  className="me-2"
                >
                  View Details
                </Button>

                {(job.status === 'running' || job.status === 'queued') && (
                  <Button
                    variant="outline-danger"
                    size="sm"
                    onClick={() => cancelJob(job.id)}
                  >
                    Cancel
                  </Button>
                )}
              </Card.Body>
            </Card>
          </Col>
        ))}
      </Row>

      {jobs.length === 0 && (
        <Card>
          <Card.Body className="text-center text-muted">
            <h4>No jobs found</h4>
            <p>Jobs will appear here when workflows are executed</p>
          </Card.Body>
        </Card>
      )}

      {/* Job Details Modal */}
      <Modal
        show={showDetailsModal}
        onHide={() => setShowDetailsModal(false)}
        size="lg"
      >
        <Modal.Header closeButton>
          <Modal.Title>
            Job Details - {selectedJob?.id?.substring(0, 8)}...
            {selectedJob?.workflow_name && (
              <div className="small text-muted">Workflow: {selectedJob.workflow_name}</div>
            )}
            {selectedJob?.wasm_module_name && (
              <div className="small text-muted">WASM Module: {selectedJob.wasm_module_name}</div>
            )}
          </Modal.Title>
        </Modal.Header>
        <Modal.Body>
          {selectedJob && (
            <div>
              <Row className="mb-3">
                <Col md={6}>
                  <strong>Status:</strong>{' '}
                  <Badge bg={getStatusVariant(selectedJob.status)}>
                    {selectedJob.status}
                  </Badge>
                </Col>
                <Col md={6}>
                  {selectedJob.workflow_name ? (
                    <div>
                      <strong>Workflow:</strong> {selectedJob.workflow_name}
                    </div>
                  ) : selectedJob.wasm_module_name ? (
                    <div>
                      <strong>WASM Module:</strong> {selectedJob.wasm_module_name}
                    </div>
                  ) : (
                    <div>
                      <strong>Workflow ID:</strong> {selectedJob.workflow_id}
                    </div>
                  )}
                </Col>
              </Row>

              {selectedJob.working_directory && (
                <Row className="mb-3">
                  <Col md={12}>
                    <strong>Working Directory:</strong> {selectedJob.working_directory}
                  </Col>
                </Row>
              )}

              <Row className="mb-3">
                <Col md={6}>
                  <strong>Created:</strong> {formatDate(selectedJob.created_at)}
                </Col>
                <Col md={6}>
                  <strong>Started:</strong> {formatDate(selectedJob.started_at)}
                </Col>
              </Row>

              <Row className="mb-3">
                <Col md={6}>
                  <strong>Completed:</strong> {formatDate(selectedJob.completed_at)}
                </Col>
              </Row>

              {selectedJob.input_data && (
                <Row className="mb-3">
                  <Col md={12}>
                    <strong>Input Data:</strong>
                    <Form.Control
                      as="textarea"
                      rows={4}
                      readOnly
                      value={JSON.stringify(selectedJob.input_data, null, 2)}
                      className="mt-1 small"
                      style={{ fontFamily: 'monospace', fontSize: '12px' }}
                    />
                  </Col>
                </Row>
              )}

              {selectedJob.output_data && (
                <Row className="mb-3">
                  <Col md={12}>
                    <strong>Output Data:</strong>
                    <Form.Control
                      as="textarea"
                      rows={6}
                      readOnly
                      value={JSON.stringify(selectedJob.output_data, null, 2)}
                      className="mt-1 small"
                      style={{ fontFamily: 'monospace', fontSize: '12px' }}
                    />
                  </Col>
                </Row>
              )}

              <h5 className="mt-4">Job Steps</h5>
              {jobSteps.length === 0 ? (
                <p className="text-muted">No steps found for this job</p>
              ) : (
                <ListGroup>
                  {jobSteps.map((step, index) => (
                    <ListGroup.Item key={step.id}>
                      <div className="d-flex justify-content-between align-items-center">
                        <div>
                          <strong>Step {index + 1}:</strong>{' '}
                          {step.agent_name ? (
                            <span>Agent: {step.agent_name}</span>
                          ) : step.wasm_module_name ? (
                            <span>WASM Module: {step.wasm_module_name}</span>
                          ) : (
                            <span>{step.workflow_step_id}</span>
                          )}
                        </div>
                        <Badge bg={getStatusVariant(step.status)}>
                          {step.status}
                        </Badge>
                      </div>

                      <div className="mt-2">
                        <small className="text-muted">
                          Started: {formatDate(step.started_at)} |
                          Completed: {formatDate(step.completed_at)}
                        </small>
                      </div>

                      {step.input_data && (
                        <div className="mt-2">
                          <strong>Input:</strong>
                          <Form.Control
                            as="textarea"
                            rows={3}
                            readOnly
                            value={JSON.stringify(step.input_data, null, 2)}
                            className="mt-1 small"
                            style={{ fontFamily: 'monospace', fontSize: '12px' }}
                          />
                        </div>
                      )}

                      {step.output_data && (
                        <div className="mt-2">
                          <strong>Output:</strong>
                          <Form.Control
                            as="textarea"
                            rows={3}
                            readOnly
                            value={JSON.stringify(step.output_data, null, 2)}
                            className="mt-1 small"
                            style={{ fontFamily: 'monospace', fontSize: '12px' }}
                          />
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
          {selectedJob && (selectedJob.status === 'running' || selectedJob.status === 'queued') && (
            <Button variant="danger" onClick={() => cancelJob(selectedJob.id)}>
              Cancel Job
            </Button>
          )}
          <Button variant="secondary" onClick={() => setShowDetailsModal(false)}>
            Close
          </Button>
        </Modal.Footer>
      </Modal>
    </div>
  );
}

export default Jobs;