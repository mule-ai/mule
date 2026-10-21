package engine

// forge.go implements the `forge_conversation` workflow step type
// (Herd H3.2, docs/PLAN-HERD.md in the ranch repo): a mule job talks to
// a long-lived forge agent conversation by address (agent id or pinned
// conversation id), posts a templated message tagged with its mule job,
// and optionally waits for the forge turn to end, returning the turn's
// assistant text as the step output.
//
// Forge endpoints (read from forge's axum router; forge has NO /api/v1
// mount prefix):
//
//	GET  /agents/:id/active         -> {"busy": bool, "current_conversation": uuid|null}
//	POST /agents/:id/conversations  -> 201 {"session": {..., "id": uuid}, "working_dir": "..."}
//	POST /messages                  -> 202 {"message": {...}}   (body: {"session_id": uuid, "content": "..."})
//	GET  /messages?session_id=...   -> {"messages": [{id, sequence, role, content, ...}]}
//	GET  /sessions/:id/events?since=N&oneshot=true
//	                                  -> SSE: `message` rows, `turn_ended` {session_id},
//	                                     `heartbeat`, `lagged`; connection closes after
//	                                     turn_ended when oneshot=true
//
// v1 choices (documented in spec.md "Forge Conversation Step Type"):
//   - the forge api_key (sk_forge_...) is inlined in the step config; a
//     secrets table with api_key_ref is the documented upgrade path.
//   - mule-origin tagging is a plain content prefix `[mule job <job> <workflow>] `
//     because forge's POST /messages accepts no per-message provenance.
//   - modes steer/follow_up are mule-level labels: v1 posts the same
//     POST /messages; fork creates a sibling via POST
//     /agents/:id/conversations {fork_from}.
//   - wait:true uses the SSE stream as the primary turn-end signal
//     (`turn_ended`) with a messages-table fallback for the attach race.
//     On timeout the step FAILS with "timed out waiting for forge turn"
//     while the forge turn keeps running (the workflow gives up, the
//     agent does not).

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/internal/validation"
)

const (
	forgeRequestTimeout       = 30 * time.Second // per REST call
	forgeFallbackPollInterval = 2 * time.Second  // between fallback checks / reconnects
	forgeShortSSEPass         = 2 * time.Second  // final short SSE pass in the attach-race fallback
)

// forgeStepConfig is the parsed+validated form of a forge_conversation
// step's config JSONB.
type forgeStepConfig struct {
	ForgeURL       string
	APIKey         string
	AgentID        string
	ConversationID string
	Mode           string // default "message"
	Content        string
	Wait           bool
	TimeoutSecs    int // default 300
}

// parseForgeStepConfig extracts the step config from the raw JSONB map,
// applying defaults. Invalid configs return a combined validation error.
func parseForgeStepConfig(config map[string]interface{}) (forgeStepConfig, error) {
	if errs := validation.NewValidator().ValidateForgeConversationStep(config); len(errs) > 0 {
		return forgeStepConfig{}, fmt.Errorf("invalid forge_conversation config: %s", errs.Error())
	}
	var cfg forgeStepConfig
	cfg.ForgeURL = strings.TrimRight(configStringOrEmpty(config, "forge_url"), "/")
	cfg.APIKey = configStringOrEmpty(config, "api_key")
	cfg.AgentID = strings.TrimSpace(configStringOrEmpty(config, "agent_id"))
	cfg.ConversationID = strings.TrimSpace(configStringOrEmpty(config, "conversation_id"))
	cfg.Content = configStringOrEmpty(config, "content")
	cfg.Mode = "message"
	if m, ok := config["mode"].(string); ok && m != "" {
		cfg.Mode = m
	}
	if w, ok := config["wait"].(bool); ok {
		cfg.Wait = w
	}
	cfg.TimeoutSecs = 300
	if n, ok := configPositiveInt(config, "timeout_secs"); ok {
		cfg.TimeoutSecs = n
	}
	return cfg, nil
}

func configStringOrEmpty(config map[string]interface{}, key string) string {
	if config == nil {
		return ""
	}
	s, _ := config[key].(string)
	return s
}

func configPositiveInt(config map[string]interface{}, key string) (int, bool) {
	if config == nil {
		return 0, false
	}
	switch n := config[key].(type) {
	case float64:
		if n > 0 && n == float64(int64(n)) {
			return int(n), true
		}
	case int:
		if n > 0 {
			return n, true
		}
	case int64:
		if n > 0 {
			return int(n), true
		}
	}
	return 0, false
}

