package voice

// config.go — the voice_call step config (Herd H6.3).
//
// The voice-loop call is a workflow step of type `voice_call`. Its
// config JSONB carries the forge credentials and the audio knobs:
//
//	{
//	  "forge_url": "http://…",          // required (as mule reaches forge)
//	  "api_key":   "sk_forge_…",        // required
//	  "agent_id":  "…",                // required unless conversation_id
//	  "conversation_id": "…",          // pin an existing conversation
//	  "mode": "steer",                 // v1: steer/message post the same
//	                                    // POST /messages (see forge.go H3.2)
//	  "tts_voice": "af_heart",
//	  "tts_response_format": "pcm",    // forge /v1/audio/speech response_format
//	  "tts_sample_rate": 24000,        // sample rate of the TTS audio out
//	  "silence_ms": 500,              // VAD min trailing silence + early-
//	                                    // partial point (H6.4)
//	  "frame_ms": 30,                  // nominal in-frame duration
//	  "endpoint_ratio": 3.0,           // speech/silence RMS ratio to end
//	  "max_utterance_ms": 15000,       // hard cap on an open utterance
//	  "vad_threshold": 300,            // RMS floor for "the user speaks"
//	  "barge_in_threshold": 600,       // RMS floor to interrupt TTS
//	  "turn_timeout_secs": 120
//	}
//
// The in/out audio sample rate is fixed by the call protocol:
// 16 kHz mono s16le both directions (the same rate as the H6.2
// mobile voice input). The TTS out-rate is whatever the TTS backend
// returns; v1 pins it via tts_sample_rate (Kokoro's PCM default is
// 24 kHz) so both surfaces can build their players up front — the
// out socket itself does not carry rate metadata.

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// CallConfig is the parsed+validated voice_call step config.
type CallConfig struct {
	ForgeURL       string
	APIKey         string
	AgentID        string
	ConversationID string
	Mode           string // "steer" (default), "message"
	// Model is the per-call "voice model" choice: a forge profile name,
	// id, or model label (e.g. "Laguna (dev)"). Empty = the agent's
	// default profile. Applied once per call (before the first turn) by
	// Call.ensureModel via forge's session model switcher; it can also
	// arrive per-job via input_data {"model": …} (Hub.StartCall).
	Model             string
	TTSVoice          string
	TTSResponseFormat string
	TTSRate           int
	SilenceDur        time.Duration
	FrameDur          time.Duration
	VADThreshold      float64
	BargeInThreshold  float64
	// H6.4 endpointing: an utterance ends when trailing silence ≥
	// SilenceDur AND mean(last 400 ms speech) / mean(last 300 ms
	// silence) ≥ EndpointRatio (digital silence passes as an infinite
	// ratio); MaxUtterance force-ends an open utterance regardless.
	EndpointRatio float64
	MaxUtterance  time.Duration
	TurnTimeout   time.Duration
}

// InSampleRate is the fixed in/out-socket audio rate (16 kHz mono
// s16le — the H6.2 voice-input rate).
const InSampleRate = 16000

// ParseCallConfig extracts and validates a CallConfig from the raw
// step config map. All defaults are applied; validation errors come
// back combined.
func ParseCallConfig(cfg map[string]interface{}) (CallConfig, error) {
	var c CallConfig
	c.ForgeURL = strings.TrimRight(strOr(cfg, "forge_url"), "/")
	c.APIKey = strOr(cfg, "api_key")
	c.Model = strings.TrimSpace(strOr(cfg, "model"))
	c.AgentID = strings.TrimSpace(strOr(cfg, "agent_id"))
	c.ConversationID = strings.TrimSpace(strOr(cfg, "conversation_id"))
	c.Mode = "steer"
	if m := strings.TrimSpace(strOr(cfg, "mode")); m != "" {
		c.Mode = m
	}
	c.TTSVoice = "af_heart"
	if v := strings.TrimSpace(strOr(cfg, "tts_voice")); v != "" {
		c.TTSVoice = v
	}
	c.TTSResponseFormat = "pcm"
	if v := strings.TrimSpace(strOr(cfg, "tts_response_format")); v != "" {
		c.TTSResponseFormat = v
	}
	c.TTSRate = 24000
	if n := intOr(cfg, "tts_sample_rate", 0); n > 0 {
		c.TTSRate = n
	}
	c.SilenceDur = 500 * time.Millisecond
	if n := intOr(cfg, "silence_ms", 0); n > 0 {
		c.SilenceDur = time.Duration(n) * time.Millisecond
	}
	c.FrameDur = 30 * time.Millisecond
	if n := intOr(cfg, "frame_ms", 0); n > 0 {
		c.FrameDur = time.Duration(n) * time.Millisecond
	}
	c.VADThreshold = 300
	if f := floatOr(cfg, "vad_threshold", 0); f > 0 {
		c.VADThreshold = f
	}
	c.BargeInThreshold = 600
	if f := floatOr(cfg, "barge_in_threshold", 0); f > 0 {
		c.BargeInThreshold = f
	}
	c.EndpointRatio = 3.0
	if f := floatOr(cfg, "endpoint_ratio", 0); f > 0 {
		c.EndpointRatio = f
	}
	c.MaxUtterance = 15 * time.Second
	if n := intOr(cfg, "max_utterance_ms", 0); n > 0 {
		c.MaxUtterance = time.Duration(n) * time.Millisecond
	}
	c.TurnTimeout = 120 * time.Second
	if n := intOr(cfg, "turn_timeout_secs", 0); n > 0 {
		c.TurnTimeout = time.Duration(n) * time.Second
	}

	var errs []string
	if c.ForgeURL == "" {
		errs = append(errs, "forge_url is required for voice_call steps")
	} else if u, err := url.Parse(c.ForgeURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errs = append(errs, "forge_url must be an http or https URL")
	}
	if c.APIKey == "" {
		errs = append(errs, "api_key is required for voice_call steps")
	}
	if c.AgentID == "" && c.ConversationID == "" {
		errs = append(errs, "either agent_id or conversation_id is required for voice_call steps")
	}
	if c.Mode != "steer" && c.Mode != "message" {
		errs = append(errs, `mode must be "steer" or "message" (fork/follow_up are conversation-management modes, not call utterances)`)
	}
	if len(errs) > 0 {
		return c, fmt.Errorf("invalid voice_call config: %s", strings.Join(errs, "; "))
	}
	return c, nil
}

func strOr(cfg map[string]interface{}, key string) string {
	if cfg == nil {
		return ""
	}
	s, _ := cfg[key].(string)
	return s
}

func intOr(cfg map[string]interface{}, key string, def int) int {
	if cfg == nil {
		return def
	}
	switch n := cfg[key].(type) {
	case float64:
		if n > 0 && n == float64(int64(n)) {
			return int(n)
		}
	case int:
		return n
	case int64:
		return int(n)
	}
	return def
}

func floatOr(cfg map[string]interface{}, key string, def float64) float64 {
	if cfg == nil {
		return def
	}
	switch n := cfg[key].(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return def
}
