# WebSocket Connection Cleanup Best Practices in Go

This document outlines best practices for handling WebSocket connections in Go applications, with a focus on proper cleanup to avoid "use of closed network connection" errors and ensure exactly-once semantics.

## 1. Exactly-Once Connection Closure

Use `sync.Once` to guarantee a connection is closed exactly once, even when multiple goroutines might attempt closure:

```go
type RobustWebSocketConn struct {
    conn      *websocket.Conn
    closeOnce sync.Once
    closed    chan struct{}
}

func (rwc *RobustWebSocketConn) Close() error {
    var err error
    rwc.closeOnce.Do(func() {
        err = rwc.conn.Close()
        close(rwc.closed)
    })
    return err
}
```

## 2. Connection State Checking

Always check if a connection is closed before performing operations to prevent "use of closed network connection" errors:

```go
func (rwc *RobustWebSocketConn) WriteMessage(messageType int, data []byte) error {
    // Check if connection is already closed
    select {
    case <-rwc.closed:
        return errors.New("connection already closed")
    default:
    }

    return rwc.conn.WriteMessage(messageType, data)
}
```

## 3. Proper Error Handling

Distinguish between different types of connection errors:

```go
_, _, err := conn.ReadMessage()
if err != nil {
    if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
        log.Printf("Unexpected WebSocket closure: %v", err)
    } else if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
        log.Println("Client closed connection normally")
    } else {
        // Network error or "use of closed network connection"
        log.Printf("Network/WebSocket error: %v", err)
    }
    return
}
```

## 4. Connection Manager Pattern

Use a centralized manager to track and coordinate connection cleanup:

```go
type ConnectionManager struct {
    connections map[*RobustWebSocketConn]bool
    mu          sync.RWMutex
}

func (cm *ConnectionManager) RemoveConnection(conn *RobustWebSocketConn) {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    
    if _, exists := cm.connections[conn]; exists {
        delete(cm.connections, conn)
        // Ensure connection is closed exactly once
        conn.Close()
    }
}
```

## 5. Graceful Shutdown

Notify clients and close connections properly during server shutdown:

```go
func (cm *ConnectionManager) CloseAllConnections() {
    cm.mu.Lock()
    defer cm.mu.Unlock()

    for conn := range cm.connections {
        // Send close message to client
        conn.WriteMessage(websocket.CloseMessage, 
            websocket.FormatCloseMessage(websocket.CloseGoingAway, "Server shutting down"))
        // Close the connection
        conn.Close()
        delete(cm.connections, conn)
    }
}
```

## 6. Heartbeat Mechanism

Implement ping/pong to detect dead connections:

```go
// Set up ping/pong handlers for connection health
conn.SetReadDeadline(time.Now().Add(60 * time.Second))
conn.SetPongHandler(func(string) error {
    conn.SetReadDeadline(time.Now().Add(60 * time.Second))
    return nil
})

// Send periodic pings
ticker := time.NewTicker(54 * time.Second)
defer ticker.Stop()

for {
    select {
    case <-ticker.C:
        if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
            return
        }
    case <-done:
        return
    }
}
```

## 7. Resource Cleanup in Defer Statements

Always ensure cleanup in defer statements:

```go
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }
    
    // Ensure cleanup when function exits
    defer func() {
        conn.Close()
    }()
    
    // ... rest of connection handling
}
```

## 8. Context-Based Cancellation

Use contexts for coordinated shutdown:

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// In connection handling goroutines
select {
case <-ctx.Done():
    // Server is shutting down
    return
case <-otherEvents:
    // Handle events normally
}
```

## Key Benefits of These Patterns

1. **Prevents race conditions** by using proper synchronization primitives
2. **Avoids "use of closed network connection" errors** through state checking
3. **Ensures exactly-once semantics** for connection closure with `sync.Once`
4. **Handles client-initiated closures gracefully** with proper error checking
5. **Enables graceful server shutdown** with client notification
6. **Manages resources efficiently** through proper cleanup

## Implementation Examples

See the following files in this repository for practical implementations:
- `/examples/websocket/robust_websocket.go` - Full chat example with robust connection management
- `/examples/websocket/connection_cleanup_patterns.go` - Focused examples of cleanup patterns