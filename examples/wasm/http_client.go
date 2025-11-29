//go:build ignore

package main

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

func main() {
	// Simple HTTP GET request example
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get("https://httpbin.org/get")
	if err != nil {
		fmt.Printf("Error making request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		return
	}

	fmt.Printf("Response Status: %s\n", resp.Status)
	fmt.Printf("Response Body: %s\n", string(body))
}