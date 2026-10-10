package voice

// vad.go — the voice-loop voice-activity detector (Herd H6.3).
//
// Energy-based VAD over 16 kHz mono s16le PCM frames (~30 ms each):
// a frame is "speech" when its RMS is at or above a threshold. The
// VAD tracks whether the caller is mid-utterance and reports an
// utterance boundary after >= 600 ms of trailing silence (the H6.3
// plan's endpointing rule). No dependency: it is a small pure state
// machine fed frame-by-frame, which is also what makes it unit-testable
// against synthetic PCM.
//
// v1 semantics (documented in spec.md "Voice loop (H6.3)"):
//   - one utterance at a time; audio arriving while the agent is
//     thinking/speaking is handled by the call session, not the VAD
//     (barge-in is a separate machine, see bargein.go);
//   - a sub-threshold frame mid-utterance only counts toward the
//     trailing-silence window, it never truncates the utterance;
//   - the VAD holds no timestamps — frame duration is assumed
//     constant (the WS audio-in protocol's 30 ms contract).

import (
	"fmt"
	"math"
	"time"
)

// VAD reports speech-start / utterance-end events as frames are fed.
type VAD struct {
	frameDur      time.Duration // nominal duration of one frame
	silenceDur    time.Duration // trailing silence => utterance boundary
	threshold     float64       // RMS threshold (0..32767 scale)
	silenceFrames int           // consecutive silence frames needed
	speechActive  bool
	seenSpeech    bool // any speech observed since Reset (for the start event)
	silenceCount  int
}

// NewVAD builds a VAD. frameDur is the assumed duration of each fed
// frame; silenceDur is the trailing-silence window that closes an
// utterance; threshold is the RMS energy floor (typical voice is
// thousands on the 0..32767 scale; room noise is usually under a few
// hundred).
func NewVAD(frameDur, silenceDur time.Duration, threshold float64) *VAD {
	if frameDur <= 0 {
		frameDur = 30 * time.Millisecond
	}
	if silenceDur <= 0 {
		silenceDur = 600 * time.Millisecond
	}
	if threshold <= 0 {
		threshold = 300
	}
	silenceFrames := int(math.Ceil(float64(silenceDur) / float64(frameDur)))
	if silenceFrames < 1 {
		silenceFrames = 1
	}
	return &VAD{
		frameDur:      frameDur,
		silenceDur:    silenceDur,
		threshold:     threshold,
		silenceFrames: silenceFrames,
	}
}

// VADFrame are the events one Feed call can produce.
type VADFrame struct {
	SpeechStarted  bool // first speech frame of an utterance
	UtteranceEnded bool // trailing-silence boundary closed the utterance
}

// FrameRMS is the root-mean-square energy of a s16le PCM frame.
// An empty frame has RMS 0.
func FrameRMS(frame []byte) float64 {
	n := len(frame) / 2
	if n == 0 {
		return 0
	}
	var sum float64
	for i := 0; i < 2*n; i += 2 {
		sample := int16(frame[i]) | int16(frame[i+1])<<8
		sum += float64(sample) * float64(sample)
	}
	return math.Sqrt(sum / float64(n))
}

// Feed advances the VAD with one PCM frame and returns the events the
// frame produced (usually none).
func (v *VAD) Feed(frame []byte) VADFrame {
	ev := VADFrame{}
	energy := FrameRMS(frame)
	if energy < v.threshold {
		if v.speechActive {
			v.silenceCount++
			if v.silenceCount >= v.silenceFrames {
				// Trailing-silence window elapsed: the utterance is
				// complete. The frame that closed the window is the
				// last frame of the utterance's audio (it was quiet,
				// but including it is harmless for STT).
				v.speechActive = false
				v.silenceCount = 0
				ev.UtteranceEnded = true
			}
		}
		return ev
	}
	// Speech frame.
	if !v.speechActive {
		v.speechActive = true
		v.silenceCount = 0
		if !v.seenSpeech {
			v.seenSpeech = true
			ev.SpeechStarted = true
		}
	}
	return ev
}

// Reset returns the VAD to its initial state (fresh utterance
// tracking). The call session resets it after each endpointing
// decision.
func (v *VAD) Reset() {
	v.speechActive = false
	v.seenSpeech = false
	v.silenceCount = 0
}

// String implements a human-readable view for logs/debugging.
func (v *VAD) String() string {
	state := "idle"
	if v.speechActive {
		state = "speaking"
	}
	return fmt.Sprintf("vad{state:%s, silence:%dms/%s, threshold:%.0f}",
		state, time.Duration(v.silenceCount)*v.frameDur, v.silenceDur, v.threshold)
}
