package api

import (
	"context"
	"log"
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
			h.mutex.Unlock()
			log.Printf("WebSocket client connected. Total clients: %d", len(h.clients))

		case client := <-h.unregister:
			h.mutex.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				if closeErr := client.Close(); closeErr != nil {
					// Don't log "use of closed network connection" errors as they're expected
					if closeErr.Error() != "use of closed network connection" {
						log.Printf("Error closing WebSocket client: %v", closeErr)
					}
				}
			}
			h.mutex.Unlock()
			log.Printf("WebSocket client disconnected. Total clients: %d", len(h.clients))

		case message := <-h.broadcast:
			h.mutex.RLock()
			for client := range h.clients {
				select {
				case <-time.After(time.Second * 10):
					// Write timeout, remove client
					delete(h.clients, client)
					if closeErr := client.Close(); closeErr != nil {
						// Don't log "use of closed network connection" errors as they're expected
						if closeErr.Error() != "use of closed network connection" {
							log.Printf("Error closing WebSocket client: %v", closeErr)
						}
					}
				default:
					if err := client.Conn().WriteJSON(message); err != nil {
						log.Printf("Error writing to WebSocket client: %v", err)
						delete(h.clients, client)
						if closeErr := client.Close(); closeErr != nil {
							// Don't log "use of closed network connection" errors as they're expected
							if closeErr.Error() != "use of closed network connection" {
								log.Printf("Error closing WebSocket client: %v", closeErr)
							}
						}
					}
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
	_ = client.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	client.conn.SetPongHandler(func(string) error {
		_ = client.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Send ping every 54 seconds to keep connection alive
	ticker := time.NewTicker(54 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				// Don't log ping errors as they're expected when the connection is closed
				return
			}

		default:
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

// monitorJobs monitors job changes and broadcasts updates
func (s *JobStreamer) monitorJobs() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var lastJobStates = make(map[string]string)

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// Get all jobs for monitoring (no pagination/filtering needed)
			jobs, _, err := s.jobStore.ListJobs(job.ListJobsOptions{})
			if err != nil {
				log.Printf("Error listing jobs for monitoring: %v", err)
				continue
			}

			for _, job := range jobs {
				lastState, exists := lastJobStates[job.ID]
				if !exists || lastState != string(job.Status) {
					// Job status changed, broadcast update
					s.hub.BroadcastJobUpdate(job)
					lastJobStates[job.ID] = string(job.Status)
				}
			}
		}
	}
}
