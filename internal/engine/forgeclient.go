package engine

// forgeclient.go — exported forge REST/SSE/audio client (Herd H6.3).
//
// The voice loop (internal/voice) talks to forge for three things:
// steering the agent conversation (H3.2's forge_conversation
// transport, minus the step scaffolding), STT (the H6.2/H6.3 audio
// proxy POST /v1/audio/transcriptions) and TTS (POST
// /v1/audio/speech). This file exposes exactly that surface on the
// engine package's forgeStepClient so the voice package can reuse the
// exact same addressing, SSE turn-wait and error shapes without the
// workflow-step machinery.
//
// The voice package depends on this; this package knows nothing about
// the voice package (the engine is wired to a VoiceExecutor
// interface at construction — see engine.go), so there is no import
// cycle.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// ForgeClient is a forge REST/SSE/audio client for use outside a
// workflow step (the voice loop is the H6.3 consumer).
type ForgeClient struct {
	rest  *forgeStepClient // REST + SSE, 30 s per-call timeout
	audio *http.Client     // audio calls: STT/TTS can be slower
}

// NewForgeClient builds a client for the given forge base URL
// (http/https, no trailing slash) and sk_forge_ API key.
func NewForgeClient(baseURL, apiKey string) *ForgeClient {
	return &ForgeClient{
		rest:  newForgeStepClient(forgeStepConfig{ForgeURL: baseURL, APIKey: apiKey}),
		audio: &http.Client{Timeout: 90 * time.Second},
	}
}

// ResolveConversation picks the target conversation: a pinned
// conversation_id wins; otherwise the agent's currently active
// conversation; otherwise a freshly created one (mirrors the
// forge_conversation step's resolution order minus fork).
func (f *ForgeClient) ResolveConversation(ctx context.Context, agentID, pinned string) (string, error) {
	if pinned != "" {
		return pinned, nil
	}
	if agentID == "" {
		return "", fmt.Errorf("resolve conversation requires agent_id or a pinned conversation_id")
	}
	_, active, err := f.rest.agentActive(ctx, agentID)
	if err != nil {
		return "", fmt.Errorf("failed to check active conversation for agent %s: %w", agentID, err)
	}
	if active != "" {
		return active, nil
	}
	id, err := f.rest.createConversation(ctx, agentID, "")
	if err != nil {
		return "", fmt.Errorf("failed to create forge conversation: %w", err)
	}
	return id, nil
}

// PostAndWait posts content to a conversation and waits for the turn
// to end (SSE turn_ended + messages-table attach-race fallback — the
// forge_conversation step's exact wait path), returning the turn's
// final assistant text. The content is NOT mule-job-tagged (the
// voice call is the user talking; the mule job is transport, not
// provenance).
func (f *ForgeClient) PostAndWait(ctx context.Context, sessionID, content string, timeout time.Duration) (string, error) {
	lastSeq, err := f.rest.lastMessageSeq(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to list forge messages for %s: %w", sessionID, err)
	}
	if err := f.rest.postMessage(ctx, sessionID, content); err != nil {
		return "", fmt.Errorf("failed to post to forge conversation %s: %w", sessionID, err)
	}
	waitSeq, err := f.rest.waitForTurn(ctx, sessionID, lastSeq, time.Now().Add(timeout))
	if err != nil {
		return "", err
	}
	assistant, err := f.rest.latestAssistantAfter(ctx, sessionID, waitSeq)
	if err != nil {
		return "", fmt.Errorf("failed to fetch the turn's assistant output for %s: %w", sessionID, err)
	}
	return assistant, nil
}

// PostMessage posts content to a conversation WITHOUT waiting for the
// turn to end (H6.4 in-turn steer: an overlap utterance lands as the
// agent's next message while its own turn is in flight; the voice
// loop does not block on it and does not TTS-respond to it).
func (f *ForgeClient) PostMessage(ctx context.Context, sessionID, content string) error {
	if err := f.rest.postMessage(ctx, sessionID, content); err != nil {
		return fmt.Errorf("failed to post to forge conversation %s: %w", sessionID, err)
	}
	return nil
}

// MatchProfile resolves a user-chosen voice-model label (profile name,
// id, or model — e.g. "Laguna (dev)" or just "laguna") against forge's
// profiles.
func (f *ForgeClient) MatchProfile(ctx context.Context, want string) (*VoiceProfile, error) {
	return f.rest.matchProfile(ctx, want)
}

