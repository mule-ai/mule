package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/itchyny/gojq"
)

// Input represents the expected input structure
type Input struct {
	Prompt interface{}            `json:"prompt"`
	Query  string                 `json:"query"`
	Data   map[string]interface{} `json:"data,omitempty"`
}

// Output represents the output structure
type Output struct {
	Result interface{} `json:"result"`
}

func main() {
	// Read input from stdin
	inputData, err := io.ReadAll(os.Stdin)
	if err != nil {
		outputError(err)
		return
	}

	// Parse input JSON
	var input Input
	if err := json.Unmarshal(inputData, &input); err != nil {
		outputError(err)
		return
	}

	// Check if we have a query
	queryStr := input.Query
	if queryStr == "" {
		outputError(fmt.Errorf("no query provided"))
		return
	}

	// Parse the jq query
	query, err := gojq.Parse(queryStr)
	if err != nil {
		outputError(err)
		return
	}

	// Handle the JSON data from prompt field
	var jsonData interface{}

	// Check the type of the prompt field
	switch prompt := input.Prompt.(type) {
	case string:
		// If prompt is a string, parse it as JSON
		if prompt != "" {
			if err := json.Unmarshal([]byte(prompt), &jsonData); err != nil {
				outputError(err)
				return
			}
		} else if input.Data != nil {
			jsonData = input.Data
		} else {
			outputError(fmt.Errorf("no JSON data provided"))
			return
		}
	case nil:
		// If prompt is nil, check Data field
		if input.Data != nil {
			jsonData = input.Data
		} else {
			outputError(fmt.Errorf("no JSON data provided"))
			return
		}
	default:
		// If prompt is already a JSON object, use it directly
		jsonData = prompt
	}

	// Execute the query
	var results []interface{}
	iter := query.Run(jsonData)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			outputError(err)
			return
		}
		results = append(results, v)
	}

	// Prepare the result
	var result interface{}
	if len(results) == 1 {
		result = results[0]
	} else {
		result = results
	}

	// Create successful output
	output := Output{
		Result: result,
	}

	// Serialize output to JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		outputError(err)
		return
	}
}

// outputError outputs an error message in the expected format
func outputError(err error) {
	// Simple error output as JSON
	fmt.Printf("{\"error\": \"%s\"}\n", err.Error())
	os.Exit(1)
}