// renderTemplate renders a Go text/template over the job input map.
// Missing keys are an error (a typo in the step config should fail the
// step, not silently post empty content).
func renderTemplate(content string, data map[string]interface{}) (string, error) {
	tmpl, err := template.New("content").Option("missingkey=error").Parse(content)
	if err != nil {
		return "", fmt.Errorf("invalid content template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render content template: %w", err)
	}
	return buf.String(), nil
}

// forgeStepClient is a minimal forge REST + SSE client.
type forgeStepClient struct {
	baseURL string
	apiKey  string
	http    *http.Client // timeout used for REST calls
}

func newForgeStepClient(cfg forgeStepConfig) *forgeStepClient {
	return &forgeStepClient{
		baseURL: cfg.ForgeURL,
		apiKey:  cfg.APIKey,
		http:    &http.Client{Timeout: forgeRequestTimeout},
	}
}

// doRequest issues one authenticated REST call.
func (c *forgeStepClient) doRequest(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	var rd io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		rd = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("forge request %s %s failed: %w", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return fmt.Errorf("failed to read forge response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("forge %s %s returned %d: %s", method, path, resp.StatusCode, truncateForError(string(data)))
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("failed to parse forge %s %s response: %w", method, path, err)
		}
	}
	return nil
}

// agentActive is GET /agents/:id/active -> {"busy", "current_conversation"}.
func (c *forgeStepClient) agentActive(ctx context.Context, agentID string) (bool, string, error) {
	var out struct {
		CurrentConversation *string `json:"current_conversation"`
		Busy                bool    `json:"busy"`
	}
	if err := c.doRequest(ctx, http.MethodGet, "/agents/"+url.PathEscape(agentID)+"/active", nil, &out); err != nil {
		return false, "", err
	}
	if out.CurrentConversation == nil {
		return out.Busy, "", nil
	}
	return out.Busy, *out.CurrentConversation, nil
}

// createConversation is POST /agents/:id/conversations -> 201
// {"session": {..., "id"}, "working_dir"}. forkFrom may be empty.
func (c *forgeStepClient) createConversation(ctx context.Context, agentID, forkFrom string) (string, error) {
	body := map[string]interface{}{}
	if forkFrom != "" {
		body["fork_from"] = forkFrom
	}
	var out struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
	}
	if err := c.doRequest(ctx, http.MethodPost, "/agents/"+url.PathEscape(agentID)+"/conversations", body, &out); err != nil {
		return "", err
	}
	if out.Session.ID == "" {
		return "", fmt.Errorf("forge did not return a session id for the new conversation")
	}
	return out.Session.ID, nil
}

// forgeMessage mirrors forge's messages row (the fields mule consumes).
type forgeMessage struct {
	Sequence int    `json:"sequence"`
	Role     string `json:"role"`
	Content  string `json:"content"`
}

func (c *forgeStepClient) listMessages(ctx context.Context, sessionID string) ([]forgeMessage, error) {
	var out struct {
		Messages []forgeMessage `json:"messages"`
	}
	if err := c.doRequest(ctx, http.MethodGet, "/messages?session_id="+url.QueryEscape(sessionID), nil, &out); err != nil {
		return nil, err
	}
	return out.Messages, nil
}

