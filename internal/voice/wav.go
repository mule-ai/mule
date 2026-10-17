package voice

// wav.go — the PCM → WAV wrap for the STT hop (Herd H6.3).
//
// The in-socket audio is raw 16 kHz mono s16le PCM. Forge's STT proxy
// (POST /v1/audio/transcriptions, the same shape the H6.2 mobile
// voice input uses) expects an uploadable audio file — a WAV is the
// zero-dependency choice: 44-byte RIFF header + the raw PCM.
//
// GOTCHA (2026-10-16): the four chunk magics are written with
// LittleEndian.PutUint32, so the constants must be the BYTE-REVERSED
// ASCII of the tag ("RIFF" on disk = bytes 52 49 46 46 = LE constant
// 0x46464952). The original constants were written big-endian-style
// ("0x52494646" etc.), producing "FFIR…WIVE…fmt …dat " on the wire —
// ffmpeg's sniffer rejected every upload (forge 415 "audio decode
// failed"), which surfaced live as mic calls silently never answering
// (mule logs "call degraded (stt)" and keeps the call open). The unit
// test TestEncodeWAVMagic pins the on-disk bytes; fakeforge never
// parsed the WAV, so it could not catch this.

import "encoding/binary"

// EncodeWAV wraps little-endian s16le mono PCM in a 44-byte RIFF/WAVE
// header for the given sample rate. The returned slice is a fresh
// buffer (header copy + pcm copy); pcm is not mutated.
func EncodeWAV(pcm []byte, sampleRate int) []byte {
	dataSize := len(pcm)
	blockAlign := 2 // mono * 1 byte/sample
	byteRate := sampleRate * blockAlign
	out := make([]byte, 44+dataSize)

	binary.LittleEndian.PutUint32(out[0:], 0x46464952) // "RIFF"
	binary.LittleEndian.PutUint32(out[4:], uint32(36+dataSize))
	binary.LittleEndian.PutUint32(out[8:], 0x45564157)  // "WAVE"
	binary.LittleEndian.PutUint32(out[12:], 0x20746d66) // "fmt "
	binary.LittleEndian.PutUint16(out[16:], 16)         // PCM fmt chunk size
	binary.LittleEndian.PutUint16(out[20:], 1)          // audio format = PCM
	binary.LittleEndian.PutUint16(out[22:], 1)          // channels = mono
	binary.LittleEndian.PutUint32(out[24:], uint32(sampleRate))
	binary.LittleEndian.PutUint32(out[28:], uint32(byteRate))
	binary.LittleEndian.PutUint16(out[32:], uint16(blockAlign))
	binary.LittleEndian.PutUint16(out[34:], 16)         // bits per sample
	binary.LittleEndian.PutUint32(out[36:], 0x61746164) // "data"
	binary.LittleEndian.PutUint32(out[40:], uint32(dataSize))
	copy(out[44:], pcm)
	return out
}
