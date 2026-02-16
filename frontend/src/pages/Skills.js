import React, { useState, useEffect } from 'react';
import { Card, Row, Col, Button, Form, Modal } from 'react-bootstrap';
import { skillsAPI } from '../services/api';

function Skills() {
  const [skills, setSkills] = useState([]);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showEditModal, setShowEditModal] = useState(false);
  const [selectedSkill, setSelectedSkill] = useState(null);
  const [newSkill, setNewSkill] = useState({
    name: '',
    description: '',
    path: '',
    enabled: true,
  });
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    loadSkills();
  }, []);

  const loadSkills = async () => {
    try {
      const response = await skillsAPI.list();
      setSkills(response.data || []);
    } catch (error) {
      console.error('Failed to load skills:', error);
    }
  };

  const handleCreateSkill = async () => {
    setLoading(true);
    try {
      await skillsAPI.create(newSkill);
      setShowCreateModal(false);
      setNewSkill({
        name: '',
        description: '',
        path: '',
        enabled: true,
      });
      loadSkills();
    } catch (error) {
      console.error('Failed to create skill:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleUpdateSkill = async () => {
    setLoading(true);
    try {
      await skillsAPI.update(selectedSkill.id, selectedSkill);
      setShowEditModal(false);
      setSelectedSkill(null);
      loadSkills();
    } catch (error) {
      console.error('Failed to update skill:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleDeleteSkill = async (skillId) => {
    if (window.confirm('Are you sure you want to delete this skill?')) {
      try {
        await skillsAPI.delete(skillId);
        loadSkills();
      } catch (error) {
        console.error('Failed to delete skill:', error);
      }
    }
  };

  const openEditModal = (skill) => {
    setSelectedSkill(skill);
    setShowEditModal(true);
  };

  const toggleSkillEnabled = async (skill) => {
    try {
      await skillsAPI.update(skill.id, { ...skill, enabled: !skill.enabled });
      loadSkills();
    } catch (error) {
      console.error('Failed to toggle skill enabled:', error);
    }
  };

  return (
    <div>
      <div className="d-flex justify-content-between align-items-center mb-4">
        <h1>Skills</h1>
        <Button variant="primary" onClick={() => setShowCreateModal(true)}>
          Create Skill
        </Button>
      </div>

      <p className="text-muted mb-4">
        Skills are pi agent capabilities that can be assigned to agents. 
        Skills provide additional tools and functionality to agents during execution.
      </p>

      <Row>
        {skills.length === 0 ? (
          <Col>
            <Card body className="text-center text-muted">
              No skills found. Create a skill to get started.
            </Card>
          </Col>
        ) : (
          skills.map((skill) => (
            <Col md={6} lg={4} className="mb-4" key={skill.id}>
              <Card>
                <Card.Header>
                  <div className="d-flex justify-content-between align-items-center">
                    <Card.Title className="mb-0">{skill.name}</Card.Title>
                    <span className={`badge ${skill.enabled ? 'bg-success' : 'bg-secondary'}`}>
                      {skill.enabled ? 'Enabled' : 'Disabled'}
                    </span>
                  </div>
                </Card.Header>
                <Card.Body>
                  <p className="text-muted">{skill.description || 'No description'}</p>
                  <div className="mb-2">
                    <strong>Path:</strong>{' '}
                    <code className="small">{skill.path}</code>
                  </div>
                  <div className="d-flex gap-2 mt-3">
                    <Button
                      variant="outline-primary"
                      size="sm"
                      onClick={() => openEditModal(skill)}
                    >
                      Edit
                    </Button>
                    <Button
                      variant={skill.enabled ? 'outline-warning' : 'outline-success'}
                      size="sm"
                      onClick={() => toggleSkillEnabled(skill)}
                    >
                      {skill.enabled ? 'Disable' : 'Enable'}
                    </Button>
                    <Button
                      variant="outline-danger"
                      size="sm"
                      onClick={() => handleDeleteSkill(skill.id)}
                    >
                      Delete
                    </Button>
                  </div>
                </Card.Body>
              </Card>
            </Col>
          ))
        )}
      </Row>

      {/* Create Skill Modal */}
      <Modal show={showCreateModal} onHide={() => setShowCreateModal(false)}>
        <Modal.Header closeButton>
          <Modal.Title>Create New Skill</Modal.Title>
        </Modal.Header>
        <Modal.Body>
          <Form>
            <Form.Group className="mb-3">
              <Form.Label>Name</Form.Label>
              <Form.Control
                type="text"
                value={newSkill.name}
                onChange={(e) =>
                  setNewSkill({ ...newSkill, name: e.target.value })
                }
                placeholder="e.g., spec-plan-generator"
                required
              />
              <Form.Text className="text-muted">
                Unique identifier for the skill
              </Form.Text>
            </Form.Group>
            <Form.Group className="mb-3">
              <Form.Label>Description</Form.Label>
              <Form.Control
                as="textarea"
                rows={2}
                value={newSkill.description}
                onChange={(e) =>
                  setNewSkill({ ...newSkill, description: e.target.value })
                }
                placeholder="Describe what this skill does"
              />
            </Form.Group>
            <Form.Group className="mb-3">
              <Form.Label>Path</Form.Label>
              <Form.Control
                type="text"
                value={newSkill.path}
                onChange={(e) =>
                  setNewSkill({ ...newSkill, path: e.target.value })
                }
                placeholder="e.g., /root/.pi/agent/skills/spec-plan-generator"
                required
              />
              <Form.Text className="text-muted">
                Absolute path to the skill directory
              </Form.Text>
            </Form.Group>
            <Form.Group className="mb-3">
              <Form.Check
                type="checkbox"
                label="Enabled"
                checked={newSkill.enabled}
                onChange={(e) =>
                  setNewSkill({ ...newSkill, enabled: e.target.checked })
                }
              />
            </Form.Group>
          </Form>
        </Modal.Body>
        <Modal.Footer>
          <Button variant="secondary" onClick={() => setShowCreateModal(false)}>
            Cancel
          </Button>
          <Button variant="primary" onClick={handleCreateSkill} disabled={loading || !newSkill.name || !newSkill.path}>
            {loading ? 'Creating...' : 'Create'}
          </Button>
        </Modal.Footer>
      </Modal>

      {/* Edit Skill Modal */}
      <Modal show={showEditModal} onHide={() => setShowEditModal(false)}>
        <Modal.Header closeButton>
          <Modal.Title>Edit Skill</Modal.Title>
        </Modal.Header>
        <Modal.Body>
          {selectedSkill && (
            <Form>
              <Form.Group className="mb-3">
                <Form.Label>Name</Form.Label>
                <Form.Control
                  type="text"
                  value={selectedSkill.name}
                  onChange={(e) =>
                    setSelectedSkill({ ...selectedSkill, name: e.target.value })
                  }
                  required
                />
              </Form.Group>
              <Form.Group className="mb-3">
                <Form.Label>Description</Form.Label>
                <Form.Control
                  as="textarea"
                  rows={2}
                  value={selectedSkill.description || ''}
                  onChange={(e) =>
                    setSelectedSkill({ ...selectedSkill, description: e.target.value })
                  }
                />
              </Form.Group>
              <Form.Group className="mb-3">
                <Form.Label>Path</Form.Label>
                <Form.Control
                  type="text"
                  value={selectedSkill.path}
                  onChange={(e) =>
                    setSelectedSkill({ ...selectedSkill, path: e.target.value })
                  }
                  required
                />
              </Form.Group>
              <Form.Group className="mb-3">
                <Form.Check
                  type="checkbox"
                  label="Enabled"
                  checked={selectedSkill.enabled}
                  onChange={(e) =>
                    setSelectedSkill({ ...selectedSkill, enabled: e.target.checked })
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
          <Button 
            variant="primary" 
            onClick={handleUpdateSkill} 
            disabled={loading || !selectedSkill?.name || !selectedSkill?.path}
          >
            {loading ? 'Updating...' : 'Update'}
          </Button>
        </Modal.Footer>
      </Modal>
    </div>
  );
}

export default Skills;
