package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mule-ai/mule/pkg/job"
)

// WebSocketMessage represents a message sent over WebSocket
type WebSocketMessage struct {
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
}

// inboundOp is a client->hub control message (Herd H3.4 per-job
// subscriptions). Clients may send {"op":"subscribe","job_id":"…"},
// {"op":"unsubscribe","job_id":"…"} or {"op":"subscribe_all"}.
type inboundOp struct {
	Op    string `json:"op"`
	JobID string `json:"job_id"`
}

// WebSocketClient wraps a websocket.Conn with exactly-once closure
// semantics plus per-job subscription state (H3.4).
//
// Default (backward-compatible) behavior: a client that has never
// subscribed to anything receives the full firehose. Once a client
// subscribes to at least one job, it receives only updates for those
// jobs (or everything after subscribe_all).
type WebSocketClient struct {
	conn      *websocket.Conn
	closeOnce sync.Once
	closed    chan struct{}

	// H3.4 per-job subscription state.
	mu             sync.RWMutex
	subscribed     map[string]bool
	subscribeAll   bool
	subscribedOnce bool // has the client ever sent a subscribe op?
}

// closeNoise reports whether a close error is an expected consequence of the
// hub closing an already-dead connection.
func closeNoise(err error) bool {
	return errors.Is(err, net.ErrClosed) || errors.Is(err, websocket.ErrCloseSent)
}

// NewWebSocketClient creates a new WebSocket client wrapper
func NewWebSocketClient(conn *websocket.Conn) *WebSocketClient {
	return &WebSocketClient{
		conn:       conn,
		closed:     make(chan struct{}),
		subscribed: make(map[string]bool),
	}
}

// Close closes the WebSocket connection exactly once
func (wsc *WebSocketClient) Close() error {
	var err error
	wsc.closeOnce.Do(func() {
		err = wsc.conn.Close()
		close(wsc.closed)
	})
	return err
}

// Conn returns the underlying websocket connection
func (wsc *WebSocketClient) Conn() *websocket.Conn {
	return wsc.conn
}

// AddSubscription subscribes this client to a job's update stream.
func (wsc *WebSocketClient) AddSubscription(jobID string) {
	wsc.mu.Lock()
	defer wsc.mu.Unlock()
	wsc.subscribed[jobID] = true
	wsc.subscribedOnce = true
}

// RemoveSubscription unsubscribes this client from a job's update stream.
// Once a client has ever subscribed, an empty subscription set means
// "receive nothing" (the firehose default only applies before the
// client's first subscribe).
func (wsc *WebSocketClient) RemoveSubscription(jobID string) {
	wsc.mu.Lock()
	defer wsc.mu.Unlock()
	delete(wsc.subscribed, jobID)
}

// SubscribeAll makes this client receive every job update regardless of
// per-job subscriptions.
func (wsc *WebSocketClient) SubscribeAll() {
	wsc.mu.Lock()
	defer wsc.mu.Unlock()
	wsc.subscribeAll = true
	wsc.subscribedOnce = true
}

// ShouldReceive reports whether this client receives a message carrying
// the given job id (or, when jobOK is false, a message with no job id
// such as an agent event).
func (wsc *WebSocketClient) ShouldReceive(jobID string, jobOK bool) bool {
	wsc.mu.RLock()
	defer wsc.mu.RUnlock()
	if wsc.subscribeAll {
		return true
	}
	if !wsc.subscribedOnce {
		// Never subscribed: full firehose (backward compatible).
		return true
	}
	if !jobOK {
		// Job-filtered client: non-job messages (agent events) are not
		// delivered. subscribe_all restores them.
		return false
	}
	return wsc.subscribed[jobID]
}

// WebSocketHub manages WebSocket connections
type WebSocketHub struct {
	clients    map[*WebSocketClient]bool
	broadcast  chan WebSocketMessage
	register   chan *WebSocketClient
	unregister chan *WebSocketClient
	mutex      sync.RWMutex
}