// lastMessageSeq is GET /messages?session_id= -> max sequence (0 when the
// conversation has no messages yet).
func (c *forgeStepClient) lastMessageSeq(ctx context.Context, sessionID string) (int, error) {
	msgs, err := c.listMessages(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	max := 0
	for _, m := range msgs {
		if m.Sequence > max {
			max = m.Sequence
		}
	}
	return max, nil
}

// latestAssistantAfter returns the content of the most recent
// assistant row with sequence > afterSeq ("" when none). The content
// is thinking-stripped: reasoning models (Qwen3.8 et al.) emit
// chain-of-thought as inline <think>…</think> INSIDE the content text
// (pi projects it verbatim; it is not a separate thinking block), and
// a voice caller must never hear "The user sent … I should respond
// briefly." read aloud.
func (c *forgeStepClient) latestAssistantAfter(ctx context.Context, sessionID string, afterSeq int) (string, error) {
	msgs, err := c.listMessages(ctx, sessionID)
	if err != nil {
		return "", err
	}
	content := ""
	for _, m := range msgs {
		if m.Sequence > afterSeq && m.Role == "assistant" && m.Content != "" {
			content = m.Content
		}
	}
	return StripThinking(content), nil
}

// thinkRe matches an inline reasoning segment: <think>…</think> (the
// usual form, possibly multiline) or a dangling <think> with no
// closer — everything to EOF is reasoning then (a truncated turn).
var thinkRe = regexp.MustCompile(`(?s)<think>.*?</think>|<think>.*$`)

// StripThinking removes inline reasoning from assistant content and
// trims. Safe on clean text: it returns it unchanged.
//
// Form handled, in order:
//  1. <think>…</think> pairs (multiline) — removed.
//  2. A dangling <think> with no closer — everything from the opener
//     to EOF is reasoning (a truncated turn) — removed.
//  3. The template-opened form Qwen3.8 actually emits on the wire: a
//     reasoning PREFIX with no opening tag, closed by </think> — the
//     answer is everything after the LAST closer. Only applied when
//     no <think> opener is present (form 1 handled that case).
func StripThinking(content string) string {
	if content == "" {
		return ""
	}
	if strings.Contains(content, "<think>") {
		content = thinkRe.ReplaceAllString(content, "")
	} else if i := strings.LastIndex(content, "</think>"); i >= 0 {
		content = content[i+len("</think>"):]
	}
	return strings.TrimSpace(strings.TrimPrefix(content, "</think>"))
}

// postMessage is POST /messages {"session_id", "content"} -> 202.
func (c *forgeStepClient) postMessage(ctx context.Context, sessionID, content string) error {
	body := map[string]interface{}{"session_id": sessionID, "content": content}
	return c.doRequest(ctx, http.MethodPost, "/messages", body, nil)
}

// VoiceProfile is the slice of a forge profile the voice loop needs to
// re-point a conversation's model (GET /profiles).
type VoiceProfile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	BaseURL  string `json:"base_url"`
}

// listProfiles is GET /profiles (owner scope: the caller's own).
func (c *forgeStepClient) listProfiles(ctx context.Context) ([]VoiceProfile, error) {
	var out struct {
		Profiles []VoiceProfile `json:"profiles"`
	}
	if err := c.doRequest(ctx, http.MethodGet, "/profiles", nil, &out); err != nil {
		return nil, err
	}
	return out.Profiles, nil
}

// matchProfile resolves a user-chosen "voice model" label against the
// forge profiles: exact (case-insensitive) match on name, id, or
// model; then a unique-substring match on name (so "laguna" picks
// "Laguna (dev)"). An unknown label lists what IS available.
func (c *forgeStepClient) matchProfile(ctx context.Context, want string) (*VoiceProfile, error) {
	profiles, err := c.listProfiles(ctx)
	if err != nil {
		return nil, err
	}
	lc := strings.ToLower(strings.TrimSpace(want))
	if lc == "" {
		return nil, fmt.Errorf("empty voice model")
	}
	for _, p := range profiles {
		if strings.EqualFold(p.Name, lc) || strings.EqualFold(p.ID, lc) || strings.EqualFold(p.Model, lc) {
			pp := p
			return &pp, nil
		}
	}
	var hits []VoiceProfile
	var names []string
	for _, p := range profiles {
		names = append(names, p.Name)
		if strings.Contains(strings.ToLower(p.Name), lc) {
			hits = append(hits, p)
		}
	}
	if len(hits) == 1 {
		return &hits[0], nil
	}
	if len(hits) > 1 {
		return nil, fmt.Errorf("voice model %q is ambiguous (matches %d profiles)", want, len(hits))
	}
	return nil, fmt.Errorf("voice model %q not found (available: %s)", want, strings.Join(names, ", "))
}

// setModelOverrides is PATCH /sessions/{id} {provider, model,
// base_url} — forge's model switcher: the effective override rebinds
// the LIVE harness conversation via pi-durable setModel, effective
// from the next provider request. (api_key is deliberately untouched:
// forge redacts it in the API, and the lab's voice models — SGLang /
// vram llama.cpp — need none.)
func (c *forgeStepClient) setModelOverrides(ctx context.Context, sessionID, provider, model, baseURL string) error {
	body := map[string]interface{}{}
	if provider != "" {
		body["provider"] = provider
	}
	if model != "" {
		body["model"] = model
	}
	if baseURL != "" {
		body["base_url"] = baseURL
	}
	return c.doRequest(ctx, http.MethodPatch, "/sessions/"+sessionID, body, nil)
}

