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
