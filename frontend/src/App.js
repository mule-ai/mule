import React, { useEffect, useState } from 'react';
import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import { Container, Nav, Navbar } from 'react-bootstrap';
import 'bootstrap/dist/css/bootstrap.min.css';
import './App.css';

import Dashboard from './pages/Dashboard';
import WorkflowBuilder from './pages/WorkflowBuilder';
import Agents from './pages/Agents';
import Providers from './pages/Providers';
import Tools from './pages/Tools';
import Jobs from './pages/Jobs';
import WasmModules from './pages/WasmModules';
import WasmCodeEditor from './pages/WasmCodeEditor';
import Settings from './pages/Settings';

function App() {
  const [theme, setTheme] = useState('light');

  useEffect(() => {
    // Detect system theme preference
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    setTheme(mediaQuery.matches ? 'dark' : 'light');

    // Listen for theme changes
    const handleThemeChange = (e) => {
      setTheme(e.matches ? 'dark' : 'light');
    };

    mediaQuery.addEventListener('change', handleThemeChange);
    return () => mediaQuery.removeEventListener('change', handleThemeChange);
  }, []);

  return (
    <Router>
      <div className="App" data-theme={theme}>
        <Navbar bg={theme === 'dark' ? 'dark' : 'light'} variant={theme === 'dark' ? 'dark' : 'light'} expand="lg" className="theme-aware-navbar">
          <Container>
            <Navbar.Brand href="/">Mule v2</Navbar.Brand>
            <Navbar.Toggle aria-controls="basic-navbar-nav" />
            <Navbar.Collapse id="basic-navbar-nav">
              <Nav className="me-auto">
                <Nav.Link href="/">Dashboard</Nav.Link>
                <Nav.Link href="/workflows">Workflows</Nav.Link>
                <Nav.Link href="/agents">Agents</Nav.Link>
                <Nav.Link href="/providers">Providers</Nav.Link>
                <Nav.Link href="/tools">Tools</Nav.Link>
                <Nav.Link href="/wasm-modules">WASM Modules</Nav.Link>
                <Nav.Link href="/wasm-editor">WASM Editor</Nav.Link>
                <Nav.Link href="/jobs">Jobs</Nav.Link>
                <Nav.Link href="/settings">Settings</Nav.Link>
              </Nav>
            </Navbar.Collapse>
          </Container>
        </Navbar>

        <Container className="mt-4">
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/workflows" element={<WorkflowBuilder />} />
            <Route path="/agents" element={<Agents />} />
            <Route path="/providers" element={<Providers />} />
            <Route path="/tools" element={<Tools />} />
            <Route path="/wasm-modules" element={<WasmModules />} />
            <Route path="/wasm-editor" element={<WasmCodeEditor />} />
            <Route path="/jobs" element={<Jobs />} />
            <Route path="/settings" element={<Settings />} />
          </Routes>
        </Container>
      </div>
    </Router>
  );
}

export default App;