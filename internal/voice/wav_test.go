package voice

import (
	"encoding/binary"
	"testing"
)

func TestEncodeWAVHeader(t *testing.T) {
	pcm := make([]byte, 3200) // 1 s of 16 kHz mono s16le
	for i := range pcm {
		pcm[i] = byte(i)
	}
	wav := EncodeWAV(pcm, 16000)

	if len(wav) != 44+3200 {
		t.Fatalf("len = %d, want %d", len(wav), 44+3200)
	}
	if got := string(wav[0:4]); got != "RIFF" {
		t.Errorf("RIFF tag = %q, want \"RIFF\"", got)
	}
	if binary.LittleEndian.Uint32(wav[4:]) != 36+3200 {
		t.Errorf("RIFF size = %d, want %d", binary.LittleEndian.Uint32(wav[4:]), 36+3200)
	}
	if got := string(wav[8:12]); got != "WAVE" {
		t.Errorf("WAVE tag = %q, want \"WAVE\"", got)
	}
	if got := string(wav[12:16]); got != "fmt " {
		t.Errorf("fmt chunk tag = %q, want \"fmt \"", got)
	}
	if got := binary.LittleEndian.Uint16(wav[20:]); got != 1 {
		t.Errorf("audio format = %d, want 1 (PCM)", got)
	}
	if got := binary.LittleEndian.Uint16(wav[22:]); got != 1 {
		t.Errorf("channels = %d, want 1", got)
	}
	if got := binary.LittleEndian.Uint32(wav[24:]); got != 16000 {
		t.Errorf("sample rate = %d, want 16000", got)
	}
	if got := binary.LittleEndian.Uint32(wav[28:]); got != 32000 {
		t.Errorf("byte rate = %d, want 32000", got)
	}
	if got := binary.LittleEndian.Uint16(wav[32:]); got != 2 {
		t.Errorf("block align = %d, want 2", got)
	}
	if got := binary.LittleEndian.Uint16(wav[34:]); got != 16 {
		t.Errorf("bits per sample = %d, want 16", got)
	}
	if got := string(wav[36:40]); got != "data" {
		t.Errorf("data chunk tag = %q, want \"data\"", got)
	}
	if got := binary.LittleEndian.Uint32(wav[40:]); got != 3200 {
		t.Errorf("data size = %d, want 3200", got)
	}
	// Payload is a copy, not a view: mutating the source must not
	// change the encoded bytes.
	pcm[100] = 0xFF
	if wav[144] == 0xFF {
		t.Error("EncodeWAV aliases its input payload")
	}
}

func TestEncodeWAVEmpty(t *testing.T) {
	if want, got := 44, len(EncodeWAV(nil, 16000)); got != want {
		t.Fatalf("empty-pcm WAV len = %d, want %d", got, want)
	}
	wav := EncodeWAV(nil, 24000)
	if got := binary.LittleEndian.Uint32(wav[40:]); got != 0 {
		t.Errorf("empty data size = %d, want 0", got)
	}
	if got := binary.LittleEndian.Uint32(wav[24:]); got != 24000 {
		t.Errorf("sample rate = %d, want 24000", got)
	}
	if got := len(wav) - 44; got != 0 {
		t.Errorf("empty payload length = %d, want 0", got)
	}
}
