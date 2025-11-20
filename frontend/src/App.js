import React from 'react';
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

function App() {
  return (
    <Router>
      <div className="App">
        <Navbar bg="dark" variant="dark" expand="lg">
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
                <Nav.Link href="/jobs">Jobs</Nav.Link>
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
            <Route path="/jobs" element={<Jobs />} />
          </Routes>
        </Container>
      </div>
    </Router>
  );
}

export default App;