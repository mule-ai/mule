package testserver

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

type MessageRequest struct {
	Message string `json:"message"`
}

type MessageResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func handleMessage(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse the JSON
	var req MessageRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}

	// Log the received message
	log.Printf("Received message: %s", req.Message)

	// Create response
	response := MessageResponse{
		Status:  "success",
		Message: fmt.Sprintf("Message received: %s", req.Message),
	}

	// Send JSON response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Run starts the test server
func Run() {
	http.HandleFunc("/api/messages", handleMessage)

	log.Println("Server starting on :8080")
	log.Println("Send POST requests to http://localhost:8080/api/messages")
	log.Fatal(http.ListenAndServe(":8080", nil))
}