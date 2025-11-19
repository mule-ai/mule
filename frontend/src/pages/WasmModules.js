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
      // Convert file to base64 for JSON transport
      const moduleData = await new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => {
          const result = reader.result;
          // Remove data URL prefix and get base64
          const base64 = result.split(',')[1];
          resolve(base64);
        };
        reader.onerror = reject;
        reader.readAsDataURL(newModule.module_data);
      });

      await wasmModulesAPI.create({
        ...newModule,
        module_data: newModule.module_data,
      });
      
      setShowCreateModal(false);
      setNewModule({
        name: '',
        description: '',
        module_data: null,
      });
      loadModules();
    } catch (error) {
      setError(error.response?.data?.error || 'Failed to create WASM module');
    } finally {
      setLoading(false);
    }
  };

  const handleUpdateModule = async () => {
    setLoading(true);
    setError('');
    
    try {
      const updateData = {
        name: selectedModule.name,
        description: selectedModule.description,
      };

      // Only update module data if a new file is provided
      if (selectedModule.new_module_data) {
        const moduleData = await new Promise((resolve, reject) => {
          const reader = new FileReader();
          reader.onload = () => {
            const result = reader.result;
            const base64 = result.split(',')[1];
            resolve(base64);
          };
          reader.onerror = reject;
          reader.readAsDataURL(selectedModule.new_module_data);
        });
        updateData.module_data = selectedModule.new_module_data;
      }

      await wasmModulesAPI.update(selectedModule.id, updateData);
      setShowEditModal(false);
      setSelectedModule(null);
      loadModules();
    } catch (error) {
      setError(error.response?.data?.error || 'Failed to update WASM module');
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
    setSelectedModule({
      ...module,
      new_module_data: null,
    });
    setShowEditModal(true);
  };

  const formatFileSize = (bytes) => {
    if (!bytes) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
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
                  <strong>Size:</strong>{' '}
                  <span className="small text-muted">
                    {formatFileSize(module.module_data?.length || 0)}
                  </span>
                </div>
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