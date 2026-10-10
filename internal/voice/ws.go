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
//	                          (non-audio surfaces print it); a final
//	                          {"done":true} marks the end. H6.4:
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
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// waitCallTimeout bounds how long a voice socket waits for the engine
// to start the job (job poll interval is 1 s; allow generous slack).
const waitCallTimeout = 5 * time.Minute

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

		for chunk := range call.Out() {
			if chunk.Flush {
				// H6.4 barge-in control: mule dropped everything queued
				// behind this; tell the client to drop its playback
				// buffer too (wire + phone tail).
				if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"flush":true}`)); err != nil {
					call.MarkOutClosed()
					log.Printf("voice loop: out-socket for job %s closed: %v", jobID, err)
					return
				}
				continue
			}
			if chunk.Text != "" {
				if err := conn.WriteJSON(map[string]string{"assistant": chunk.Text}); err != nil {
					call.MarkOutClosed()
					log.Printf("voice loop: out-socket for job %s closed: %v", jobID, err)
					return
				}
				continue
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, chunk.Audio); err != nil {
				// Client went away (it hung up via the in socket or
				// closed this socket): stop TTS synthesis. The call
				// itself keeps listening; it only notices through the
				// flag below.
				call.MarkOutClosed()
				log.Printf("voice loop: out-socket for job %s closed: %v", jobID, err)
				return
			}
		}
		// The call's out channel closed: send the end-of-stream
		// marker best-effort (plain text frame, no trailing newline).
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"done":true}`))
	})
}