// SetModel re-points a conversation's live model via forge's session
// model switcher (PATCH /sessions/:id — rebinds the attached harness
// conversation via pi-durable setModel).
func (f *ForgeClient) SetModel(ctx context.Context, sessionID string, p *VoiceProfile) error {
	if p == nil {
		return fmt.Errorf("nil profile")
	}
	return f.rest.setModelOverrides(ctx, sessionID, p.Provider, p.Model, p.BaseURL)
}

// SessionContext is the slice of GET /sessions/:id/context the voice
// loop's pre-call budget check consumes.
type SessionContext struct {
	Source             string `json:"source"`
	ActiveContextChars int64  `json:"active_context_chars"`
	ActiveEntryCount   int    `json:"active_entry_count"`
}

// CatalogModel is one entry of GET /v1/models/catalog (forge's
// models.json): the id plus the contextWindow the harness compacts
// against.
type CatalogModel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextWindow uint64 `json:"context_window"`
}

// SessionContext fetches the session's ACTIVE context (chars; the
// harness source reflects post-compaction reality).
func (f *ForgeClient) SessionContext(ctx context.Context, sessionID string) (SessionContext, error) {
	return f.rest.sessionContext(ctx, sessionID)
}

// CompactSession submits a background compaction (forge POST
// /sessions/:id/compact). It runs as a conversation-owned task; an
// in-flight turn is never interrupted and the summary lands at the
// next turn boundary at the latest.
func (f *ForgeClient) CompactSession(ctx context.Context, sessionID string) error {
	return f.rest.compactSession(ctx, sessionID)
}

// ModelCatalog returns the machine's declared models (id +
// contextWindow) from forge's models.json.
func (f *ForgeClient) ModelCatalog(ctx context.Context) ([]CatalogModel, error) {
	return f.rest.modelCatalog(ctx)
}

// Transcribe sends 16 kHz mono s16le PCM bytes wrapped as WAV to forge's
// STT proxy (POST /v1/audio/transcriptions — the same multipart shape
// the H6.2 mobile voice input and the forge web UI send) and returns
// the transcript text.
func (f *ForgeClient) Transcribe(ctx context.Context, wav []byte, filename string) (string, error) {
	if filename == "" {
		filename = "utterance.wav"
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("failed to create multipart file part: %w", err)
	}
	if _, err := fw.Write(wav); err != nil {
		return "", fmt.Errorf("failed to write multipart file part: %w", err)
	}
	for field, value := range map[string]string{
		"model":           "parakeet",
		"response_format": "json",
	} {
		if err := mw.WriteField(field, value); err != nil {
			return "", fmt.Errorf("failed to write multipart field %s: %w", field, err)
		}
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		f.rest.baseURL+"/v1/audio/transcriptions", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+f.rest.apiKey)

	resp, err := f.audio.Do(req)
	if err != nil {
		return "", fmt.Errorf("forge STT request failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("failed to read forge STT response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("forge STT %s returned %d: %s",
			"/v1/audio/transcriptions", resp.StatusCode, truncateForError(string(data)))
	}
	// Parakeet's shape: {"text": "..."}.
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("failed to parse forge STT response: %w", err)
	}
	return out.Text, nil
}

// Speech synthesizes text to audio via forge's TTS proxy (POST
// /v1/audio/speech — the OpenAI-compatible shape the forge web UI
// sends). It returns the audio bytes and the response Content-Type
// (the voice loop streams whatever the backend returned; v1 requests
// response_format=pcm).
func (f *ForgeClient) Speech(ctx context.Context, text, voice, responseFormat string) (contentType string, data []byte, err error) {
	payload, err := json.Marshal(map[string]interface{}{
		"model":           "kokoro",
		"input":           text,
		"voice":           voice,
		"response_format": responseFormat,
		"speed":           1,
	})
	if err != nil {
		return "", nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		f.rest.baseURL+"/v1/audio/speech", bytes.NewReader(payload))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.rest.apiKey)

	resp, err := f.audio.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("forge TTS request failed: %w", err)
	}
	defer resp.Body.Close()
	data, err = io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return "", nil, fmt.Errorf("failed to read forge TTS response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, fmt.Errorf("forge TTS /v1/audio/speech returned %d: %s", resp.StatusCode, truncateForError(string(data)))
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	return ct, data, nil
}
