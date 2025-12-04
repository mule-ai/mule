import axios from 'axios';

const API_BASE_URL = process.env.REACT_APP_API_URL || '';

const api = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Provider APIs
export const providersAPI = {
  list: () => api.get('/api/v1/providers'),
  get: (id) => api.get(`/api/v1/providers/${id}`),
  create: (data) => api.post('/api/v1/providers', data),
  update: (id, data) => api.put(`/api/v1/providers/${id}`, data),
  delete: (id) => api.delete(`/api/v1/providers/${id}`),
  getModels: (id) => api.get(`/api/v1/providers/${id}/models`),
};

// Tool APIs
export const toolsAPI = {
  list: () => api.get('/api/v1/tools'),
  get: (id) => api.get(`/api/v1/tools/${id}`),
  create: (data) => api.post('/api/v1/tools', data),
  update: (id, data) => api.put(`/api/v1/tools/${id}`, data),
  delete: (id) => api.delete(`/api/v1/tools/${id}`),
};

// Agent APIs
export const agentsAPI = {
  list: () => api.get('/api/v1/agents'),
  get: (id) => api.get(`/api/v1/agents/${id}`),
  create: (data) => api.post('/api/v1/agents', data),
  update: (id, data) => api.put(`/api/v1/agents/${id}`, data),
  delete: (id) => api.delete(`/api/v1/agents/${id}`),
  getTools: (id) => api.get(`/api/v1/agents/${id}/tools`),
  assignTool: (id, toolId) => api.post(`/api/v1/agents/${id}/tools`, { tool_id: toolId }),
  removeTool: (id, toolId) => api.delete(`/api/v1/agents/${id}/tools/${toolId}`),
};

// Workflow APIs
export const workflowsAPI = {
  list: () => api.get('/api/v1/workflows'),
  get: (id) => api.get(`/api/v1/workflows/${id}`),
  create: (data) => api.post('/api/v1/workflows', data),
  update: (id, data) => api.put(`/api/v1/workflows/${id}`, data),
  delete: (id) => api.delete(`/api/v1/workflows/${id}`),
  getSteps: (id) => api.get(`/api/v1/workflows/${id}/steps`),
  createStep: (id, data) => api.post(`/api/v1/workflows/${id}/steps`, data),
  updateStep: (workflowId, stepId, data) => api.put(`/api/v1/workflows/${workflowId}/steps/${stepId}`, data),
  deleteStep: (workflowId, stepId) => api.delete(`/api/v1/workflows/${workflowId}/steps/${stepId}`),
  reorderSteps: (id, stepIds) => api.post(`/api/v1/workflows/${id}/steps/reorder`, { step_ids: stepIds }),
};

// Job APIs
export const jobsAPI = {
  list: () => api.get('/api/v1/jobs'),
  get: (id) => api.get(`/api/v1/jobs/${id}`),
  getSteps: (id) => api.get(`/api/v1/jobs/${id}/steps`),
  create: (data) => api.post('/api/v1/jobs', data),
  cancel: (id) => api.delete(`/api/v1/jobs/${id}`),
};

// WASM Module APIs
export const wasmModulesAPI = {
  list: () => api.get('/api/v1/wasm-modules'),
  get: (id) => api.get(`/api/v1/wasm-modules/${id}`),
  create: (data) => {
    // If data is already a FormData object, use it directly
    if (data instanceof FormData) {
      return api.post('/api/v1/wasm-modules', data, {
        headers: {
          'Content-Type': 'multipart/form-data',
        },
      });
    }

    // Otherwise, create FormData from the object
    const formData = new FormData();
    Object.keys(data).forEach(key => {
      if (data[key] !== null && data[key] !== undefined) {
        formData.append(key, data[key]);
      }
    });
    return api.post('/api/v1/wasm-modules', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    });
  },
  update: (id, data) => {
    // If data is already a FormData object, use it directly
    if (data instanceof FormData) {
      return api.put(`/api/v1/wasm-modules/${id}`, data, {
        headers: {
          'Content-Type': 'multipart/form-data',
        },
      });
    }

    // Otherwise, create FormData from the object
    const formData = new FormData();
    Object.keys(data).forEach(key => {
      if (data[key] !== null && data[key] !== undefined) {
        formData.append(key, data[key]);
      }
    });
    return api.put(`/api/v1/wasm-modules/${id}`, formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    });
  },
  delete: (id) => api.delete(`/api/v1/wasm-modules/${id}`),
  // WASM compilation and testing APIs
  compile: (data) => api.post('/api/v1/wasm-modules/compile', data),
  getSource: (id) => api.get(`/api/v1/wasm-modules/${id}/source`),
  updateSource: (id, data) => api.put(`/api/v1/wasm-modules/${id}/source`, data),
  test: (data) => api.post('/api/v1/wasm-modules/test', data),
  getExampleCode: (language) => api.get(`/api/v1/wasm-modules/example?language=${language}`),
};

// Chat completion API
export const chatAPI = {
  complete: (data) => api.post('/v1/chat/completions', data),
  models: () => api.get('/v1/models'),
};

// WASM Editor chat API
export const wasmEditorChatAPI = {
  chat: (data) => api.post('/v1/chat/completions', {
    ...data,
    // This ensures we're using the chat completions endpoint with proper formatting
  }),
};

// Memory configuration API
export const memoryConfigAPI = {
  get: () => api.get('/api/v1/memory-config'),
  update: (data) => api.put('/api/v1/memory-config', data),
};

export default api;