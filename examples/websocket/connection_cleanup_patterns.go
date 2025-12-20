// Package websocket_examples demonstrates robust WebSocket connection management
// focusing on proper cleanup patterns to avoid "use of closed network connection" errors.
package websocket_examples

import (
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// RobustWebSocketConn demonstrates proper connection management with exactly-once semantics
type RobustWebSocketConn struct {
	conn      *websocket.Conn
	closeOnce sync.Once
	closed    chan struct{}
}

// NewRobustWebSocketConn creates a new robust WebSocket connection wrapper
func NewRobustWebSocketConn(conn *websocket.Conn) *RobustWebSocketConn {
	return &RobustWebSocketConn{
		conn:   conn,
		closed: make(chan struct{}),
	}
}

// WriteMessage safely writes a message to the WebSocket connection
func (rwc *RobustWebSocketConn) WriteMessage(messageType int, data []byte) error {
	// Check if connection is already closed
	select {
	case <-rwc.closed:
		return errors.New("connection already closed")
	default:
	}

	// Set write deadline to prevent hanging
	if err := rwc.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}

	return rwc.conn.WriteMessage(messageType, data)
}

// ReadMessage safely reads a message from the WebSocket connection
func (rwc *RobustWebSocketConn) ReadMessage() (int, []byte, error) {
	// Check if connection is already closed
	select {
	case <-rwc.closed:
		return 0, nil, errors.New("connection already closed")
	default:
	}

	// Set read deadline to prevent hanging
	if err := rwc.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
		return 0, nil, err
	}

	return rwc.conn.ReadMessage()
}

// Close ensures the connection is closed exactly once
func (rwc *RobustWebSocketConn) Close() error {
	var err error
	rwc.closeOnce.Do(func() {
		err = rwc.conn.Close()
		close(rwc.closed)
	})
	return err
}

// IsClosed checks if the connection has been closed
func (rwc *RobustWebSocketConn) IsClosed() bool {
	select {
	case <-rwc.closed:
		return true
	default:
		return false
	}
}

// Example of a connection manager that handles multiple connections
type ConnectionManager struct {
	connections map[*RobustWebSocketConn]bool
	mu          sync.RWMutex
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		connections: make(map[*RobustWebSocketConn]bool),
	}
}

// AddConnection registers a new connection
func (cm *ConnectionManager) AddConnection(conn *RobustWebSocketConn) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.connections[conn] = true
}

// RemoveConnection unregisters and closes a connection
func (cm *ConnectionManager) RemoveConnection(conn *RobustWebSocketConn) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, exists := cm.connections[conn]; exists {
		delete(cm.connections, conn)
		// Ensure connection is closed exactly once
		if err := conn.Close(); err != nil {
			log.Printf("Error closing connection: %v", err)
		}
	}
}

// Broadcast sends a message to all connections
func (cm *ConnectionManager) Broadcast(message []byte) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	for conn := range cm.connections {
		// Skip closed connections
		if conn.IsClosed() {
			continue
		}
		
		// Try to send message
		if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
			// Handle send error by removing the connection
			log.Printf("Error sending message to client: %v", err)
			// Schedule removal in a separate goroutine to avoid deadlock
			go cm.RemoveConnection(conn)
		}
	}
}

// CloseAll closes all connections
func (cm *ConnectionManager) CloseAll() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for conn := range cm.connections {
		if err := conn.Close(); err != nil {
			log.Printf("Error closing connection: %v", err)
		}
	}
	cm.connections = make(map[*RobustWebSocketConn]bool)
}

// Example WebSocket handler demonstrating proper connection management
func handleWebSocket(w http.ResponseWriter, r *http.Request, connManager *ConnectionManager) {
	// Upgrade HTTP connection to WebSocket
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins in this example
		},
	}
	
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}
	
	// Wrap connection with our robust wrapper
	robustConn := NewRobustWebSocketConn(conn)
	
	// Register connection with manager
	connManager.AddConnection(robustConn)
	
	// Ensure cleanup when function exits
	defer func() {
		connManager.RemoveConnection(robustConn)
	}()
	
	// Set up ping/pong handlers for connection health
	if err := conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
		log.Printf("Error setting read deadline: %v", err)
	}
	conn.SetPongHandler(func(string) error {
		if err := conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			return err
		}
		return nil
	})
	
	// Send periodic pings
	go func() {
		ticker := time.NewTicker(54 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				if robustConn.IsClosed() {
					return
				}
				
				if err := robustConn.WriteMessage(websocket.PingMessage, nil); err != nil {
					log.Printf("Failed to send ping: %v", err)
					return
				}
			case <-robustConn.closed:
				return
			}
		}
	}()
	
	// Main message loop
	for !robustConn.IsClosed() {
		messageType, message, err := robustConn.ReadMessage()
		if err != nil {
			// Handle different types of errors appropriately
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket unexpected close error: %v", err)
			} else if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("WebSocket closed normally: %v", err)
			} else {
				// This could be "use of closed network connection" or other network errors
				log.Printf("WebSocket read error: %v", err)
			}
			break
		}

		// Echo message back to client
		if messageType == websocket.TextMessage {
			if err := robustConn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("Failed to echo message: %v", err)
				break
			}
		}
	}
}

func RunConnectionCleanupExample() {
	// Create connection manager
	connManager := NewConnectionManager()
	defer connManager.CloseAll()

	// Set up HTTP handler
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(w, r, connManager)
	})

	// Simple health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			log.Printf("Error writing health check response: %v", err)
		}
	})

	log.Println("Starting WebSocket server on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}