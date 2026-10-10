package voice

// bargein.go — the barge-in decision (Herd H6.3, "barge-in v1").
//
// While the agent's TTS audio is streaming to the out socket, the
// in-socket audio keeps flowing. If the user speaks over the TTS,
// the call must stop playback and start a new STT buffer. This is a
// pure state machine: it is fed the RMS energy of every in-frame that
// arrives during playback and reports the barge-in exactly once when
// the threshold is met for enough consecutive frames (the hit-count
// hysteresis keeps single-frame noise pops from killing a sentence).
//
// v1 semantics (documented in spec.md):
//   - barge-in is only evaluated while the call is in its Speaking
//     phase (the session feeds it only then);
//   - a trigger is a single-shot: after it fires the machine stays
//     armed=false until the session calls Reset (new TTS playback).

// BargeIn decides when user speech interrupts TTS playback.
type BargeIn struct {
	Threshold float64 // RMS above which a frame counts as "user speech"
	HitFrames int     // consecutive hot frames required to trigger

	hits  int
	armed bool
}

// NewBargeIn builds a barge-in machine. A threshold clearly above the
// VAD's listening threshold is sensible (you want the user to actually
// talk over the agent, not ambient room noise). HitFrames ~3 at 30 ms
// frames means ~90 ms of sustained speech.
func NewBargeIn(threshold float64, hitFrames int) *BargeIn {
	if threshold <= 0 {
		threshold = 600
	}
	if hitFrames <= 0 {
		hitFrames = 3
	}
	return &BargeIn{Threshold: threshold, HitFrames: hitFrames, armed: true}
}

// Feed one in-frame's RMS energy while TTS is playing. Returns true
// exactly once, on the frame that completes the trigger.
func (b *BargeIn) Feed(rms float64) bool {
	if !b.armed {
		return false
	}
	if rms >= b.Threshold {
		b.hits++
	} else {
		b.hits = 0
	}
	if b.hits >= b.HitFrames {
		b.armed = false
		return true
	}
	return false
}

// Reset re-arms the machine for the next TTS playback.
func (b *BargeIn) Reset() {
	b.hits = 0
	b.armed = true
}
