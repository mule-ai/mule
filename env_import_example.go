package main

// This directive tells the compiler to import this function from the "env" module
//
//go:wasmimport env log_message
func logMessage(message string)

func main() {
	// Call the imported function
	logMessage("Hello from WASM with env import!")
}