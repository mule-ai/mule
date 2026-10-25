package api

// websocket_test.go: H3.4 per-job WebSocket subscription tests.
//
// One client subscribes to job A, one to job B, one sends
// subscribe_all, one never subscribes. Updates to A must reach only
// the A client, the subscribe-all client, and the never-subscribed
// client; updates to B only at the B/all/default trio. Unsubscribe
// stops delivery; JobStreamer's tail updates flow through the same
// BroadcastJobUpdate/BroadcastJobStepUpdate path and inherit the filter.
//
// NOTE on test design: a timed-out WebSocket read permanently poisons
// a gorilla connection (its NextReader caches the first read error), so
// every NEGATIVE assertion (expectNoUpdate) runs on a dedicated conn
// that is never read again. Positive assertions can share a conn.

import (
	"encoding/json"
	"errors"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mule-ai/mule/pkg/job"
)

const noUpdateWindow = 400 * time.Millisecond

// startWSHub starts a hub + handler + test server.
func startWSHub(t *testing.T) (*WebSocketHub, *httptest.Server) {
	t.Helper()
	hub := NewWebSocketHub()
	go hub.Run()
	srv := httptest.NewServer(NewWebSocketHandler(hub))
	t.Cleanup(srv.Close)
	return hub, srv
}

func wsURLFor(serverURL string) string {
	return "ws" + strings.TrimPrefix(serverURL, "http") + "/ws"
}

func dialWS(t *testing.T, serverURL string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(wsURLFor(serverURL), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// sendOp sends an inbound control op and waits for the server's ack.
func sendOp(t *testing.T, c *websocket.Conn, op string, jobID string, wantAck string) {
	t.Helper()
	payload := map[string]string{"op": op}
	if jobID != "" {
		payload["job_id"] = jobID
	}
	require.NoError(t, c.WriteJSON(payload))
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, raw, err := c.ReadMessage()
	require.NoError(t, err, "read ack for %s", op)
	var m struct {
		Op string `json:"op"`
	}
	require.NoError(t, json.Unmarshal(raw, &m))
	assert.Equal(t, wantAck, m.Op)
}

// expectUpdate waits for the next job_update frame and checks its job id.
func expectUpdate(t *testing.T, c *websocket.Conn, wantJobID string) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, raw, err := c.ReadMessage()
	require.NoError(t, err, "expected a job_update frame")
	var m WebSocketMessage
	require.NoError(t, json.Unmarshal(raw, &m))
	assert.Equal(t, "job_update", m.Type)
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(dataJSON(m.Data), &data))
	assert.Equal(t, wantJobID, data["id"])
}

// expectNoUpdate asserts that no frame arrives within the window.
// The connection is poisoned afterward and must not be reused.
func expectNoUpdate(t *testing.T, c *websocket.Conn, label string) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(noUpdateWindow))
	_, _, err := c.ReadMessage()
	assert.Error(t, err, "%s: expected no frame within %v", label, noUpdateWindow)
}

// dataJSON re-marshals a frame's payload for inspection.
func dataJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("null")
	}
	return b
}

// TestWebSocketPerJobSubscriptions is the H3.4 acceptance matrix.
func TestWebSocketPerJobSubscriptions(t *testing.T) {
	hub, srv := startWSHub(t)

	// Long-lived conns used only for positive reads.
	defConn := dialWS(t, srv.URL) // default: never subscribes
	aConn := dialWS(t, srv.URL)
	bConn := dialWS(t, srv.URL)
	allConn := dialWS(t, srv.URL)
	sendOp(t, aConn, "subscribe", "job-A", "subscribed")
	sendOp(t, bConn, "subscribe", "job-B", "subscribed")
	sendOp(t, allConn, "subscribe_all", "", "subscribed_all")

	// expectNoUpdateOnFiltered dials a throwaway client with the given
	// subscription and asserts it does NOT receive the next broadcast.
	expectNoUpdateOnFiltered := func(subscribeOp string, subscribeID, label string) {
		c := dialWS(t, srv.URL)
		if subscribeOp == "subscribe_all" {
			sendOp(t, c, "subscribe_all", "", "subscribed_all")
		} else {
			sendOp(t, c, "subscribe", subscribeID, "subscribed")
		}
		expectNoUpdate(t, c, label)
	}

	jobA := &job.Job{ID: "job-A", Status: job.StatusRunning}
	jobB := &job.Job{ID: "job-B", Status: job.StatusRunning}

	// Update to A: the A client, the all client, and the default
	// client get it; a B-only client does not.
	hub.BroadcastJobUpdate(jobA)
	expectUpdate(t, aConn, "job-A")
	expectUpdate(t, allConn, "job-A")
	expectUpdate(t, defConn, "job-A")
	expectNoUpdateOnFiltered("subscribe", "job-B", "B-only client must not see A")

	// Update to B: symmetric.
	hub.BroadcastJobUpdate(jobB)
	expectUpdate(t, bConn, "job-B")
	expectUpdate(t, allConn, "job-B")
	expectUpdate(t, defConn, "job-B")
	expectNoUpdateOnFiltered("subscribe", "job-A", "A-only client must not see B")

	// Unsubscribe stops delivery: a client that subscribed to A then
	// unsubscribed gets nothing for A, while the default still fires.
	uConn := dialWS(t, srv.URL)
	sendOp(t, uConn, "subscribe", "job-A", "subscribed")
	sendOp(t, uConn, "unsubscribe", "job-A", "unsubscribed")
	hub.BroadcastJobUpdate(jobA)
	expectUpdate(t, defConn, "job-A")
	expectNoUpdate(t, uConn, "unsubscribed client must not see A")

	// subscribe_all restores the firehose for a filtered client.
	saConn := dialWS(t, srv.URL)
	sendOp(t, saConn, "subscribe", "job-A", "subscribed")
	sendOp(t, saConn, "unsubscribe", "job-A", "unsubscribed")
	sendOp(t, saConn, "subscribe_all", "", "subscribed_all")
	hub.BroadcastJobUpdate(jobA)
	expectUpdate(t, saConn, "job-A")
}

