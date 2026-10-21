package voice

// ws.go — the voice-loop transport endpoints (Herd H6.3).
//
// Two WebSocket paths, one per direction, both addressed by the mule
// job id (the call is a mule job, so the job id is the call's address):
//
//	WS /voice/{job_id}/in   audio IN:  binary frames of 16 kHz mono
//	                          s16le PCM (~30 ms each). Text frames:
//	                          {"hangup":true} hangs up; {"text": …}
//	                          is a typed utterance (the CLI's
//	                          key-typed-text fallback: skips STT, goes
//	                          straight to the agent turn); anything
//	                          else is ignored. The in socket closing
//	                          also hangs up the call.
//	WS /voice/{job_id}/out  audio OUT: binary frames carrying the TTS
//	                          audio chunks (out_sample_rate /
//	                          out_format from the step config; v1 is
//	                          24 kHz PCM from Kokoro). Text frames:
//	                          {"assistant": …} after each agent turn
//	                          {"notice": …} on a degraded hop (stt/tts/
//	                          agent failure) — the caller is told, not
//	                          left in silence
//	                          (non-audio surfaces print it);
//	                          {"user": …} is what the caller just said
//	                          (final STT or typed line) so their own
//	                          words show up on screen; {"state": …} is
//	                          the call phase — listening/utterance/
//	                          thinking/speaking/greeting — the "is it
//	                          working?" status lane (2026-10-17: calls
//	                          looked dead between dial and first
//	                          answer);
//	                          a final {"done":true} marks the end. H6.4:
//	                          {"flush":true} is the barge-in control —
//	                          mule has drained its out queue, so the
//	                          client must drop its OWN playback buffer
//	                          immediately (AudioTrack flush).
//	                          Surfaces that do not implement the flush
//	                          must at least ignore the frame.
//
// v1 transport decision (documented in spec.md and the example's
// README): the audio flows PHONE/HOST ⇄ MULE directly over these
// sockets; the surfaces create the job via mule's jobs API and then
// attach. Ranch's JSON-lines wire protocol is NOT involved — audio is
// the wrong payload for it, and the daemon stays out of the media
// path (documented in the ranch-2 commit for H6.3).
//
// Both handlers wait (WaitCall) for the engine to register the call
// for the job, because the client can attach immediately after job
// creation while the engine worker is still polling the queue.
//
// Auth: like the fleet's /ws feed, these routes are unauthenticated;
// the 128-bit job id is the capability (documented in spec.md).

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// waitCallTimeout bounds how long a voice socket waits for the engine
// to start the job (job poll interval is 1 s; allow generous slack).
const waitCallTimeout = 5 * time.Minute

// outPingInterval keeps the out socket alive across idle stretches
// (the caller just listening). Cloudflare's edge drops idle WebSockets
// at ~100 s and cloudflared likewise ages out silent streams — a dead
// out socket previously turned every LATER turn silent (no TTS, no
// captions) while the call looked healthy. A control ping every 20 s
// is traffic, and its write failure is how we learn the client is
// actually gone (fast MarkOutClosed instead of a stalled TTS write).
const outPingInterval = 20 * time.Second

// upgrader is shared; origin checking is intentionally off in v1 (the
// call's job id is the capability, same posture as /ws).
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// callID extracts the job id from the route.
func callID(r *http.Request) string {
	vars := mux.Vars(r)
	return vars["job_id"]
}

// waitCallForJob blocks until the hub has a call for the job, the
// request context dies, or the timeout fires.
func waitCallForJob(hub *Hub, ctx context.Context, jobID string) (*Call, error) {
	call, ok := hub.WaitCall(ctx, jobID, waitCallTimeout)
	if !ok {
		return nil, fmt.Errorf("no voice call for job %s (is the job running a voice_call workflow?)", jobID)
	}
	return call, nil
}

