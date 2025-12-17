import React, { useState, useEffect, useCallback } from 'react';
import { Card, Row, Col, Badge, Button, Modal, ListGroup, Form, Pagination, InputGroup, FormControl } from 'react-bootstrap';
import { jobsAPI } from '../services/api';

function Jobs() {
  const [jobs, setJobs] = useState([]);
  const [selectedJob, setSelectedJob] = useState(null);
  const [jobSteps, setJobSteps] = useState([]);
  const [showDetailsModal, setShowDetailsModal] = useState(false);

  // Pagination and filtering state
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [totalPages, setTotalPages] = useState(0);
  const [totalCount, setTotalCount] = useState(0);
  const [statusFilter, setStatusFilter] = useState('');
  const [searchQuery, setSearchQuery] = useState('');
  const [workflowNameFilter, setWorkflowNameFilter] = useState('');

  const loadJobs = useCallback(async () => {
    try {
      const params = {
        page: currentPage,
        page_size: pageSize,
        status: statusFilter || undefined,
        search: searchQuery || undefined,
        workflow_name: workflowNameFilter || undefined
      };

      const response = await jobsAPI.list(params);
      setJobs(response.data.jobs || []);
      setTotalPages(response.data.total_pages || 0);
      setTotalCount(response.data.total_count || 0);
    } catch (error) {
      console.error('Failed to load jobs:', error);
    }
  }, [currentPage, pageSize, statusFilter, searchQuery, workflowNameFilter]);

  useEffect(() => {
    loadJobs();
    // Poll for updates every 5 seconds
    const interval = setInterval(loadJobs, 5000);
    return () => clearInterval(interval);
  }, [currentPage, pageSize, statusFilter, searchQuery, workflowNameFilter, loadJobs]);

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

  // Pagination handlers
  const handlePageChange = (page) => {
    setCurrentPage(page);
  };

  const handlePageSizeChange = (size) => {
    setPageSize(size);
    setCurrentPage(1); // Reset to first page when changing page size
  };

  const handleStatusFilterChange = (status) => {
    setStatusFilter(status);
    setCurrentPage(1); // Reset to first page when changing filter
  };

  const handleSearchChange = (query) => {
    setSearchQuery(query);
    setCurrentPage(1); // Reset to first page when changing search
  };

  const handleWorkflowNameFilterChange = (name) => {
    setWorkflowNameFilter(name);
    setCurrentPage(1); // Reset to first page when changing filter
  };

  // Render pagination controls
  const renderPagination = () => {
    const items = [];
    const maxVisiblePages = 5;

    let startPage = Math.max(1, currentPage - Math.floor(maxVisiblePages / 2));
    let endPage = Math.min(totalPages, startPage + maxVisiblePages - 1);

    if (endPage - startPage + 1 < maxVisiblePages) {
      startPage = Math.max(1, endPage - maxVisiblePages + 1);
    }

    // Previous button
    items.push(
      <Pagination.Prev
        key="prev"
        onClick={() => handlePageChange(currentPage - 1)}
        disabled={currentPage === 1}
      />
    );

    // First page
    if (startPage > 1) {
      items.push(
        <Pagination.Item key={1} onClick={() => handlePageChange(1)}>
          1
        </Pagination.Item>
      );
      if (startPage > 2) {
        items.push(<Pagination.Ellipsis key="start-ellipsis" />);
      }
    }

    // Page numbers
    for (let i = startPage; i <= endPage; i++) {
      items.push(
        <Pagination.Item
          key={i}
          active={i === currentPage}
          onClick={() => handlePageChange(i)}
        >
          {i}
        </Pagination.Item>
      );
    }

    // Last page
    if (endPage < totalPages) {
      if (endPage < totalPages - 1) {
        items.push(<Pagination.Ellipsis key="end-ellipsis" />);
      }
      items.push(
        <Pagination.Item key={totalPages} onClick={() => handlePageChange(totalPages)}>
          {totalPages}
        </Pagination.Item>
      );
    }

    // Next button
    items.push(
      <Pagination.Next
        key="next"
        onClick={() => handlePageChange(currentPage + 1)}
        disabled={currentPage === totalPages}
      />
    );

    return (
      <div className="d-flex justify-content-between align-items-center mt-3">
        <div>
          Showing {Math.min((currentPage - 1) * pageSize + 1, totalCount)} to {Math.min(currentPage * pageSize, totalCount)} of {totalCount} jobs
        </div>
        <Pagination className="mb-0">
          {items}
        </Pagination>
        <div>
          <Form.Select
            size="sm"
            value={pageSize}
            onChange={(e) => handlePageSizeChange(parseInt(e.target.value))}
            style={{ width: 'auto', display: 'inline-block' }}
          >
            <option value={10}>10 per page</option>
            <option value={20}>20 per page</option>
            <option value={50}>50 per page</option>
            <option value={100}>100 per page</option>
          </Form.Select>
        </div>
      </div>
    );
  };

  return (
    <div>
      <div className="d-flex justify-content-between align-items-center mb-4">
        <h1>Jobs</h1>
        <Button variant="outline-primary" onClick={loadJobs}>
          Refresh
        </Button>
      </div>

      {/* Filters */}
      <Card className="mb-4">
        <Card.Body>
          <Row>
            <Col md={4} className="mb-3 mb-md-0">
              <InputGroup>
                <InputGroup.Text>Search</InputGroup.Text>
                <FormControl
                  type="text"
                  placeholder="Search by workflow ID or working directory..."
                  value={searchQuery}
                  onChange={(e) => handleSearchChange(e.target.value)}
                />
              </InputGroup>
            </Col>
            <Col md={4} className="mb-3 mb-md-0">
              <InputGroup>
                <InputGroup.Text>Workflow</InputGroup.Text>
                <FormControl
                  type="text"
                  placeholder="Filter by workflow name..."
                  value={workflowNameFilter}
                  onChange={(e) => handleWorkflowNameFilterChange(e.target.value)}
                />
              </InputGroup>
            </Col>
            <Col md={4}>
              <InputGroup>
                <InputGroup.Text>Status</InputGroup.Text>
                <Form.Select
                  value={statusFilter}
                  onChange={(e) => handleStatusFilterChange(e.target.value)}
                >
                  <option value="">All Statuses</option>
                  <option value="queued">Queued</option>
                  <option value="running">Running</option>
                  <option value="completed">Completed</option>
                  <option value="failed">Failed</option>
                  <option value="cancelled">Cancelled</option>
                </Form.Select>
              </InputGroup>
            </Col>
          </Row>
        </Card.Body>
      </Card>

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
            <p>Try adjusting your filters or jobs will appear here when workflows are executed</p>
          </Card.Body>
        </Card>
      )}

      {/* Pagination */}
      {totalPages > 1 && renderPagination()}

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