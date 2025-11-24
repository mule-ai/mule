package wasmcompiler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mule-ai/mule/pkg/database"
)

// Compiler handles WASM compilation from source code
type Compiler struct {
	workDir string
}

// NewCompiler creates a new WASM compiler
func NewCompiler(workDir string) *Compiler {
	// Create the working directory if it doesn't exist
	if err := os.MkdirAll(workDir, 0755); err != nil {
		log.Printf("Warning: failed to create compiler work directory %s: %v", workDir, err)
	}

	return &Compiler{
		workDir: workDir,
	}
}

// CompileRequest represents a compilation request
type CompileRequest struct {
	SourceCode string `json:"source_code"`
	Language   string `json:"language"`
	ModuleName string `json:"module_name"`
}

// CompileResult represents the result of a compilation
type CompileResult struct {
	Success        bool      `json:"success"`
	ModuleData     []byte    `json:"module_data,omitempty"`
	Error          string    `json:"error,omitempty"`
	CompiledAt     time.Time `json:"compiled_at"`
	SourceChecksum string    `json:"source_checksum"`
}

// Compile compiles source code to WASM based on language
func (c *Compiler) Compile(ctx context.Context, req CompileRequest) (*CompileResult, error) {
	result := &CompileResult{
		CompiledAt: time.Now(),
	}

	// Calculate source checksum
	hash := sha256.Sum256([]byte(req.SourceCode))
	result.SourceChecksum = hex.EncodeToString(hash[:])

	switch strings.ToLower(req.Language) {
	case "go":
		return c.compileGo(ctx, req)
	default:
		result.Success = false
		result.Error = fmt.Sprintf("unsupported language: %s", req.Language)
		return result, nil
	}
}

// compileGo compiles Go source code to WASM
func (c *Compiler) compileGo(ctx context.Context, req CompileRequest) (*CompileResult, error) {
	result := &CompileResult{
		CompiledAt: time.Now(),
	}

	// Create temporary directory for compilation
	tmpDir, err := os.MkdirTemp(c.workDir, "wasm-compile-")
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to create temp directory: %v", err)
		return result, nil
	}
	defer os.RemoveAll(tmpDir)

	// Write source code to main.go
	mainFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(mainFile, []byte(req.SourceCode), 0644); err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to write source file: %v", err)
		return result, nil
	}

	// Create go.mod file
	goModContent := fmt.Sprintf(`module %s

go 1.24
`, strings.ToLower(req.ModuleName))
	goModFile := filepath.Join(tmpDir, "go.mod")
	if err := os.WriteFile(goModFile, []byte(goModContent), 0644); err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to write go.mod: %v", err)
		return result, nil
	}

	// Compile to WASM
	wasmFile := filepath.Join(tmpDir, "main.wasm")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", wasmFile, ".")
	cmd.Dir = tmpDir
	cmd.Env = append(os.Environ(),
		"GOOS=wasip1",
		"GOARCH=wasm",
		"CGO_ENABLED=0",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("compilation failed: %v\nOutput: %s", err, string(output))
		return result, nil
	}

	// Read the compiled WASM module
	moduleData, err := os.ReadFile(wasmFile)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to read compiled module: %v", err)
		return result, nil
	}

	result.Success = true
	result.ModuleData = moduleData
	return result, nil
}

// ValidateGoSource performs basic validation on Go source code
func ValidateGoSource(sourceCode string) error {
	// Check for package main
	if !strings.Contains(sourceCode, "package main") {
		return fmt.Errorf("Go WASM modules must have 'package main'")
	}

	// Check for main function
	if !strings.Contains(sourceCode, "func main()") {
		return fmt.Errorf("Go WASM modules must have a 'main' function")
	}

	// Check for fmt import (commonly needed for WASM output)
	if !strings.Contains(sourceCode, "import") || !strings.Contains(sourceCode, "fmt") {
		// This is just a warning, not an error
		// fmt.Println is commonly used for WASM output but not strictly required
		log.Println("Warning: source code may be missing fmt import, which is commonly used for WASM output")
	}

	return nil
}