// sessionContext is GET /sessions/:id/context — the ACTIVE context
// (harness source: post-compaction chars; unmigrated sessions fall
// back to a chars/4 estimate over the messages table).
func (c *forgeStepClient) sessionContext(ctx context.Context, sessionID string) (SessionContext, error) {
	var out SessionContext
	if err := c.doRequest(ctx, http.MethodGet, "/sessions/"+sessionID+"/context", nil, &out); err != nil {
		return SessionContext{}, err
	}
	return out, nil
}

// compactSession is POST /sessions/:id/compact — submit a background
// compaction (the harness CompactionTask; summary lands at the next
// turn boundary at the latest).
func (c *forgeStepClient) compactSession(ctx context.Context, sessionID string) error {
	return c.doRequest(ctx, http.MethodPost, "/sessions/"+sessionID+"/compact", map[string]interface{}{}, nil)
}

// modelCatalog is GET /v1/models/catalog — the machine's models.json
// (id + contextWindow per model, secrets stripped).
func (c *forgeStepClient) modelCatalog(ctx context.Context) ([]CatalogModel, error) {
	var out struct {
		Providers map[string]struct {
			Models []CatalogModel `json:"models"`
		} `json:"providers"`
	}
	if err := c.doRequest(ctx, http.MethodGet, "/v1/models/catalog", nil, &out); err != nil {
		return nil, err
	}
	var models []CatalogModel
	for _, p := range out.Providers {
		models = append(models, p.Models...)
	}
	return models, nil
}

// waitForTurn blocks until the turn started after the given pre-turn
// sequence has ended, or ctx/deadline fires. It returns the pre-turn
// boundary: the caller fetches the turn's output as the latest
// assistant row with sequence greater than the returned value.
//
// Primary signal: the SSE stream at /sessions/:id/events
// (?since=&oneshot=true) — forge closes it after the `turn_ended`
// event. `message` events advance the since-point so a reconnect after
// a dropped connection resumes without replaying pre-turn history.
//
// Attach-race fallback: `turn_ended` is a LIVE bus event; if the turn
// ends before our SSE connection attaches, no event ever arrives. So
// when a stream attempt delivers no turn_ended, we consult the messages
// table: a post-turn assistant row means the turn produced its final
// answer (a turn still mid-tool-call shows tool rows, not a fresh
// assistant row). This is the SSE-vs-poll decision: SSE gives
// immediate, exact turn-end; the messages-table check only covers the
// attach race, never the happy path.
//
// On deadline the caller gets an error; the forge turn keeps running.
func (c *forgeStepClient) waitForTurn(ctx context.Context, sessionID string, since int, deadline time.Time) (int, error) {
	// boundary is the pre-turn high-water mark, kept separate from the
	// resume point below: it stays put so the caller can fetch the
	// turn's assistant row even after we observed it on the stream.
	boundary := since
	for {
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("timed out waiting for forge turn %s (the forge turn keeps running; the workflow gave up waiting)", sessionID)
		}
		gotEnd, finalSeq, streamErr := c.readTurnEvent(ctx, sessionID, since, deadline)
		if gotEnd {
			return boundary, nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return 0, fmt.Errorf("forge_conversation wait cancelled: %w", ctxErr)
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("timed out waiting for forge turn %s (the forge turn keeps running; the workflow gave up waiting)", sessionID)
		}
		if streamErr != nil {
			// Dropped connection: the read advanced the resume point as
			// far as it saw.
			since = maxInt(since, finalSeq)
		}

		content, err := c.latestAssistantAfter(ctx, sessionID, boundary)
		if err != nil {
			return 0, fmt.Errorf("failed to check forge messages while waiting for turn %s: %w", sessionID, err)
		}
		if content != "" {
			// Prefer a clean turn_ended on one short reconnect pass;
			// otherwise settle on the assistant row.
			if gotEnd, _, _ := c.readTurnEvent(ctx, sessionID, since, time.Now().Add(forgeShortSSEPass)); gotEnd {
				return boundary, nil
			}
			return boundary, nil
		}

		select {
		case <-ctx.Done():
			return 0, fmt.Errorf("forge_conversation wait cancelled: %w", ctx.Err())
		case <-time.After(forgeFallbackPollInterval):
		}
	}
}

