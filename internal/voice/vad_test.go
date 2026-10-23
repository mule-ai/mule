package voice

import (
	"testing"
	"time"
)

// makeFrame builds one s16le PCM frame: len samples of the given
// amplitude (a square "tone"); amplitude 0 is digital silence.
func makeFrame(n int, amp int16) []byte {
	b := make([]byte, 2*n)
	for i := 0; i < n; i++ {
		b[2*i] = byte(amp)
		b[2*i+1] = byte(amp >> 8)
	}
	return b
}

func TestFrameRMS(t *testing.T) {
	if got := FrameRMS(nil); got != 0 {
		t.Errorf("empty frame RMS = %v, want 0", got)
	}
	if got := FrameRMS(make([]byte, 1)); got != 0 {
		t.Errorf("odd-length frame RMS = %v, want 0 (no complete samples)", got)
	}
	// Constant amplitude 8000 → RMS exactly 8000.
	if got := FrameRMS(makeFrame(100, 8000)); got != 8000 {
		t.Errorf("RMS(8000 constant) = %v, want 8000", got)
	}
	// Mixed: half 0, half 10000 → RMS = 10000/sqrt(2).
	f := makeFrame(100, 0)
	copy(f[100:], makeFrame(50, 10000))
	want := 7071.0678 // 10000 * sqrt(0.5)
	if got := FrameRMS(f); got < want-1 || got > want+1 {
		t.Errorf("RMS(half-frame) = %v, want ~%v", got, want)
	}
}

func TestVADBoundaryAtSilenceWindow(t *testing.T) {
	vad := NewVAD(30*time.Millisecond, 600*time.Millisecond, 300)
	speech := makeFrame(480, 8000) // 30 ms of voice at 16 kHz
	silence := makeFrame(480, 0)

	// 1 s of speech: only the first frame reports a start.
	started := 0
	for i := 0; i < 33; i++ {
		if ev := vad.Feed(speech); ev.SpeechStarted {
			started++
		}
	}
	if started != 1 {
		t.Fatalf("speech-start events = %d, want 1", started)
	}
	if ended := vad.Feed(speech).UtteranceEnded; ended {
		t.Fatal("utterance ended while speech is still flowing")
	}

	// 600 ms of silence = 20 frames: the boundary must land within the
	// window (no earlier, no later than the 20th silent frame).
	boundaryAt := -1
	for i := 0; i < 25; i++ {
		if vad.Feed(silence).UtteranceEnded {
			boundaryAt = i
			break
		}
	}
	if boundaryAt != 19 {
		t.Fatalf("utterance boundary on silence frame %d, want 19 (600 ms at 30 ms frames)", boundaryAt)
	}

	// After the boundary the VAD is idle again: silence produces no
	// further events.
	if ev := vad.Feed(silence); ev.UtteranceEnded || ev.SpeechStarted {
		t.Errorf("post-boundary silence produced event %+v", ev)
	}
}

func TestVADSubWindowGapDoesNotEndUtterance(t *testing.T) {
	vad := NewVAD(30*time.Millisecond, 600*time.Millisecond, 300)
	speech := makeFrame(480, 8000)
	silence := makeFrame(480, 0)

	vad.Feed(speech) // speech started
	// 300 ms of gap (10 frames) is below the 600 ms window.
	for i := 0; i < 10; i++ {
		if vad.Feed(silence).UtteranceEnded {
			t.Fatalf("utterance ended after only %d ms of gap", (i+1)*30)
		}
	}
	// Speech resumes: still the same utterance — no new start event
	// until a full Reset.
	if ev := vad.Feed(speech); ev.SpeechStarted {
		t.Error("resumed speech reported a new start within the same utterance")
	}
	// Then silence to the window → one boundary total.
	ended := 0
	for i := 0; i < 30 && ended == 0; i++ {
		if vad.Feed(silence).UtteranceEnded {
			ended++
		}
	}
	if ended != 1 {
		t.Fatal("expected exactly one utterance-end after resume")
	}
}

func TestVADBelowThresholdIsSilence(t *testing.T) {
	vad := NewVAD(30*time.Millisecond, 600*time.Millisecond, 300)
	// Room noise at amplitude 100 (RMS 100 < 300): never starts.
	noise := makeFrame(480, 100)
	for i := 0; i < 40; i++ {
		if ev := vad.Feed(noise); ev.SpeechStarted {
			t.Fatalf("below-threshold noise started speech on frame %d", i)
		}
	}
}

func TestVADReset(t *testing.T) {
	vad := NewVAD(30*time.Millisecond, 600*time.Millisecond, 300)
	speech := makeFrame(480, 8000)
	if !vad.Feed(speech).SpeechStarted {
		t.Fatal("first frame should start speech")
	}
	vad.Reset()
	if ev := vad.Feed(speech); !ev.SpeechStarted {
		t.Error("post-Reset speech should report a fresh start")
	}
}

// ---------------------------------------------------------------- H6.4
// endpointing: the pure rule + the VAD wiring

