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
};

// Job APIs
export const jobsAPI = {
  list: () => api.get('/api/v1/jobs'),
  get: (id) => api.get(`/api/v1/jobs/${id}`),
  getSteps: (id) => api.get(`/api/v1/jobs/${id}/steps`),
  create: (data) => api.post('/api/v1/jobs', data),
};

// WASM Module APIs
export const wasmModulesAPI = {
  list: () => api.get('/api/v1/wasm-modules'),
  get: (id) => api.get(`/api/v1/wasm-modules/${id}`),
  create: (data) => {
    const formData = new FormData();
    Object.keys(data).forEach(key => {
      formData.append(key, data[key]);
    });
    return api.post('/api/v1/wasm-modules', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    });
  },
  update: (id, data) => {
    const formData = new FormData();
    Object.keys(data).forEach(key => {
      formData.append(key, data[key]);
    });
    return api.put(`/api/v1/wasm-modules/${id}`, formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    });
  },
  delete: (id) => api.delete(`/api/v1/wasm-modules/${id}`),
};

// Chat completion API
export const chatAPI = {
  complete: (data) => api.post('/v1/chat/completions', data),
  models: () => api.get('/v1/models'),
};

export default api;