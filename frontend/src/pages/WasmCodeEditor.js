import React, { useState, useEffect, useCallback } from 'react';
import { Card, Row, Col, Button, Form, Modal, Alert, Tabs, Tab, Badge, Spinner, Dropdown, InputGroup } from 'react-bootstrap';
import { wasmModulesAPI, agentsAPI, wasmEditorChatAPI } from '../services/api';
import Editor from '@monaco-editor/react';

function WasmCodeEditor() {
  const [modules, setModules] = useState([]);
  const [selectedModule, setSelectedModule] = useState(null);
  const [sourceCode, setSourceCode] = useState('');
  const [language, setLanguage] = useState('go');
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [newModule, setNewModule] = useState({
    name: '',
    description: '',
    config: ''
  });
  const [testInput, setTestInput] = useState('');
  const [testOutput, setTestOutput] = useState('');
  const [testError, setTestError] = useState('');
  const [isCompiling, setIsCompiling] = useState(false);
  const [isTesting, setIsTesting] = useState(false);
  const [compilationResult, setCompilationResult] = useState(null);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [activeTab, setActiveTab] = useState('editor');

  // Chat functionality state
  const [availableAgents, setAvailableAgents] = useState([]);
  const [selectedAgent, setSelectedAgent] = useState(null);
  const [chatMessages, setChatMessages] = useState([]);
  const [chatInput, setChatInput] = useState('');
  const [isChatLoading, setIsChatLoading] = useState(false);
  const [showChat, setShowChat] = useState(false);

  const loadExampleCode = useCallback(async () => {
    try {
      const response = await wasmModulesAPI.getExampleCode(language);
      setSourceCode(response.data.example_code);
    } catch (error) {
      console.error('Failed to load example code:', error);
      setSourceCode(`package main

import (
    "encoding/json"
    "fmt"
    "os"
)

// InputData represents the expected input structure
type InputData struct {
    Message string                 \`json:"message"\`
    Data    map[string]interface{} \`json:"data"\`
}

// OutputData represents the output structure
type OutputData struct {
    Result  string                 \`json:"result"\`
    Data    map[string]interface{} \`json:"data"\`
    Success bool                   \`json:"success"\`
}

func main() {
    // Read input from stdin
    decoder := json.NewDecoder(os.Stdin)
    var input InputData

    if err := decoder.Decode(&input); err != nil {
        outputError(err)
        return
    }

    // Process the input
    result := processInput(input)

    // Output result as JSON
    outputResult(result)
}

func processInput(input InputData) OutputData {
    // Your processing logic here
    return OutputData{
        Result:  fmt.Sprintf("Processed: %s", input.Message),
        Data:    input.Data,
        Success: true,
    }
}

func outputResult(result OutputData) {
    encoder := json.NewEncoder(os.Stdout)
    if err := encoder.Encode(result); err != nil {
        outputError(err)
    }
}

func outputError(err error) {
    fmt.Fprintf(os.Stderr, "Error: %v\\n", err)
}`);
    }
  }, [language]);

  const loadModuleSource = useCallback(async (moduleId) => {
    try {
      const response = await wasmModulesAPI.getSource(moduleId);
      setSourceCode(response.data.source_code || '');
      setLanguage(response.data.language || 'go');
      setCompilationResult({
        status: response.data.compilation_status,
        error: response.data.compilation_error,
        compiledAt: response.data.compiled_at
      });
    } catch (error) {
      console.error('Failed to load module source:', error);
      // If no source exists, load example code
      loadExampleCode();
    }
  }, [loadExampleCode]);

  useEffect(() => {
    loadModules();
  }, []);

  useEffect(() => {
    if (selectedModule) {
      loadModuleSource(selectedModule.id);
    }
  }, [selectedModule, loadModuleSource]);

  const loadModules = async () => {
    try {
      const response = await wasmModulesAPI.list();
      setModules(response.data.data || []);
    } catch (error) {
      console.error('Failed to load WASM modules:', error);
      setError('Failed to load WASM modules');
    }
  };

  const handleCreateModule = async () => {
    setIsCompiling(true);
    setError('');
    setSuccess('');

    // Validate config if provided
    if (newModule.config) {
      try {
        JSON.parse(newModule.config);
      } catch (e) {
        setError('Configuration must be valid JSON');
        setIsCompiling(false);
        return;
      }
    }

    try {
      const response = await wasmModulesAPI.compile({
        name: newModule.name,
        description: newModule.description,
        language: language,
        source_code: sourceCode,
        config: newModule.config
      });

      setCompilationResult({
        status: response.data.compilation_status,
        error: response.data.compilation_error,
        compiledAt: response.data.compiled_at
      });

      if (response.data.compilation_status === 'success') {
        setSuccess('Module compiled and created successfully!');
        setShowCreateModal(false);
        setNewModule({ name: '', description: '', config: '' });
        loadModules();
        setSelectedModule({
          id: response.data.module_id,
          name: newModule.name,
          description: newModule.description
        });
      } else {
        setError(`Compilation failed: ${response.data.compilation_error}`);
      }
    } catch (error) {
      setError(error.response?.data?.error || 'Failed to compile module');
    } finally {
      setIsCompiling(false);
    }
  };

  const handleUpdateModule = async () => {
    if (!selectedModule) return;

    setIsCompiling(true);
    setError('');
    setSuccess('');

    // Validate config if provided
    if (selectedModule.config) {
      try {
        JSON.parse(selectedModule.config);
      } catch (e) {
        setError('Configuration must be valid JSON');
        setIsCompiling(false);
        return;
      }
    }

    try {
      const response = await wasmModulesAPI.updateSource(selectedModule.id, {
        name: selectedModule.name,
        description: selectedModule.description,
        language: language,
        source_code: sourceCode,
        config: selectedModule.config
      });

      setCompilationResult({
        status: response.data.compilation_status,
        error: response.data.compilation_error,
        compiledAt: response.data.compiled_at
      });

      if (response.data.compilation_status === 'success') {
        setSuccess('Module updated and compiled successfully!');
        loadModules();
      } else {
        setError(`Compilation failed: ${response.data.compilation_error}`);
      }
    } catch (error) {
      setError(error.response?.data?.error || 'Failed to update module');
    } finally {
      setIsCompiling(false);
    }
  };

  const handleTestModule = async () => {
    if (!selectedModule) {
      setError('Please select or create a module first');
      return;
    }

    setIsTesting(true);
    setTestError('');
    setTestOutput('');

    try {
      let inputData;
      try {
        inputData = JSON.parse(testInput);
      } catch (e) {
        setError('Invalid JSON input');
        return;
      }

      const response = await wasmModulesAPI.test({
        module_id: selectedModule.id,
        input: inputData
      });

      if (response.data.success) {
        setTestOutput(JSON.stringify(response.data.output, null, 2));
      } else {
        setTestError(response.data.error || 'Test failed');
      }
    } catch (error) {
      setTestError(error.response?.data?.error || 'Test failed');
    } finally {
      setIsTesting(false);
    }
  };

  const handleSelectModule = async (module) => {
    // Load the full module details to get the config
    try {
      const response = await wasmModulesAPI.get(module.id);
      const fullModule = response.data;

      setSelectedModule({
        ...module,
        config: fullModule.config ? JSON.stringify(JSON.parse(new TextDecoder().decode(fullModule.config)), null, 2) : ''
      });
    } catch (error) {
      console.error('Failed to load module details:', error);
      setSelectedModule(module);
    }
    setActiveTab('editor');
  };

  const handleCreateNew = () => {
    setSelectedModule(null);
    setSourceCode('');
    setCompilationResult(null);
    setShowCreateModal(true);
    loadExampleCode();
  };

  // Chat functionality functions
  const loadAgents = async () => {
    try {
      const response = await agentsAPI.list();
      const agents = response.data || [];
      setAvailableAgents(agents);

      // Find WASM Editor agent as default
      const wasmEditor = agents.find(a => a.name === 'WASM Editor');
      if (wasmEditor) {
        setSelectedAgent(wasmEditor);
      } else if (agents.length > 0) {
        setSelectedAgent(agents[0]);
      }
    } catch (error) {
      console.error('Failed to load agents:', error);
    }
  };

  const handleChatSubmit = async () => {
    if (!chatInput.trim() || !selectedAgent) return;

    const userMessage = {
      role: 'user',
      content: chatInput,
      timestamp: new Date()
    };

    setChatMessages(prev => [...prev, userMessage]);
    setChatInput('');
    setIsChatLoading(true);

    try {
      // Prepare the context with current code
      const codeContext = `Current WASM Module Code (${language}):
\`\`\`
${sourceCode}
\`\`\`

User Question: ${chatInput}`;

      const response = await wasmEditorChatAPI.chat({
        model: `agent/${selectedAgent.name.toLowerCase()}`,
        messages: [
          ...(chatMessages.map(msg => ({
            role: msg.role,
            content: msg.content
          }))),
          { role: 'user', content: codeContext }
        ],
        stream: false
      });

      const assistantMessage = {
        role: 'assistant',
        content: response.data.choices[0].message.content,
        timestamp: new Date()
      };

      setChatMessages(prev => [...prev, assistantMessage]);
    } catch (error) {
      console.error('Chat error:', error);
      const errorMessage = {
        role: 'assistant',
        content: 'Sorry, I encountered an error. Please try again.',
        timestamp: new Date(),
        isError: true
      };
      setChatMessages(prev => [...prev, errorMessage]);
    } finally {
      setIsChatLoading(false);
    }
  };

  const handleInsertCode = (code) => {
    setSourceCode(code);
    setSuccess('Code inserted from chat!');
  };

  const toggleChat = () => {
    setShowChat(!showChat);
    if (!showChat && availableAgents.length === 0) {
      loadAgents();
    }
  };

  const formatChatMessage = (content) => {
    // Simple markdown-like formatting for code blocks
    const parts = content.split(/```([\s\S]*?)```/g);
    return parts.map((part, index) => {
      if (index % 2 === 1) {
        // This is a code block
        return (
          <div key={index} className="position-relative">
            <pre className="bg-dark text-light p-3 rounded mt-2 mb-2" style={{ whiteSpace: 'pre-wrap', fontSize: '12px' }}>
              {part}
            </pre>
            <Button
              size="sm"
              variant="outline-light"
              className="position-absolute"
              style={{ top: '10px', right: '10px', fontSize: '11px' }}
              onClick={() => handleInsertCode(part)}
            >
              Insert Code
            </Button>
          </div>
        );
      }
      return <span key={index}>{part}</span>;
    });
  };

  const getCompilationBadge = () => {
    if (!compilationResult) return null;
    
    const status = compilationResult.status;
    if (status === 'success') {
      return <Badge bg="success">Compiled Successfully</Badge>;
    } else if (status === 'failed') {
      return <Badge bg="danger">Compilation Failed</Badge>;
    } else if (status === 'compiling') {
      return <Badge bg="warning">Compiling...</Badge>;
    }
    return null;
  };

  return (
    <div>
      <div className="d-flex justify-content-between align-items-center mb-4">
        <h1>WASM Code Editor</h1>
        <div className="d-flex gap-2">
          <Button variant="outline-info" onClick={toggleChat}>
            {showChat ? 'Hide' : 'Show'} AI Assistant
          </Button>
          <Button variant="outline-primary" onClick={handleCreateNew}>
            Create New Module
          </Button>
          {selectedModule && (
            <Button variant="primary" onClick={handleUpdateModule} disabled={isCompiling}>
              {isCompiling ? 'Compiling...' : 'Compile & Save'}
            </Button>
          )}
        </div>
      </div>

      {error && <Alert variant="danger" dismissible onClose={() => setError('')}>{error}</Alert>}
      {success && <Alert variant="success" dismissible onClose={() => setSuccess('')}>{success}</Alert>}

      <Row>
        {showChat && (
          <Col md={3} className="d-flex flex-column" style={{ maxHeight: 'calc(100vh - 200px)' }}>
            <Card className="mb-4 flex-grow-1 d-flex flex-column">
              <Card.Header>
                <div className="d-flex justify-content-between align-items-center">
                  <Card.Title className="mb-0">AI Assistant</Card.Title>
                  <Dropdown>
                    <Dropdown.Toggle variant="outline-secondary" size="sm">
                      {selectedAgent?.name || 'Select Agent'}
                    </Dropdown.Toggle>
                    <Dropdown.Menu>
                      {availableAgents.map((agent) => (
                        <Dropdown.Item key={agent.id} onClick={() => setSelectedAgent(agent)}>
                          {agent.name}
                        </Dropdown.Item>
                      ))}
                    </Dropdown.Menu>
                  </Dropdown>
                </div>
              </Card.Header>
              <Card.Body className="flex-grow-1 d-flex flex-column" style={{ overflow: 'hidden' }}>
                <div className="flex-grow-1 mb-3" style={{ overflowY: 'auto', maxHeight: '400px' }}>
                  {chatMessages.length === 0 ? (
                    <p className="text-muted small">
                      Ask me anything about your WASM module! I can help you write code, debug errors, and optimize your module.
                    </p>
                  ) : (
                    chatMessages.map((msg, index) => (
                      <div key={index} className={`mb-3 ${msg.role === 'user' ? 'text-end' : ''}`}>
                        <div className={`d-inline-block p-2 rounded ${msg.role === 'user' ? 'bg-primary text-white' : msg.isError ? 'bg-danger text-white' : 'bg-light'}`}>
                          {formatChatMessage(msg.content)}
                        </div>
                        <div className="small text-muted mt-1">
                          {msg.role === 'user' ? 'You' : selectedAgent?.name} • {msg.timestamp.toLocaleTimeString()}
                        </div>
                      </div>
                    ))
                  )}
                  {isChatLoading && (
                    <div className="text-center">
                      <Spinner animation="border" size="sm" />
                      <span className="ms-2 text-muted">Thinking...</span>
                    </div>
                  )}
                </div>
                <InputGroup>
                  <Form.Control
                    as="textarea"
                    rows={2}
                    value={chatInput}
                    onChange={(e) => setChatInput(e.target.value)}
                    onKeyPress={(e) => {
                      if (e.key === 'Enter' && !e.shiftKey) {
                        e.preventDefault();
                        handleChatSubmit();
                      }
                    }}
                    placeholder="Ask about your WASM module..."
                    disabled={isChatLoading}
                  />
                  <Button variant="primary" onClick={handleChatSubmit} disabled={isChatLoading || !chatInput.trim()}>
                    Send
                  </Button>
                </InputGroup>
              </Card.Body>
            </Card>
          </Col>
        )}

        <Col md={showChat ? 3 : 3}>
          <Card className="mb-4">
            <Card.Header>
              <Card.Title className="mb-0">WASM Modules</Card.Title>
            </Card.Header>
            <Card.Body>
              <div className="d-grid gap-2">
                {modules.map((module) => (
                  <Button
                    key={module.id}
                    variant={selectedModule?.id === module.id ? "primary" : "outline-primary"}
                    size="sm"
                    className="text-start"
                    onClick={() => handleSelectModule(module)}
                  >
                    {module.name}
                  </Button>
                ))}
              </div>
              {modules.length === 0 && (
                <p className="text-muted text-center mt-3">No modules found</p>
              )}
            </Card.Body>
          </Card>

          <Card>
            <Card.Header>
              <Card.Title className="mb-0">Quick Info</Card.Title>
            </Card.Header>
            <Card.Body>
              <div className="mb-3">
                <strong>Language:</strong> 
                <Badge bg="info" className="ms-2">{language.toUpperCase()}</Badge>
              </div>
              <div className="mb-3">
                <strong>Status:</strong>
                <div className="mt-2">
                  {getCompilationBadge()}
                </div>
              </div>
              {compilationResult?.compiledAt && (
                <div className="text-muted small">
                  <strong>Last Compiled:</strong><br/>
                  {new Date(compilationResult.compiledAt).toLocaleString()}
                </div>
              )}
            </Card.Body>
          </Card>
        </Col>

        <Col md={showChat ? 6 : 9}>
          <Tabs activeKey={activeTab} onSelect={(k) => setActiveTab(k)} className="mb-4">
            <Tab eventKey="editor" title="Code Editor">
              <Card>
                <Card.Header>
                  <div className="d-flex justify-content-between align-items-center">
                    <Card.Title className="mb-0">
                      {selectedModule ? `Editing: ${selectedModule.name}` : 'Create New Module'}
                    </Card.Title>
                    <Button
                      variant="outline-secondary"
                      size="sm"
                      onClick={loadExampleCode}
                    >
                      Load Example
                    </Button>
                  </div>
                </Card.Header>
                <Card.Body style={{ height: '500px' }}>
                  <Editor
                    height="100%"
                    language={language}
                    theme="vs-dark"
                    value={sourceCode}
                    onChange={(value) => setSourceCode(value)}
                    options={{
                      minimap: { enabled: false },
                      fontSize: 14,
                      scrollBeyondLastLine: false,
                      automaticLayout: true,
                    }}
                  />
                </Card.Body>
              </Card>

              {compilationResult?.error && (
                <Card className="mt-3 border-danger">
                  <Card.Header className="bg-danger text-white">
                    <Card.Title className="mb-0">Compilation Error</Card.Title>
                  </Card.Header>
                  <Card.Body>
                    <pre className="mb-0 text-danger">{compilationResult.error}</pre>
                  </Card.Body>
                </Card>
              )}
            </Tab>

            <Tab eventKey="config" title="Configuration">
              <Card>
                <Card.Header>
                  <Card.Title className="mb-0">Module Configuration</Card.Title>
                </Card.Header>
                <Card.Body>
                  <p className="text-muted">
                    Configure static settings for your WASM module. This configuration will be merged with
                    input data when the module executes in a workflow.
                  </p>

                  {selectedModule ? (
                    <Form.Group>
                      <Form.Label>Configuration (JSON)</Form.Label>
                      <Form.Control
                        as="textarea"
                        rows={10}
                        value={selectedModule.config || ''}
                        onChange={(e) => setSelectedModule({...selectedModule, config: e.target.value})}
                        placeholder={`{
  "api_key": "your-api-key",
  "endpoint": "https://api.example.com",
  "timeout": 30
}`}
                      />
                      <Form.Text className="text-muted">
                        JSON configuration that will be merged with input data when the module executes.
                        This is useful for API keys, endpoints, and other static configuration values.
                      </Form.Text>
                    </Form.Group>
                  ) : (
                    <div className="text-center py-5">
                      <p className="text-muted">Create or select a module to configure it</p>
                    </div>
                  )}
                </Card.Body>
              </Card>
            </Tab>

            <Tab eventKey="test" title="Test Module">
              <Row>
                <Col md={6}>
                  <Card>
                    <Card.Header>
                      <Card.Title className="mb-0">Test Input</Card.Title>
                    </Card.Header>
                    <Card.Body>
                      <Form.Group>
                        <Form.Label>JSON Input</Form.Label>
                        <Form.Control
                          as="textarea"
                          rows={10}
                          value={testInput}
                          onChange={(e) => setTestInput(e.target.value)}
                          placeholder={`Enter JSON input simulating workflow input, e.g.:\n{\n  "prompt": "Text from previous workflow step"\n}\n\nOr with additional data:\n{\n  "prompt": "Process this text",\n  "data": {\n    "key": "value",\n    "count": 42\n  }\n}`}
                        />
                      </Form.Group>
                      <Button
                        variant="primary"
                        onClick={handleTestModule}
                        disabled={isTesting || !selectedModule}
                        className="mt-3"
                      >
                        {isTesting ? (
                          <>
                            <Spinner animation="border" size="sm" className="me-2" />
                            Testing...
                          </>
                        ) : (
                          'Run Test'
                        )}
                      </Button>
                    </Card.Body>
                  </Card>
                </Col>

                <Col md={6}>
                  <Card>
                    <Card.Header>
                      <Card.Title className="mb-0">Test Output</Card.Title>
                    </Card.Header>
                    <Card.Body>
                      {testOutput ? (
                        <pre className="mb-0" style={{ whiteSpace: 'pre-wrap' }}>
                          {testOutput}
                        </pre>
                      ) : (
                        <p className="text-muted mb-0">
                          {testError || 'Run a test to see output here'}
                        </p>
                      )}
                    </Card.Body>
                  </Card>
                </Col>
              </Row>
            </Tab>

            <Tab eventKey="docs" title="Documentation">
              <Card>
                <Card.Header>
                  <Card.Title className="mb-0">WASM Module Documentation</Card.Title>
                </Card.Header>
                <Card.Body>
                  <h5>Input/Output Structure</h5>
                  <p>
                    WASM modules in Mule receive input via <code>stdin</code> as JSON and should output
                    results via <code>stdout</code> as JSON. When used in workflows, the input is the output
                    from the previous step.
                  </p>

                  <h6 className="mt-3">Typical Input Format (from previous step):</h6>
                  <pre>{`{
  "prompt": "string - content from previous step (agent output or workflow input)"
}`}</pre>
                  <p className="text-muted small">
                    Note: The input structure depends on what the previous step outputs.
                    When testing in the editor, you can simulate this format.
                  </p>

                  <h6 className="mt-3">Output Format (for next step):</h6>
                  <pre>{`{
  "result": "string - processing result",
  "data": {} // processed data (optional)
  "success": true/false
}`}</pre>
                  <p className="text-muted small">
                    The output should be a JSON object. The workflow engine will pass the "output" field
                    to the next step as the "prompt" field.
                  </p>

                  <h5 className="mt-4">Go WASM Specifics</h5>
                  <ul>
                    <li>Package must be <code>main</code></li>
                    <li>Must have a <code>main()</code> function</li>
                    <li>Use <code>encoding/json</code> for JSON parsing</li>
                    <li>Read from <code>os.Stdin</code>, write to <code>os.Stdout</code></li>
                    <li>Errors should be written to <code>os.Stderr</code></li>
                    <li>Be flexible with input structure - previous steps may output different formats</li>
                  </ul>

                  <h5 className="mt-4">Configuration</h5>
                  <p>
                    You can define static configuration for your WASM modules in the Configuration tab.
                    This configuration will be merged with the input data when the module executes.
                    For example, if your configuration is:
                  </p>
                  <pre>{`{
  "api_key": "secret123",
  "endpoint": "https://api.example.com"
}`}</pre>
                  <p>
                    And your input data is:
                  </p>
                  <pre>{`{
  "prompt": "Process this text"
}`}</pre>
                  <p>
                    Your WASM module will receive:
                  </p>
                  <pre>{`{
  "prompt": "Process this text",
  "api_key": "secret123",
  "endpoint": "https://api.example.com"
}`}</pre>

                  <div className="alert alert-info mt-4">
                    <strong>Workflow Integration:</strong> In a workflow, the output from each step becomes
                    the input to the next step. WASM modules should expect a <code>prompt</code> field containing
                    text from the previous step, and should output JSON that the next step can consume.
                  </div>

                  <div className="alert alert-warning mt-3">
                    <strong>Testing Tip:</strong> Use the Test tab to verify your module handles the expected
                    input format. Test with inputs like <code>{'{"prompt": "your test text"}'}</code> to simulate
                    real workflow conditions.
                  </div>
                </Card.Body>
              </Card>
            </Tab>
          </Tabs>
        </Col>
      </Row>

      {/* Create Module Modal */}
      <Modal show={showCreateModal} onHide={() => setShowCreateModal(false)} size="lg">
        <Modal.Header closeButton>
          <Modal.Title>Create New WASM Module</Modal.Title>
        </Modal.Header>
        <Modal.Body>
          <Form>
            <Form.Group className="mb-3">
              <Form.Label>Module Name</Form.Label>
              <Form.Control
                type="text"
                value={newModule.name}
                onChange={(e) => setNewModule({ ...newModule, name: e.target.value })}
                placeholder="e.g., data-processor, text-analyzer"
                required
              />
            </Form.Group>
            <Form.Group className="mb-3">
              <Form.Label>Description</Form.Label>
              <Form.Control
                as="textarea"
                rows={2}
                value={newModule.description}
                onChange={(e) => setNewModule({ ...newModule, description: e.target.value })}
                placeholder="Describe what this module does..."
              />
            </Form.Group>
            <Form.Group className="mb-3">
              <Form.Label>Configuration (JSON, optional)</Form.Label>
              <Form.Control
                as="textarea"
                rows={4}
                value={newModule.config}
                onChange={(e) => setNewModule({ ...newModule, config: e.target.value })}
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
          </Form>
        </Modal.Body>
        <Modal.Footer>
          <Button variant="secondary" onClick={() => setShowCreateModal(false)}>
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={handleCreateModule}
            disabled={isCompiling || !newModule.name || !sourceCode}
          >
            {isCompiling ? (
              <>
                <Spinner animation="border" size="sm" className="me-2" />
                Compiling...
              </>
            ) : (
              'Create & Compile'
            )}
          </Button>
        </Modal.Footer>
      </Modal>
    </div>
  );
}

export default WasmCodeEditor;