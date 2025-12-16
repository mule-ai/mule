package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"event-service/internal/supabase"
)

// WebhookProcessor handles processing of Supabase events
type WebhookProcessor struct {
	muleAPIURL    string
	muleAPIToken  string
	httpClient    *http.Client
}

// NewWebhookProcessor creates a new webhook processor
func NewWebhookProcessor(muleAPIURL, muleAPIToken string) *WebhookProcessor {
	return &WebhookProcessor{
		muleAPIURL:   muleAPIURL,
		muleAPIToken: muleAPIToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ProcessEvent transforms a Supabase event and sends it to the Mule API
func (wp *WebhookProcessor) ProcessEvent(ctx context.Context, event supabase.Event) error {
	log.Printf("Processing event %s of type %s", event.ID, event.EventType)

	// Transform the event into the format expected by Mule API
	muleRequest, err := wp.transformEvent(event)
	if err != nil {
		return fmt.Errorf("failed to transform event: %w", err)
	}

	// Send to Mule API
	err = wp.sendToMuleAPI(ctx, muleRequest)
	if err != nil {
		return fmt.Errorf("failed to send event to Mule API: %w", err)
	}

	log.Printf("Successfully processed event %s", event.ID)
	return nil
}

// transformEvent converts a Supabase event to Mule API format
func (wp *WebhookProcessor) transformEvent(event supabase.Event) (map[string]interface{}, error) {
	// Parse the payload
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
		return nil, fmt.Errorf("failed to parse event payload: %w", err)
	}

	// Create the Mule API request format
	muleRequest := map[string]interface{}{
		"model": "gpt-4",
		"messages": []map[string]interface{}{
			{
				"role": "system",
				"content": "You are processing a webhook event. Use the event information to determine the appropriate action.",
			},
			{
				"role": "user",
				"content": fmt.Sprintf("Process this %s event: %s", event.EventType, event.Payload),
			},
		},
		"stream": false,
	}

	return muleRequest, nil
}

// sendToMuleAPI sends the transformed event to the Mule API
func (wp *WebhookProcessor) sendToMuleAPI(ctx context.Context, request map[string]interface{}) error {
	// Marshal the request
	jsonData, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create the HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", wp.muleAPIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if wp.muleAPIToken != "" {
		req.Header.Set("Authorization", "Bearer "+wp.muleAPIToken)
	}

	// Send the request
	resp, err := wp.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check the response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Mule API returned status %d", resp.StatusCode)
	}

	log.Printf("Successfully sent event to Mule API, status: %d", resp.StatusCode)
	return nil
}