// GenerateExampleGoCode returns example Go code for WASM modules
func GenerateExampleGoCode() string {
	return `package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// InputData represents the flexible input structure from workflow steps
type InputData struct {
	Prompt string                 ` + "`json:\"prompt\"`" + ` // Main input from previous workflow step
	Message string                 ` + "`json:\"message,omitempty\"`" + ` // Alternative input field (backward compatibility)
	Data    map[string]interface{} ` + "`json:\"data,omitempty\"`" + ` // Additional data
}

// OutputData represents the output structure for the next workflow step
type OutputData struct {
	Result  string                 ` + "`json:\"result\"`" + ` // Main result to pass to next step
	Data    map[string]interface{} ` + "`json:\"data,omitempty\"`" + ` // Additional processed data
	Success bool                   ` + "`json:\"success\"`" + ` // Success flag
}

func main() {
	// Read input from stdin
	decoder := json.NewDecoder(os.Stdin)
	var input InputData

	if err := decoder.Decode(&input); err != nil {
		outputError(fmt.Errorf("failed to decode input: %w", err))
		return
	}

	// Process the input
	result := processInput(input)

	// Output result as JSON
	outputResult(result)
}

func processInput(input InputData) OutputData {
	// Your processing logic here
	// In workflows, the primary input comes as the "prompt" field from the previous step

	var textToProcess string
	if input.Prompt != "" {
		// Typical workflow input - previous step passes a "prompt" field
		textToProcess = input.Prompt
	} else if input.Message != "" {
		// Alternative format - some steps may pass a "message" field
		textToProcess = input.Message
	} else {
		// Fallback - handle empty input gracefully
		textToProcess = "No input provided"
	}

	// Example processing: convert to uppercase
	processedText := fmt.Sprintf("%s (processed by WASM module)", textToProcess)

	return OutputData{
		Result:  processedText,
		Data:    input.Data, // Pass through any additional data
		Success: true,
	}
}

func outputResult(result OutputData) {
	encoder := json.NewEncoder(os.Stdout)
	// Important: Disable HTML escaping to preserve special characters
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(result); err != nil {
		outputError(fmt.Errorf("failed to encode output: %w", err))
	}
}

func outputError(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
`
}

// CreateWasmModuleWithSource creates a new WASM module with source code
func CreateWasmModuleWithSource(ctx context.Context, compiler *Compiler, wasmModuleMgr interface {
	CreateWasmModule(ctx context.Context, name, description string, moduleData []byte) (*database.WasmModule, error)
}, sourceMgr interface {
	CreateSource(ctx context.Context, source *database.WasmModuleSource) error
}, name, description, language, sourceCode string) (*database.WasmModule, *database.WasmModuleSource, error) {

	// First compile the source code
	compileReq := CompileRequest{
		SourceCode: sourceCode,
		Language:   language,
		ModuleName: name,
	}

	result, err := compiler.Compile(ctx, compileReq)
	if err != nil {
		return nil, nil, fmt.Errorf("compilation failed: %w", err)
	}

	// Create the WASM module
	var moduleData []byte
	if result.Success {
		moduleData = result.ModuleData
	} else {
		// Create empty module if compilation failed
		moduleData = []byte{}
	}

	wasmModule, err := wasmModuleMgr.CreateWasmModule(ctx, name, description, moduleData)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create WASM module: %w", err)
	}

	// Create the source record
	source := &database.WasmModuleSource{
		ID:                uuid.New().String(),
		WasmModuleID:      wasmModule.ID,
		Language:          language,
		SourceCode:        sourceCode,
		Version:           1,
		CompilationStatus: getCompilationStatus(result),
	}

	if !result.Success {
		source.CompilationError = &result.Error
	}

	if result.Success {
		source.CompiledAt = &result.CompiledAt
	}

	if err := sourceMgr.CreateSource(ctx, source); err != nil {
		// Don't fail the whole operation if source storage fails
		log.Printf("Warning: failed to store source code: %v", err)
	}

	return wasmModule, source, nil
}

func getCompilationStatus(result *CompileResult) string {
	if result.Success {
		return "success"
	}
	return "failed"
}
