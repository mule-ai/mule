// Routing regression tests for the react-router v7 migration (#123).
//
// These tests pin the app's route table: every major path must resolve to its
// page component, the navbar must render, unknown paths must not blow up, and
// client-side navigation (popstate) must work. Written because the frontend
// had ZERO unit tests and we merged a routing-library major (v6 -> v7).
//
// API / WebSocket / Monaco are mocked so route assertions never touch network.
import React from 'react';
import { describe, it, expect, beforeAll, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';

// ---- module mocks (paths resolve to the same files the pages import) ----
vi.mock('./services/api', () => {
  const ok = () => Promise.resolve({ data: { data: [], jobs: [], steps: [] } });
  const fn = () => vi.fn(ok);
  return {
    default: {
      get: vi.fn(async (url = '') =>
        url.includes('max_tool_calls')
          ? { data: { value: '5' } }
          : { data: [] }), // /api/v1/settings -> array of settings
      put: fn(),
      post: fn(),
      delete: fn(),
    },
    providersAPI: { list: fn(), get: fn(), create: fn(), update: fn(), delete: fn(), getModels: fn() },
    toolsAPI: { list: fn(), get: fn(), create: fn(), update: fn(), delete: fn() },
    skillsAPI: { list: fn(), get: fn(), create: fn(), update: fn(), delete: fn() },
    agentsAPI: { list: fn(), get: fn(), create: fn(), update: fn(), delete: fn(), getSkills: fn(), assignSkill: fn(), removeSkill: fn(), getTools: fn(), assignTool: fn(), removeTool: fn() },
    workflowsAPI: { list: fn(), get: fn(), create: fn(), update: fn(), delete: fn(), getSteps: fn(), createStep: fn(), updateStep: fn(), deleteStep: fn(), reorderSteps: fn() },
    jobsAPI: { list: fn(), get: fn(), getSteps: fn(), create: fn(), cancel: fn() },
    wasmModulesAPI: { list: fn(), get: fn(), create: fn(), update: fn(), delete: fn(), compile: fn(), getSource: fn(), updateSource: fn(), test: fn(), getExampleCode: fn() },
    chatAPI: { complete: fn(), models: fn() },
    wasmEditorChatAPI: { chat: fn() },
    memoryConfigAPI: { get: fn(), update: fn() },
  };
});

vi.mock('./services/websocket', () => ({
  default: {
    connect: vi.fn(),
    disconnect: vi.fn(),
    subscribe: vi.fn(),
    unsubscribe: vi.fn(),
    publish: vi.fn(),
  },
}));

vi.mock('@monaco-editor/react', () => ({
  default: () => <div data-testid="monaco-editor-stub" />,
}));

import App from './App';
import { chatAPI, jobsAPI } from './services/api';

// jsdom lacks matchMedia, which App.jsx uses for theme detection.
beforeAll(() => {
  window.matchMedia =
    window.matchMedia ||
    ((query) => ({
      matches: false,
      media: query,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    }));
});

// Render App with the browser history positioned at `path` (BrowserRouter
// picks up location from window.history at mount time).
function renderAt(path) {
  window.history.pushState({}, '', path);
  return render(<App />);
}

const ROUTES = [
  ['/', 'Dashboard'],
  ['/workflows', 'Workflow Builder'],
  ['/agents', 'Agents'],
  ['/providers', 'Providers'],
  ['/skills', 'Skills'],
  ['/tools', 'Skills'], // legacy alias: /tools must still render the Skills page
  ['/wasm-modules', 'WASM Modules'],
  ['/wasm-editor', 'WASM Code Editor'],
  ['/jobs', 'Jobs'],
  ['/settings', 'Settings'],
];

describe('route table (react-router v7)', () => {
  for (const [path, heading] of ROUTES) {
    it(`renders the "${heading}" page at ${path}`, async () => {
      renderAt(path);
      const h = await screen.findByRole('heading', { level: 1, name: heading });
      expect(h).toBeInTheDocument();
    });
  }

  it('renders the navbar brand on every route', async () => {
    renderAt('/agents');
    expect(await screen.findByText('Mule')).toBeInTheDocument();
    expect(screen.getByText('Dashboard')).toBeInTheDocument(); // nav link
    expect(screen.getByText('Settings')).toBeInTheDocument(); // nav link
  });

  it('mount-time data fetch still fires on the Dashboard route', async () => {
    renderAt('/');
    await screen.findByRole('heading', { level: 1, name: 'Dashboard' });
    await waitFor(() => expect(chatAPI.models).toHaveBeenCalled());
  });

  it('mount-time jobs fetch fires on the Jobs route', async () => {
    renderAt('/jobs');
    await screen.findByRole('heading', { level: 1, name: 'Jobs' });
    await waitFor(() => expect(jobsAPI.list).toHaveBeenCalled());
  });

  it('renders no page for an unknown route (404 behaviour, no crash)', async () => {
    renderAt('/definitely-not-a-route');
    // Navbar renders...
    expect(await screen.findByText('Mule')).toBeInTheDocument();
    // ...but none of the page headings do.
    expect(screen.queryByRole('heading', { level: 1 })).not.toBeInTheDocument();
  });

  it('re-renders on client-side navigation (popstate)', async () => {
    renderAt('/');
    expect(await screen.findByRole('heading', { level: 1, name: 'Dashboard' })).toBeInTheDocument();

    // Simulate an in-app client-side navigation to /settings.
    window.history.pushState({}, '', '/settings');
    window.dispatchEvent(new PopStateEvent('popstate', { state: {} }));

    expect(await screen.findByRole('heading', { level: 1, name: 'Settings' })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { level: 1, name: 'Dashboard' })).not.toBeInTheDocument();
  });
});
