import React, { useState, useEffect } from 'react';
import { Container, Row, Col, Card, Form, Button, Alert, Spinner } from 'react-bootstrap';
import api from '../services/api';
import ToolExecutionConfig from '../components/ToolExecutionConfig';

function Settings() {
  const [settings, setSettings] = useState([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState(null);
  const [messageType, setMessageType] = useState('success');

  useEffect(() => {
    const fetchSettings = async () => {
      try {
        setLoading(true);
        const response = await api.get('/api/v1/settings');
        setSettings(response.data);
      } catch (error) {
        showMessage('Failed to load settings', 'danger');
        console.error('Error fetching settings:', error);
      } finally {
        setLoading(false);
      }
    };

    fetchSettings();
  }, []);

  const handleSettingChange = (key, value) => {
    setSettings(prevSettings =>
      prevSettings.map(setting =>
        setting.key === key ? { ...setting, value } : setting
      )
    );
  };

  const handleSave = async (key) => {
    try {
      setSaving(true);
      const setting = settings.find(s => s.key === key);
      if (!setting) return;

      await api.put(`/api/v1/settings/${key}`, setting);
      showMessage(`Setting "${setting.description || setting.key}" saved successfully`, 'success');
    } catch (error) {
      showMessage(`Failed to save setting: ${error.message}`, 'danger');
      console.error('Error saving setting:', error);
    } finally {
      setSaving(false);
    }
  };

  const showMessage = (text, type = 'success') => {
    setMessage(text);
    setMessageType(type);
    setTimeout(() => setMessage(null), 5000);
  };

  const groupSettingsByCategory = () => {
    const grouped = {};
    settings.forEach(setting => {
      if (!grouped[setting.category]) {
        grouped[setting.category] = [];
      }
      grouped[setting.category].push(setting);
    });
    return grouped;
  };

  const renderSettingInput = (setting) => {
    const isNumeric = setting.key.includes('seconds') || 
                     setting.key.includes('timeout') || 
                     setting.key.includes('port') ||
                     setting.key.includes('count') ||
                     setting.key.includes('size');

    if (isNumeric) {
      return (
        <Form.Control
          type="number"
          value={setting.value}
          onChange={(e) => handleSettingChange(setting.key, e.target.value)}
          min="1"
        />
      );
    }

    // Default to text input
    return (
      <Form.Control
        type="text"
        value={setting.value}
        onChange={(e) => handleSettingChange(setting.key, e.target.value)}
      />
    );
  };

  if (loading) {
    return (
      <Container className="mt-4">
        <Row className="justify-content-center">
          <Col md={6} className="text-center">
            <Spinner animation="border" variant="primary" />
            <p className="mt-2">Loading settings...</p>
          </Col>
        </Row>
      </Container>
    );
  }

  const groupedSettings = groupSettingsByCategory();

  return (
    <Container className="mt-4">
      <Row>
        <Col>
          <h1>Settings</h1>
          <p className="text-muted">Configure application settings</p>
        </Col>
      </Row>

      {message && (
        <Row className="mb-3">
          <Col>
            <Alert variant={messageType} dismissible onClose={() => setMessage(null)}>
              {message}
            </Alert>
          </Col>
        </Row>
      )}

      {Object.entries(groupedSettings).map(([category, categorySettings]) => (
        <Row key={category} className="mb-4">
          <Col>
            <Card>
              <Card.Header>
                <h5 className="mb-0">
                  {category.charAt(0).toUpperCase() + category.slice(1)} Settings
                </h5>
              </Card.Header>
              <Card.Body>
                {categorySettings.map((setting) => (
                  <Row key={setting.key} className="mb-3 align-items-center">
                    <Col md={8}>
                      <Form.Group>
                        <Form.Label>
                          <strong>{setting.description || setting.key}</strong>
                          <div className="text-muted small">
                            Key: <code>{setting.key}</code>
                          </div>
                        </Form.Label>
                        {renderSettingInput(setting)}
                      </Form.Group>
                    </Col>
                    <Col md={4} className="text-end">
                      <Button
                        variant="primary"
                        size="sm"
                        onClick={() => handleSave(setting.key)}
                        disabled={saving}
                      >
                        {saving ? (
                          <>
                            <Spinner
                              as="span"
                              animation="border"
                              size="sm"
                              role="status"
                              aria-hidden="true"
                              className="me-1"
                            />
                            Saving...
                          </>
                        ) : (
                          'Save'
                        )}
                      </Button>
                    </Col>
                  </Row>
                ))}
              </Card.Body>
            </Card>
          </Col>
        </Row>
      ))}

      <Row className="mt-4">
        <Col>
          <ToolExecutionConfig />
        </Col>
      </Row>

      <Row className="mt-4">
        <Col>
          <Card className="bg-light">
            <Card.Body>
              <h6>About Settings</h6>
              <p className="text-muted small mb-0">
                Settings are stored in the database and take effect immediately.
                Some settings may require a page refresh to fully apply.
              </p>
            </Card.Body>
          </Card>
        </Col>
      </Row>
    </Container>
  );
}

export default Settings;