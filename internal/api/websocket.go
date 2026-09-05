package api

import (
	"context"
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

// WebSocketClient wraps a websocket.Conn with exactly-once closure semantics
type WebSocketClient struct {
	conn      *websocket.Conn
	closeOnce sync.Once
	closed    chan struct{}
}

// closeNoise reports whether a close error is an expected consequence of the
// hub closing an already-dead connection.
func closeNoise(err error) bool {
	return errors.Is(err, net.ErrClosed) || errors.Is(err, websocket.ErrCloseSent)
}

// NewWebSocketClient creates a new WebSocket client wrapper
func NewWebSocketClient(conn *websocket.Conn) *WebSocketClient {
	return &WebSocketClient{
		conn:   conn,
		closed: make(chan struct{}),
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
			for client := range h.clients {
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

// BroadcastJobUpdate broadcasts a job update to all connected clients
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

	// Main read loop
	for {
		// Read messages from client (for now, we don't expect any)
		_, _, err := client.conn.ReadMessage()
		if err != nil {
			// Don't log close errors as they're expected when the connection is closed
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			return
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