// TestWebSocketStepUpdatesFollowJobFilter verifies job step updates
// (the JobStreamer tail path) are filtered by the same per-job rules.
func TestWebSocketStepUpdatesFollowJobFilter(t *testing.T) {
	hub, srv := startWSHub(t)

	aConn := dialWS(t, srv.URL)
	defConn := dialWS(t, srv.URL)
	sendOp(t, aConn, "subscribe", "job-A", "subscribed")

	// A client filtered to job-A must not see job-B step updates.
	negConn := dialWS(t, srv.URL)
	sendOp(t, negConn, "subscribe", "job-A", "subscribed")
	hub.BroadcastJobStepUpdate(&job.JobStep{ID: "step-b", JobID: "job-B", Status: "running"})
	expectNoUpdate(t, negConn, "job-A-filtered client must not see job-B step")

	// A job-A step update reaches the job-A client and the default. The
	// default (firehose) client also buffered the job-B step above, so
	// read that first, then the job-A one.
	hub.BroadcastJobStepUpdate(&job.JobStep{ID: "step-a", JobID: "job-A", Status: "running"})
	expectUpdateForStep(t, aConn, "job-A")
	expectUpdateForStep(t, defConn, "job-B")
	expectUpdateForStep(t, defConn, "job-A")
}

func expectUpdateForStep(t *testing.T, c *websocket.Conn, wantJobID string) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, raw, err := c.ReadMessage()
	require.NoError(t, err, "expected a job_step_update frame")
	var m WebSocketMessage
	require.NoError(t, json.Unmarshal(raw, &m))
	assert.Equal(t, "job_step_update", m.Type)
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(dataJSON(m.Data), &data))
	assert.Equal(t, wantJobID, data["job_id"])
}

// TestWebSocketConcurrentWrites exercises the per-connection write
// path under -race: the hub broadcast loop and the read loop's
// subscription acks write the same connection concurrently (the pair
// that panicked in CI at gorilla websocket.go:187 — three
// unsynchronized write sites, fixed by WebSocketClient.writeMu). A
// regression re-exposes the race detector report or the panic.
func TestWebSocketConcurrentWrites(t *testing.T) {
	hub, srv := startWSHub(t)
	c := dialWS(t, srv.URL)

	// Drain frames continuously so server writes never block on the
	// test client's read buffer (and vice versa). gorilla allows one
	// reader per connection, so the drainer must be fully stopped
	// before the final assertion reads.
	done := make(chan struct{})
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for {
			select {
			case <-done:
				return
			default:
			}
			_ = c.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			if _, _, err := c.ReadMessage(); err != nil {
				var ne net.Error
				if errors.As(err, &ne) && ne.Timeout() {
					continue
				}
				return
			}
		}
	}()

	jobA := &job.Job{ID: "job-A", Status: job.StatusRunning}

	var wg sync.WaitGroup
	// Broadcast writers hammering the hub loop.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 200; k++ {
				hub.BroadcastJobUpdate(jobA)
				if k%32 == 0 {
					hub.BroadcastAgentEvent("agent_event", map[string]any{"k": k})
				}
			}
		}()
	}
	// A control-writer driving server-side ack writes (read loop) in
	// flight with the broadcasts. The test-conn write is single-
	// goroutine — gorilla's one-writer rule applies to us too.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for k := 0; k < 150; k++ {
			if err := c.WriteJSON(map[string]string{"op": "subscribe", "job_id": "job-A"}); err != nil {
				return
			}
		}
	}()
	wg.Wait()
	close(done)
	<-drained

	// Still alive after the storm: one more op must produce a frame
	// (ack or update — anything proves the server write path survived).
	require.NoError(t, c.SetWriteDeadline(time.Now().Add(2*time.Second)))
	require.NoError(t, c.WriteJSON(map[string]string{"op": "subscribe_all"}))
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, err := c.ReadMessage()
	require.NoError(t, err, "server write path must remain functional under concurrent writes")
}