// NewWebSocketHub creates a new WebSocket hub
func NewWebSocketHub() *WebSocketHub {
	return &WebSocketHub{
		clients:    make(map[*WebSocketClient]bool),
		broadcast:  make(chan WebSocketMessage, 256),
		register:   make(chan *WebSocketClient),
		unregister: make(chan *WebSocketClient),
	}
}

// Run starts the WebSocket hub
func (h *WebSocketHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mutex.Lock()
			h.clients[client] = true
			n := len(h.clients)
			h.mutex.Unlock()
			log.Printf("WebSocket client connected. Total clients: %d", n)

		case client := <-h.unregister:
			h.mutex.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				if closeErr := client.Close(); !closeNoise(closeErr) {
					log.Printf("Error closing WebSocket client: %v", closeErr)
				}
			}
			n := len(h.clients)
			h.mutex.Unlock()
			log.Printf("WebSocket client disconnected. Total clients: %d", n)

		case message := <-h.broadcast:
			h.mutex.RLock()
			jobID, hasJob := jobIDOfMessage(message)
			for client := range h.clients {
				// H3.4 per-job filter (default: clients that never
				// subscribed receive everything).
				if !client.ShouldReceive(jobID, hasJob) {
					continue
				}
				// Bound each write so one stuck client can't stall the
				// broadcast loop for everyone else.
				if err := client.Conn().SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
					delete(h.clients, client)
					_ = client.Close()
					continue
				}
				if err := client.Conn().WriteJSON(message); err != nil {
					log.Printf("Error writing to WebSocket client: %v", err)
					delete(h.clients, client)
					_ = client.Close()
				}
			}
			h.mutex.RUnlock()
		}
	}
}

// jobIDOfMessage extracts the job id from a broadcast message. Job
// updates and job step updates carry it; agent events do not.
func jobIDOfMessage(message WebSocketMessage) (string, bool) {
	switch d := message.Data.(type) {
	case *job.Job:
		if d != nil && d.ID != "" {
			return d.ID, true
		}
		return "", false
	case *job.JobStep:
		if d != nil && d.JobID != "" {
			return d.JobID, true
		}
		return "", false
	}
	return "", false
}

// BroadcastJobUpdate broadcasts a job update to all connected clients
// (filtered per-client by H3.4 subscriptions; JobStreamer's tail
// updates flow through the same filter).
func (h *WebSocketHub) BroadcastJobUpdate(job *job.Job) {
	message := WebSocketMessage{
		Type:      "job_update",
		Data:      job,
		Timestamp: time.Now(),
	}
	select {
	case h.broadcast <- message:
	default:
		// Channel is full, skip this update
	}
}

// BroadcastJobStepUpdate broadcasts a job step update to all connected clients
func (h *WebSocketHub) BroadcastJobStepUpdate(step *job.JobStep) {
	message := WebSocketMessage{
		Type:      "job_step_update",
		Data:      step,
		Timestamp: time.Now(),
	}
	select {
	case h.broadcast <- message:
	default:
		// Channel is full, skip this update
	}
}

// BroadcastAgentEvent broadcasts an agent event to all connected clients
func (h *WebSocketHub) BroadcastAgentEvent(eventType string, data interface{}) {
	message := WebSocketMessage{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now(),
	}
	select {
	case h.broadcast <- message:
	default:
		// Channel is full, skip this update
	}
}

// WebSocketHandler handles WebSocket connections
type WebSocketHandler struct {
	hub      *WebSocketHub
	upgrader websocket.Upgrader
}

// NewWebSocketHandler creates a new WebSocket handler
func NewWebSocketHandler(hub *WebSocketHub) *WebSocketHandler {
	return &WebSocketHandler{
		hub: hub,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// Allow connections from any origin in development
				// In production, implement proper origin checking
				return true
			},
		},
	}
}