// readTurnEvent streams the session's SSE until `turn_ended`, the
// connection closes, or ctx fires. It returns whether turn_ended was
// seen, the highest message sequence observed (for since tracking),
// and a stream error if the connection ended before turn_ended.
// until (zero value = no extra bound) caps the attempt with a derived
// context so the wait deadline is honored even while blocking on the
// stream body.
func (c *forgeStepClient) readTurnEvent(ctx context.Context, sessionID string, since int, until time.Time) (gotEnd bool, finalSeq int, streamErr error) {
	// Cap the attempt BEFORE building the request: the request's context
	// is what aborts the body read when the deadline fires.
	if !until.IsZero() && time.Until(until) > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, until)
		defer cancel()
	}
	u := fmt.Sprintf("/sessions/%s/events?since=%d&oneshot=true", url.PathEscape(sessionID), since)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+u, nil)
	if err != nil {
		return false, since, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "text/event-stream")

	// No client-level timeout for the stream; ctx + deadlines rule.
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return false, since, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return false, since, fmt.Errorf("forge SSE %s returned %d: %s", u, resp.StatusCode, truncateForError(string(body)))
	}

	lastSeq := since
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	eventName := ""
	var data bytes.Buffer

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			// Blank line delimits an SSE event: dispatch it.
			if eventName == "turn_ended" {
				return true, lastSeq, nil
			}
			eventName = ""
			data.Reset()
			continue
		}
		switch {
		case strings.HasPrefix(line, "event:"):
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data.WriteString(strings.TrimPrefix(line, "data:"))
		}
		if eventName == "message" && data.Len() > 0 {
			var m forgeMessage
			if jsonErr := json.Unmarshal([]byte(data.String()), &m); jsonErr == nil && m.Sequence > lastSeq {
				lastSeq = m.Sequence
			}
		}
	}
	streamErr = scanner.Err()
	if streamErr == nil {
		streamErr = io.EOF
	}
	// ctx cancellation surfaces as a body read error; re-wrap the
	// original context error so callers can tell.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return false, lastSeq, fmt.Errorf("forge SSE stream cancelled: %w", ctxErr)
	}
	return false, lastSeq, streamErr
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func truncateForError(s string) string {
	const max = 300
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

// ForgeSubmission is a forge-conversation address for posting a message
// OUTSIDE a workflow step — the wakes fire path (Herd H3.3) uses it.
// Same addressing shape as the forge_conversation step config (H3.2),
// minus content/wait/timeout: the caller supplies the content and the
// submission is fire-and-forget (no turn wait).
type ForgeSubmission struct {
	ForgeURL       string
	APIKey         string
	AgentID        string
	ConversationID string
}

// ParseForgeSubmission extracts and validates the submission config from
// a raw JSONB-style map ({forge_url, api_key, agent_id?, conversation_id?}).
func ParseForgeSubmission(cfg map[string]interface{}) (ForgeSubmission, error) {
	var s ForgeSubmission
	s.ForgeURL = strings.TrimRight(configStringOrEmpty(cfg, "forge_url"), "/")
	s.APIKey = configStringOrEmpty(cfg, "api_key")
	s.AgentID = strings.TrimSpace(configStringOrEmpty(cfg, "agent_id"))
	s.ConversationID = strings.TrimSpace(configStringOrEmpty(cfg, "conversation_id"))

	var errs []string
	if s.ForgeURL == "" {
		errs = append(errs, "forge_url is required")
	} else if u, err := url.Parse(s.ForgeURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errs = append(errs, "forge_url must be an http or https URL")
	}
	if s.APIKey == "" {
		errs = append(errs, "api_key is required")
	}
	if s.AgentID == "" && s.ConversationID == "" {
		errs = append(errs, "agent_id or conversation_id is required")
	}
	if len(errs) > 0 {
		return s, fmt.Errorf("invalid forge submission: %s", strings.Join(errs, "; "))
	}
	return s, nil
}

// PostMessage resolves the target conversation (pinned conversation_id
// > the agent's active conversation > a newly created conversation) and
// posts content to it. Fire-and-forget: it does NOT wait for the turn
// to end. Returns the conversation id the message was posted to.
func (s ForgeSubmission) PostMessage(ctx context.Context, content string) (string, error) {
	client := newForgeStepClient(forgeStepConfig{ForgeURL: s.ForgeURL, APIKey: s.APIKey})

	sessionID := s.ConversationID
	if sessionID == "" {
		if s.AgentID == "" {
			return "", fmt.Errorf("forge submission requires agent_id or conversation_id")
		}
		_, active, activeErr := client.agentActive(ctx, s.AgentID)
		if activeErr == nil && active != "" {
			sessionID = active
		} else {
			sessionID, err := client.createConversation(ctx, s.AgentID, "")
			if err != nil {
				if activeErr != nil {
					return "", fmt.Errorf("failed to resolve forge agent %s: %w", s.AgentID, activeErr)
				}
				return "", fmt.Errorf("failed to create forge conversation: %w", err)
			}
			return sessionID, s.postTo(ctx, client, sessionID, content)
		}
	}
	if err := s.postTo(ctx, client, sessionID, content); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (s ForgeSubmission) postTo(ctx context.Context, client *forgeStepClient, sessionID, content string) error {
	if err := client.postMessage(ctx, sessionID, content); err != nil {
		return fmt.Errorf("failed to post to forge conversation %s: %w", sessionID, err)
	}
	return nil
}

// processForgeConversationStep executes a forge_conversation step.
//
// Addressing resolution order:
//  1. pinned conversation_id wins;
//  2. mode "fork": agent_id required -> POST /agents/:id/conversations
//     {fork_from: <current active or pinned conversation>};
//  3. agent_id: GET /agents/:id/active -> current_conversation; when
//     the agent has none, create one via POST /agents/:id/conversations.
//
// Output: {"forge_conversation": <session id>, "output": <text>} where
// text is the turn's final assistant message (wait:true) or the posted
// content (wait:false).
func (e *Engine) processForgeConversationStep(ctx context.Context, step *primitive.WorkflowStep, jobID string, inputData map[string]interface{}, workingDir string) (map[string]interface{}, error) {
	// Check for context cancellation before processing
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("forge_conversation step cancelled: %w", ctx.Err())
	default:
	}

	cfg, err := parseForgeStepConfig(step.Config)
	if err != nil {
		return nil, err
	}

	client := newForgeStepClient(cfg)

	// Render the content template over the job input map.
	rendered, err := renderTemplate(cfg.Content, inputData)
	if err != nil {
		return nil, err
	}

	// Resolve the target conversation.
	sessionID := cfg.ConversationID
	if cfg.Mode == "fork" {
		if cfg.AgentID == "" {
			return nil, fmt.Errorf("mode fork requires agent_id")
		}
		var forkFrom string
		if sessionID != "" {
			forkFrom = sessionID
		} else {
			if _, active, activeErr := client.agentActive(ctx, cfg.AgentID); activeErr == nil {
				forkFrom = active
			}
		}
		sessionID, err = client.createConversation(ctx, cfg.AgentID, forkFrom)
		if err != nil {
			return nil, fmt.Errorf("failed to fork forge conversation: %w", err)
		}
	} else if sessionID == "" {
		if cfg.AgentID == "" {
			return nil, fmt.Errorf("either conversation_id or agent_id is required")
		}
		_, active, activeErr := client.agentActive(ctx, cfg.AgentID)
		if activeErr == nil && active != "" {
			sessionID = active
		} else {
			// Agent has no conversation yet: create one.
			sessionID, err = client.createConversation(ctx, cfg.AgentID, "")
			if err != nil {
				return nil, fmt.Errorf("failed to create forge conversation: %w", err)
			}
		}
	}

	// Tag the message with its mule origin. Forge's POST /messages
	// accepts plain content only (no provenance field), so v1 prepends
	// a mule-job-tagged prefix (documented in spec.md).
	tagged := fmt.Sprintf("[mule job %s %s] %s", jobID, step.WorkflowID, rendered)

	// Snapshot the conversation's high-water mark BEFORE posting so
	// the turn-output fetch (and the SSE since-point) can distinguish
	// the turn we just triggered from history.
	lastSeq, err := client.lastMessageSeq(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list forge messages for %s: %w", sessionID, err)
	}

	if err := client.postMessage(ctx, sessionID, tagged); err != nil {
		return nil, fmt.Errorf("failed to post message to forge conversation %s: %w", sessionID, err)
	}

	if !cfg.Wait {
		return map[string]interface{}{
			"forge_conversation": sessionID,
			"output":             tagged,
		}, nil
	}

	deadline := time.Now().Add(time.Duration(cfg.TimeoutSecs) * time.Second)
	waitSeq, err := client.waitForTurn(ctx, sessionID, lastSeq, deadline)
	if err != nil {
		return nil, err
	}

	assistant, err := client.latestAssistantAfter(ctx, sessionID, waitSeq)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch the turn's assistant output for %s: %w", sessionID, err)
	}
	if assistant == "" {
		return nil, fmt.Errorf("forge turn in conversation %s ended without an assistant reply", sessionID)
	}

	return map[string]interface{}{
		"forge_conversation": sessionID,
		"output":             assistant,
	}, nil
}
