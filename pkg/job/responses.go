package job

// EnhancedJob extends the base Job struct with additional information for API responses
type EnhancedJob struct {
	*Job
	WorkflowName   string `json:"workflow_name,omitempty"`
	WasmModuleName string `json:"wasm_module_name,omitempty"`
}

// EnhancedJobStep extends the base JobStep struct with additional information for API responses
type EnhancedJobStep struct {
	*JobStep
	AgentName      string `json:"agent_name,omitempty"`
	WasmModuleName string `json:"wasm_module_name,omitempty"`
}