// ServeHTTP handles WebSocket upgrade and connection
func (h *WebSocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Don't log upgrade errors as they're often client-side issues
		return
	}

	// Create a new WebSocket client wrapper
	client := NewWebSocketClient(conn)

	// Register the new client
	h.hub.register <- client

	// Start a goroutine to handle this connection
	go h.handleConnection(client)
}

// handleConnection handles a WebSocket connection
func (h *WebSocketHandler) handleConnection(client *WebSocketClient) {
	defer func() {
		h.hub.unregister <- client
	}()

	// Set read deadline and pong handler
	if err := client.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
		log.Printf("Error setting read deadline: %v", err)
		return
	}
	client.conn.SetPongHandler(func(string) error {
		if err := client.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			return err
		}
		return nil
	})

	// Send ping every 54 seconds to keep connection alive
	ticker := time.NewTicker(54 * time.Second)
	defer ticker.Stop()

	// Goroutine to send pings
	go func() {
		for {
			select {
			case <-ticker.C:
				if err := client.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
					log.Printf("Error setting write deadline: %v", err)
					return
				}
				if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					// Don't log ping errors as they're expected when the connection is closed
					return
				}
			case <-client.closed:
				return
			}
		}
	}()

	// Main read loop. Inbound control messages (H3.4) adjust this
	// client's per-job subscription set; anything else is ignored.
	for {
		msgType, raw, err := client.conn.ReadMessage()
		if err != nil {
			// Don't log close errors as they're expected when the connection is closed
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			return
		}
		if msgType != websocket.TextMessage {
			continue
		}
		var op inboundOp
		if err := json.Unmarshal(raw, &op); err != nil {
			continue
		}
		switch op.Op {
		case "subscribe":
			if op.JobID != "" {
				client.AddSubscription(op.JobID)
			}
			if err := client.conn.WriteJSON(map[string]interface{}{"op": "subscribed", "job_id": op.JobID}); err != nil {
				return
			}
		case "unsubscribe":
			if op.JobID != "" {
				client.RemoveSubscription(op.JobID)
			}
			if err := client.conn.WriteJSON(map[string]interface{}{"op": "unsubscribed", "job_id": op.JobID}); err != nil {
				return
			}
		case "subscribe_all":
			client.SubscribeAll()
			if err := client.conn.WriteJSON(map[string]interface{}{"op": "subscribed_all"}); err != nil {
				return
			}
		}
	}
}

// JobStreamer streams job updates in real-time
type JobStreamer struct {
	hub      *WebSocketHub
	jobStore job.JobStore
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewJobStreamer creates a new job streamer
func NewJobStreamer(hub *WebSocketHub, jobStore job.JobStore) *JobStreamer {
	ctx, cancel := context.WithCancel(context.Background())
	return &JobStreamer{
		hub:      hub,
		jobStore: jobStore,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start starts the job streamer
func (s *JobStreamer) Start() {
	go s.monitorJobs()
}

// Stop stops the job streamer
func (s *JobStreamer) Stop() {
	s.cancel()
}

// monitorJobs polls the job list and broadcasts updates for jobs whose
// status changed since the last poll.
func (s *JobStreamer) monitorJobs() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastJobStates := make(map[string]string)

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// Request a wide page so recently created jobs aren't missed
			// between polls, and prune state for jobs that fell off the list.
			jobs, _, err := s.jobStore.ListJobs(job.ListJobsOptions{PageSize: 1000})
			if err != nil {
				log.Printf("Error listing jobs for monitoring: %v", err)
				continue
			}

			seen := make(map[string]bool, len(jobs))
			for _, j := range jobs {
				seen[j.ID] = true
				lastState, exists := lastJobStates[j.ID]
				if !exists || lastState != string(j.Status) {
					s.hub.BroadcastJobUpdate(j)
					lastJobStates[j.ID] = string(j.Status)
				}
			}
			for id := range lastJobStates {
				if !seen[id] {
					delete(lastJobStates, id)
				}
			}
		}
	}
}