func TestDecideEndpointPure(t *testing.T) {
	min := 500 * time.Millisecond
	max := 15 * time.Second
	cases := []struct {
		name        string
		speech, sil float64
		ratio       float64
		silence     time.Duration
		open        time.Duration
		wantEnd     bool
		wantReason  string
	}{
		{"silence below the window", 8000, 0, 3.0, 200 * time.Millisecond, 2 * time.Second, false, ""},
		{"window + digital silence ends", 8000, 0, 3.0, 500 * time.Millisecond, 2 * time.Second, true, "ratio"},
		{"window + loud-enough speech/silence ratio", 8000, 2000, 3.0, 500 * time.Millisecond, 2 * time.Second, true, "ratio"}, // 8000/2000 = 4.0
		{"window + ratio at the threshold", 6000, 2000, 3.0, 500 * time.Millisecond, 2 * time.Second, true, "ratio"},           // exactly 3.0
		{"window + ratio below the threshold holds", 500, 200, 3.0, 500 * time.Millisecond, 2 * time.Second, false, ""},        // 2.5 < 3.0
		{"window + no speech reference holds", 0, 200, 3.0, 500 * time.Millisecond, 2 * time.Second, false, ""},
		{"hard cap force-ends mid-speech", 8000, 0, 3.0, 0, 15 * time.Second, true, "max"},
		{"hard cap just under", 8000, 0, 3.0, 0, 14999 * time.Millisecond, false, ""},
		{"zero ratio falls back to 3.0", 8000, 2000, 0, 500 * time.Millisecond, 2 * time.Second, true, "ratio"},
	}
	for _, tc := range cases {
		d := DecideEndpoint(tc.speech, tc.sil, tc.ratio, tc.silence, min, tc.open, max)
		if d.End != tc.wantEnd || (d.End && d.Reason != tc.wantReason) {
			t.Errorf("%s: DecideEndpoint = %+v, want end=%v reason=%q", tc.name, d, tc.wantEnd, tc.wantReason)
		}
	}
}

// feedUtterancePattern feeds the VAD: speechAmp speech frames, then
// noiseAmp below-threshold frames, then digital-silence frames, and
// reports where the events landed.
func TestVADRatioHoldsUtteranceOverNoisySilence(t *testing.T) {
	vad := NewVAD(30*time.Millisecond, 500*time.Millisecond, 300)
	speech := makeFrame(480, 500) // RMS 500 (above the 300 floor)
	noise := makeFrame(480, 200)  // RMS 200 (below: "silence"), 500/200 = 2.5 < 3.0
	silence := makeFrame(480, 0)  // digital silence

	vad.Feed(speech) // start
	for i := 0; i < 3; i++ {
		vad.Feed(speech)
	}
	// 170 ms of noisy silence: below the 500 ms window — no event.
	for i := 0; i < 5; i++ {
		if ev := vad.Feed(noise); ev.UtteranceEnded || ev.Partial {
			t.Fatalf("event %+v at noisy-silence frame %d (only %d ms of silence)", ev, i, (i+1)*30)
		}
	}
	// 500 ms of noisy silence: the PARTIAL fires (first window) but the
	// ratio (2.5 < 3.0) must hold the utterance open.
	var partialAt, endAt int = -1, -1
	for i := 0; i < 17; i++ {
		ev := vad.Feed(noise)
		if ev.Partial && partialAt < 0 {
			partialAt = i
		}
		if ev.UtteranceEnded && endAt < 0 {
			endAt = i
		}
	}
	if partialAt != 11 { // 5 pre-loop + frame 11 = 17th silence frame = 510 ms ≥ 500 ms window
		t.Fatalf("partial on frame %d, want 11 (first 500 ms window)", partialAt)
	}
	if endAt >= 0 {
		t.Fatalf("utterance ended on noisy-silence frame %d, want held open (ratio 2.5 < 3.0)", endAt)
	}
	// Speech resumes: the same utterance continues.
	if ev := vad.Feed(speech); ev.SpeechStarted || ev.UtteranceEnded {
		t.Fatalf("resumed speech produced %+v", ev)
	}
	// Digital silence now: the ratio is infinite → the endpoint lands
	// on the first full window after the resume.
	ended := 0
	for i := 0; i < 25; i++ {
		if vad.Feed(silence).UtteranceEnded {
			ended++
		}
	}
	if ended != 1 {
		t.Fatalf("post-resume endpoint count = %d, want 1", ended)
	}
}

func TestVADPartialFiresOnce(t *testing.T) {
	vad := NewVAD(30*time.Millisecond, 500*time.Millisecond, 300)
	speech := makeFrame(480, 500)
	noise := makeFrame(480, 200)
	silence := makeFrame(480, 0)
	vad.Feed(speech)
	partials := 0
	for i := 0; i < 12; i++ { // 360 ms of noisy silence: window not reached
		vad.Feed(noise)
	}
	// Cross the window once (held open by the ratio), then end on
	// digital silence: the partial must have fired exactly once, and
	// never on the ending frame.
	for i := 0; i < 20; i++ {
		ev := vad.Feed(noise)
		partials += boolInt(ev.Partial)
		if ev.UtteranceEnded && ev.Partial {
			t.Fatal("partial and endpoint fired on the same frame")
		}
	}
	for i := 0; i < 20; i++ {
		if ev := vad.Feed(silence); ev.Partial {
			t.Fatal("second partial fired")
		}
	}
	if partials != 1 {
		t.Fatalf("partials = %d, want 1", partials)
	}
}

func TestVADMaxUtteranceCap(t *testing.T) {
	vad := NewVAD(30*time.Millisecond, 500*time.Millisecond, 300)
	vad.MaxUtteranceDur = 15 * time.Second
	speech := makeFrame(480, 8000)
	// 15 s of unbroken speech = 500 frames: force-end mid-speech.
	endedAt := -1
	for i := 0; i < 600; i++ {
		if vad.Feed(speech).UtteranceEnded {
			endedAt = i
			break
		}
	}
	if endedAt != 499 { // frame index 499 = the 500th frame = 15 s
		t.Fatalf("max-cap endpoint on frame %d, want 499 (15 s at 30 ms)", endedAt)
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
