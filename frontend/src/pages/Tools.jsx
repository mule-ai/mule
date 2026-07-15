import React, { useState, useEffect } from 'react';
import { Card, Row, Col, Button, Form, Modal } from 'react-bootstrap';
import { toolsAPI } from '../services/api';
import MemoryConfig from '../components/MemoryConfig';

function Tools() {
  const [tools, setTools] = useState([]);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showEditModal, setShowEditModal] = useState(false);
  const [selectedTool, setSelectedTool] = useState(null);
  const [newTool, setNewTool] = useState({
    name: '',
    description: '',
    metadata: {
      tool_type: 'http',
      config: {},
    },
  });
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    loadTools();
  }, []);

  const loadTools = async () => {
    try {
      const response = await toolsAPI.list();
      setTools(response.data || []);
    } catch (error) {
      console.error('Failed to load tools:', error);
    }
  };

  const handleCreateTool = async () => {
    setLoading(true);
    try {
      await toolsAPI.create(newTool);
      setShowCreateModal(false);
      setNewTool({
        name: '',
        description: '',
        metadata: {
          tool_type: 'http',
          config: {},
        },
      });
      loadTools();
    } catch (error) {
      console.error('Failed to create tool:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleUpdateTool = async () => {
    setLoading(true);
    try {
      await toolsAPI.update(selectedTool.id, selectedTool);
      setShowEditModal(false);
      setSelectedTool(null);
      loadTools();
    } catch (error) {
      console.error('Failed to update tool:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleDeleteTool = async (toolId) => {
    if (window.confirm('Are you sure you want to delete this tool?')) {
      try {
        await toolsAPI.delete(toolId);
        loadTools();
      } catch (error) {
        console.error('Failed to delete tool:', error);
      }
    }
  };

  const openEditModal = (tool) => {
    setSelectedTool(tool);
    setShowEditModal(true);
  };

  return (
    <div>
      <div className="d-flex justify-content-between align-items-center mb-4">
        <h1>Tools</h1>
        <Button variant="primary" onClick={() => setShowCreateModal(true)}>
          Create Tool
        </Button>
      </div>

      <MemoryConfig />

      <Row>
        {tools.map((tool) => (
          <Col md={6} lg={4} className="mb-4" key={tool.id}>
            <Card>
              <Card.Header>
                <Card.Title className="mb-0">{tool.name}</Card.Title>
              </Card.Header>
              <Card.Body>
                <p className="text-muted">{tool.description}</p>
                <div className="mb-2">
                  <strong>Type:</strong>{' '}
                  <span className="badge bg-secondary">
                    {tool.metadata?.tool_type || 'unknown'}
                  </span>
                </div>
                <div className="mb-3">
                  <strong>Configuration:</strong>
                  <pre className="small text-muted mt-1">
                    {JSON.stringify(tool.metadata?.config || {}, null, 2)}
                  </pre>
                </div>
                <div className="d-flex gap-2">
                  <Button
                    variant="outline-primary"
                    size="sm"
                    onClick={() => openEditModal(tool)}
                  >
                    Edit
                  </Button>
                  <Button
                    variant="outline-danger"
                    size="sm"
                    onClick={() => handleDeleteTool(tool.id)}
                  >
                    Delete
                  </Button>
                </div>
              </Card.Body>
            </Card>
          </Col>
        ))}
      </Row>

      {/* Create Tool Modal */}
      <Modal show={showCreateModal} onHide={() => setShowCreateModal(false)}>
        <Modal.Header closeButton>
          <Modal.Title>Create New Tool</Modal.Title>
        </Modal.Header>
        <Modal.Body>
          <Form>
            <Form.Group className="mb-3">
              <Form.Label>Name</Form.Label>
              <Form.Control
                type="text"
                value={newTool.name}
                onChange={(e) =>
                  setNewTool({ ...newTool, name: e.target.value })
                }
                required
              />
            </Form.Group>
            <Form.Group className="mb-3">
              <Form.Label>Description</Form.Label>
              <Form.Control
                as="textarea"
                rows={2}
                value={newTool.description}
                onChange={(e) =>
                  setNewTool({ ...newTool, description: e.target.value })
                }
              />
            </Form.Group>
            <Form.Group className="mb-3">
              <Form.Label>Type</Form.Label>
              <Form.Select
                value={newTool.metadata.tool_type}
                onChange={(e) =>
                  setNewTool({
                    ...newTool,
                    metadata: { ...newTool.metadata, tool_type: e.target.value }
                  })
                }
              >
                <option value="http">HTTP</option>
                <option value="database">Database</option>
                <option value="memory">Memory</option>
                <option value="filesystem">Filesystem</option>
                <option value="bash">Bash</option>
              </Form.Select>
            </Form.Group>
            <Form.Group className="mb-3">
              <Form.Label>Configuration (JSON)</Form.Label>
              <Form.Control
                as="textarea"
                rows={4}
                value={JSON.stringify(newTool.metadata.config, null, 2)}
                onChange={(e) => {
                  try {
                    const config = JSON.parse(e.target.value);
                    setNewTool({
                      ...newTool,
                      metadata: { ...newTool.metadata, config }
                    });
                  } catch (err) {
                    // Invalid JSON, don't update state
                  }
                }}
                placeholder='{"url": "https://api.example.com", "method": "GET"}'
              />
            </Form.Group>
          </Form>
        </Modal.Body>
        <Modal.Footer>
          <Button variant="secondary" onClick={() => setShowCreateModal(false)}>
            Cancel
          </Button>
          <Button variant="primary" onClick={handleCreateTool} disabled={loading}>
            {loading ? 'Creating...' : 'Create'}
          </Button>
        </Modal.Footer>
      </Modal>

      {/* Edit Tool Modal */}
      <Modal show={showEditModal} onHide={() => setShowEditModal(false)}>
        <Modal.Header closeButton>
          <Modal.Title>Edit Tool</Modal.Title>
        </Modal.Header>
        <Modal.Body>
          {selectedTool && (
            <Form>
              <Form.Group className="mb-3">
                <Form.Label>Name</Form.Label>
                <Form.Control
                  type="text"
                  value={selectedTool.name}
                  onChange={(e) =>
                    setSelectedTool({ ...selectedTool, name: e.target.value })
                  }
                  required
                />
              </Form.Group>
              <Form.Group className="mb-3">
                <Form.Label>Description</Form.Label>
                <Form.Control
                  as="textarea"
                  rows={2}
                  value={selectedTool.description}
                  onChange={(e) =>
                    setSelectedTool({ ...selectedTool, description: e.target.value })
                  }
                />
              </Form.Group>
              <Form.Group className="mb-3">
                <Form.Label>Type</Form.Label>
                <Form.Select
                  value={selectedTool.metadata?.tool_type || 'http'}
                  onChange={(e) =>
                    setSelectedTool({
                      ...selectedTool,
                      metadata: { ...selectedTool.metadata, tool_type: e.target.value }
                    })
                  }
                >
                  <option value="http">HTTP</option>
                  <option value="database">Database</option>
                  <option value="memory">Memory</option>
                  <option value="filesystem">Filesystem</option>
                  <option value="bash">Bash</option>
                </Form.Select>
              </Form.Group>
              <Form.Group className="mb-3">
                <Form.Label>Configuration (JSON)</Form.Label>
                <Form.Control
                  as="textarea"
                  rows={4}
                  value={JSON.stringify(selectedTool.metadata?.config || {}, null, 2)}
                  onChange={(e) => {
                    try {
                      const config = JSON.parse(e.target.value);
                      setSelectedTool({
                        ...selectedTool,
                        metadata: { ...selectedTool.metadata, config }
                      });
                    } catch (err) {
                      // Invalid JSON, don't update state
                    }
                  }}
                />
              </Form.Group>
            </Form>
          )}
        </Modal.Body>
        <Modal.Footer>
          <Button variant="secondary" onClick={() => setShowEditModal(false)}>
            Cancel
          </Button>
          <Button variant="primary" onClick={handleUpdateTool} disabled={loading}>
            {loading ? 'Updating...' : 'Update'}
          </Button>
        </Modal.Footer>
      </Modal>
    </div>
  );
}

export default Tools;