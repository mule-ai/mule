//go:build ignore

package main

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

func main() {
	// Test different HTTP methods
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Test GET request
	fmt.Println("Testing GET request...")
	resp, err := client.Get("https://httpbin.org/get")
	if err != nil {
		fmt.Printf("Error making GET request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading GET response: %v\n", err)
		return
	}

	fmt.Printf("GET Response Status: %s\n", resp.Status)
	fmt.Printf("GET Response Body: %s\n\n", string(body))

	// Test POST request
	fmt.Println("Testing POST request...")
	postData := `{"key": "value", "test": "data"}`
	resp, err = client.Post("https://httpbin.org/post", "application/json", 
		http.NoBody) // Using NoBody for simplicity in this example
	if err != nil {
		fmt.Printf("Error making POST request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading POST response: %v\n", err)
		return
	}

	fmt.Printf("POST Response Status: %s\n", resp.Status)
	fmt.Printf("POST Response Body: %s\n", string(body))
}