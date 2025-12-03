import React, { useState, useEffect } from 'react';
import { Card, Row, Col, Button, Form, Modal, Alert } from 'react-bootstrap';
import { wasmModulesAPI } from '../services/api';

function WasmModules() {
  const [modules, setModules] = useState([]);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showEditModal, setShowEditModal] = useState(false);
  const [selectedModule, setSelectedModule] = useState(null);
  const [newModule, setNewModule] = useState({
    name: '',
    description: '',
    module_data: null,
    config: '',
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    loadModules();
  }, []);

  const loadModules = async () => {
    try {
      const response = await wasmModulesAPI.list();
      setModules(response.data.data || []);
    } catch (error) {
      console.error('Failed to load WASM modules:', error);
    }
  };

  const handleCreateModule = async () => {
    setLoading(true);
    setError('');

    try {
      // Validate config if provided
      if (newModule.config) {
        try {
          JSON.parse(newModule.config);
        } catch (e) {
          throw new Error('Configuration must be valid JSON');
        }
      }

      // Pass plain JS object instead of FormData
      const moduleData = {
        name: newModule.name,
        description: newModule.description,
        module_data: newModule.module_data,
      };
      if (newModule.config) {
        moduleData.config = newModule.config;
      }

      await wasmModulesAPI.create(moduleData);

      setShowCreateModal(false);
      setNewModule({
        name: '',
        description: '',
        module_data: null,
        config: '',
      });
      loadModules();
    } catch (error) {
      setError(error.message || error.response?.data?.error || 'Failed to create WASM module');
    } finally {
      setLoading(false);
    }
  };

  const handleUpdateModule = async () => {
    setLoading(true);
    setError('');

    try {
      // Validate config if provided
      if (selectedModule.config) {
        try {
          JSON.parse(selectedModule.config);
        } catch (e) {
          throw new Error('Configuration must be valid JSON');
        }
      }

      // Pass plain JS object instead of FormData
      const moduleData = {
        name: selectedModule.name,
        description: selectedModule.description,
      };

      // Only update module data if a new file is provided
      if (selectedModule.new_module_data) {
        moduleData.module_data = selectedModule.new_module_data;
      }

      if (selectedModule.config !== undefined) {
        moduleData.config = selectedModule.config;
      }

      await wasmModulesAPI.update(selectedModule.id, moduleData);
      setShowEditModal(false);
      setSelectedModule(null);
      loadModules();
    } catch (error) {
      setError(error.message || error.response?.data?.error || 'Failed to update WASM module');
    } finally {
      setLoading(false);
    }
  };

  const handleDeleteModule = async (moduleId) => {
    if (window.confirm('Are you sure you want to delete this WASM module?')) {
      try {
        await wasmModulesAPI.delete(moduleId);
        loadModules();
      } catch (error) {
        setError(error.response?.data?.error || 'Failed to delete WASM module');
      }
    }
  };

  const openEditModal = (module) => {
    let configStr = '';
    if (module.config) {
      try {
        // Config is a JSON object when coming from the API
        if (typeof module.config === 'string') {
          // If it's already a string, try to parse it as JSON
          configStr = JSON.stringify(JSON.parse(module.config), null, 2);
        } else {
          // If it's an object, stringify it directly
          configStr = JSON.stringify(module.config, null, 2);
        }
      } catch (e) {
        console.error('Failed to parse config:', e);
        // If parsing fails, use the raw config
        configStr = typeof module.config === 'string' ? module.config : JSON.stringify(module.config);
      }
    }

    setSelectedModule({
      ...module,
      new_module_data: null,
      config: configStr,
    });
    setShowEditModal(true);
  };


  return (
    <div>
      <div className="d-flex justify-content-between align-items-center mb-4">
        <h1>WASM Modules</h1>
        <Button variant="primary" onClick={() => setShowCreateModal(true)}>
          Upload Module
        </Button>
      </div>

      {error && <Alert variant="danger" dismissible onClose={() => setError('')}>{error}</Alert>}

      <Row>
        {modules.map((module) => (
          <Col md={6} lg={4} className="mb-4" key={module.id}>
            <Card>
              <Card.Header>
                <Card.Title className="mb-0">{module.name}</Card.Title>
              </Card.Header>
              <Card.Body>
                <p className="text-muted">{module.description}</p>
                <div className="mb-2">
                  <strong>Created:</strong>{' '}
                  <span className="small text-muted">
                    {new Date(module.created_at).toLocaleDateString()}
                  </span>
                </div>
                <div className="mb-3">
                  <strong>Updated:</strong>{' '}
                  <span className="small text-muted">
                    {new Date(module.updated_at).toLocaleDateString()}
                  </span>
                </div>
                <div className="d-flex gap-2">
                  <Button
                    variant="outline-primary"
                    size="sm"
                    onClick={() => openEditModal(module)}
                  >
                    Edit
                  </Button>
                  <Button
                    variant="outline-danger"
                    size="sm"
                    onClick={() => handleDeleteModule(module.id)}
                  >
                    Delete
                  </Button>
                </div>
              </Card.Body>
            </Card>
          </Col>
        ))}
      </Row>

      {modules.length === 0 && (
        <Card>
          <Card.Body className="text-center text-muted">
            <h4>No WASM modules found</h4>
            <p>Upload your first WebAssembly module to get started</p>
          </Card.Body>
        </Card>
      )}

      {/* Create Module Modal */}
      <Modal show={showCreateModal} onHide={() => setShowCreateModal(false)} size="lg">
        <Modal.Header closeButton>
          <Modal.Title>Upload WASM Module</Modal.Title>
        </Modal.Header>
        <Modal.Body>
          <Form>
            <Form.Group className="mb-3">
              <Form.Label>Name</Form.Label>
              <Form.Control
                type="text"
                value={newModule.name}
                onChange={(e) =>
                  setNewModule({ ...newModule, name: e.target.value })
                }
                placeholder="e.g., data-processor, text-analyzer"
                required
              />
            </Form.Group>
            <Form.Group className="mb-3">
              <Form.Label>Description</Form.Label>
              <Form.Control
                as="textarea"
                rows={3}
                value={newModule.description}
                onChange={(e) =>
                  setNewModule({ ...newModule, description: e.target.value })
                }
                placeholder="Describe what this module does..."
              />
            </Form.Group>
            <Form.Group className="mb-3">
              <Form.Label>Configuration (JSON, optional)</Form.Label>
              <Form.Control
                as="textarea"
                rows={4}
                value={newModule.config}
                onChange={(e) =>
                  setNewModule({ ...newModule, config: e.target.value })
                }
                placeholder={`{
  "api_key": "your-api-key",
  "endpoint": "https://api.example.com",
  "timeout": 30
}`}
              />
              <Form.Text className="text-muted">
                Optional JSON configuration that will be merged with input data when the module executes
              </Form.Text>
            </Form.Group>
            <Form.Group className="mb-3">
              <Form.Label>WASM File (.wasm)</Form.Label>
              <Form.Control
                type="file"
                accept=".wasm"
                onChange={(e) =>
                  setNewModule({ ...newModule, module_data: e.target.files[0] })
                }
                required
              />
              <Form.Text className="text-muted">
                Select a compiled WebAssembly (.wasm) file
              </Form.Text>
            </Form.Group>
          </Form>
        </Modal.Body>
        <Modal.Footer>
          <Button variant="secondary" onClick={() => setShowCreateModal(false)}>
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={handleCreateModule}
            disabled={loading || !newModule.name || !newModule.module_data}
          >
            {loading ? 'Uploading...' : 'Upload Module'}
          </Button>
        </Modal.Footer>
      </Modal>

      {/* Edit Module Modal */}
      <Modal show={showEditModal} onHide={() => setShowEditModal(false)} size="lg">
        <Modal.Header closeButton>
          <Modal.Title>Edit WASM Module</Modal.Title>
        </Modal.Header>
        <Modal.Body>
          {selectedModule && (
            <Form>
              <Form.Group className="mb-3">
                <Form.Label>Name</Form.Label>
                <Form.Control
                  type="text"
                  value={selectedModule.name}
                  onChange={(e) =>
                    setSelectedModule({ ...selectedModule, name: e.target.value })
                  }
                  required
                />
              </Form.Group>
              <Form.Group className="mb-3">
                <Form.Label>Description</Form.Label>
                <Form.Control
                  as="textarea"
                  rows={3}
                  value={selectedModule.description}
                  onChange={(e) =>
                    setSelectedModule({ ...selectedModule, description: e.target.value })
                  }
                />
              </Form.Group>
              <Form.Group className="mb-3">
                <Form.Label>Configuration (JSON, optional)</Form.Label>
                <Form.Control
                  as="textarea"
                  rows={4}
                  value={selectedModule.config}
                  onChange={(e) =>
                    setSelectedModule({ ...selectedModule, config: e.target.value })
                  }
                  placeholder={`{
  "api_key": "your-api-key",
  "endpoint": "https://api.example.com",
  "timeout": 30
}`}
                />
                <Form.Text className="text-muted">
                  Optional JSON configuration that will be merged with input data when the module executes
                </Form.Text>
              </Form.Group>
              <Form.Group className="mb-3">
                <Form.Label>Update WASM File (optional)</Form.Label>
                <Form.Control
                  type="file"
                  accept=".wasm"
                  onChange={(e) =>
                    setSelectedModule({ ...selectedModule, new_module_data: e.target.files[0] })
                  }
                />
                <Form.Text className="text-muted">
                  Leave empty to keep the current module file
                </Form.Text>
              </Form.Group>
            </Form>
          )}
        </Modal.Body>
        <Modal.Footer>
          <Button variant="secondary" onClick={() => setShowEditModal(false)}>
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={handleUpdateModule}
            disabled={loading || !selectedModule?.name}
          >
            {loading ? 'Updating...' : 'Update Module'}
          </Button>
        </Modal.Footer>
      </Modal>
    </div>
  );
}

export default WasmModules;