// InHandler serves WS /voice/{job_id}/in.
func InHandler(hub *Hub) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jobID := callID(r)
		call, err := waitCallForJob(hub, r.Context(), jobID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			// Upgrade failed (often client-side); nothing to clean up.
			return
		}
		defer conn.Close()

		// Spec: a hang-up control frame OR the in socket closing ends
		// the call. The defer covers the close path; the explicit
		// hang-up frame is processed below.
		defer call.HangUp()

		// The in socket is the "the caller is here" signal: Run uses it
		// to fire the answer-the-call greeting (never for a job nobody
		// dialed).
		call.OnInAttached()

		// Reader pump: binary frames are audio; text frames are
		// control ({"hangup":true}).
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				// Connection ended (clean or not): the deferred
				// HangUp() terminates the call.
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					log.Printf("voice loop: in-socket for job %s closed: %v", jobID, err)
				}
				return
			}
			switch msgType {
			case websocket.BinaryMessage:
				call.FeedIn(data)
			case websocket.TextMessage:
				var ctrl struct {
					HangUp bool   `json:"hangup"`
					Text   string `json:"text"`
				}
				if err := json.Unmarshal(data, &ctrl); err == nil {
					if ctrl.HangUp {
						log.Printf("voice loop: job %s hung up (control frame)", jobID)
						return
					}
					if strings.TrimSpace(ctrl.Text) != "" {
						// Typed utterance (CLI text fallback): no STT.
						call.FeedText(strings.TrimSpace(ctrl.Text))
					}
				}
				// Unknown text frames are ignored (forward compat).
			}
		}
	})
}

// OutHandler serves WS /voice/{job_id}/out: it streams the call's TTS
// chunks to the client until the call ends, then a done marker.
//
// Liveness (2026-10-17): the handler pings every outPingInterval (a
// dead-in-the-water edge — Cloudflare/cloudflared age out idle
// WebSockets) and treats a failed write as the client leaving
// (MarkOutClosed → TTS stops streaming into the void). Attach sets
// outOpen back to 1, so a client that reconnects this socket resumes
// its audio lane mid-call.
func OutHandler(hub *Hub) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jobID := callID(r)
		call, err := waitCallForJob(hub, r.Context(), jobID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Single writer: the drain loop and the ping ticker share the
		// conn (gorilla forbids concurrent writes).
		var writeMu sync.Mutex
		write := func(mt int, b []byte) error {
			writeMu.Lock()
			defer writeMu.Unlock()
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			return conn.WriteMessage(mt, b)
		}
		fail := func(why string, e error) {
			call.MarkOutClosed()
			log.Printf("voice loop: out-socket for job %s %s: %v", jobID, why, e)
		}

		call.MarkOutOpen()

		// Keepalive + fast client-gone detection. A client that stopped
		// reading dies here long before a TTS write would stall.
		pingDone := make(chan struct{})
		defer close(pingDone)
		go func() {
			tick := time.NewTicker(outPingInterval)
			defer tick.Stop()
			for {
				select {
				case <-pingDone:
					return
				case <-tick.C:
					writeMu.Lock()
					_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
					perr := conn.WriteMessage(websocket.PingMessage, nil)
					writeMu.Unlock()
					if perr != nil {
						call.MarkOutClosed()
						log.Printf("voice loop: out-socket for job %s ping failed: %v", jobID, perr)
						return
					}
				}
			}
		}()

		for chunk := range call.Out() {
			if chunk.Flush {
				// H6.4 barge-in control: mule dropped everything queued
				// behind this; tell the client to drop its playback
				// buffer too (wire + phone tail).
				if e := write(websocket.TextMessage, []byte(`{"flush":true}`)); e != nil {
					fail("closed", e)
					return
				}
				continue
			}
			var payload []byte
			switch {
			case chunk.Notice != "":
				payload, _ = json.Marshal(map[string]string{"notice": chunk.Notice})
			case chunk.User != "":
				payload, _ = json.Marshal(map[string]string{"user": chunk.User})
			case chunk.State != "":
				payload, _ = json.Marshal(map[string]string{"state": chunk.State})
			case chunk.Text != "":
				payload, _ = json.Marshal(map[string]string{"assistant": chunk.Text})
			}
			if payload != nil {
				if e := write(websocket.TextMessage, payload); e != nil {
					fail("closed", e)
					return
				}
				continue
			}
			// Client went away (it hung up via the in socket or closed
			// this socket): stop TTS synthesis. The call itself keeps
			// listening; it only notices through the flag below.
			if e := write(websocket.BinaryMessage, chunk.Audio); e != nil {
				fail("closed", e)
				return
			}
		}
		// The call's out channel closed: send the end-of-stream
		// marker best-effort (plain text frame, no trailing newline).
		_ = write(websocket.TextMessage, []byte(`{"done":true}`))
	})
}
