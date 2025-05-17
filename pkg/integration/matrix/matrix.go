package matrix

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/mule-ai/mule/pkg/integration/types"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type Config struct {
	Enabled          bool   `json:"enabled,omitempty"`
	MessageOnConnect bool   `json:"messageOnConnect,omitempty"`
	HomeServer       string `json:"homeServer,omitempty"`
	UserID           string `json:"userId,omitempty"`
	AccessToken      string `json:"accessToken,omitempty"`
	DeviceID         string `json:"deviceId,omitempty"`
	RecoveryKey      string `json:"recoveryKey,omitempty"`
	PickleKey        string `json:"pickleKey,omitempty"`
	RoomID           string `json:"roomId,omitempty"`
}

type Matrix struct {
	config                *Config
	l                     logr.Logger
	client                *mautrix.Client
	channel               chan any
	cryptoHelper          *cryptohelper.CryptoHelper
	requestedSessionMutex sync.Mutex
	requestedSessions     map[string]time.Time
	mentionRegex          *regexp.Regexp
	slashCommandRegex     *regexp.Regexp
	triggers              map[string]chan any
}

var (
	events = map[string]any{
		"newMessage":   struct{}{},
		"slashCommand": struct{}{},
	}
)

func New(config *Config, l logr.Logger) *Matrix {
	m := &Matrix{
		config:            config,
		l:                 l,
		channel:           make(chan any),
		requestedSessions: make(map[string]time.Time),
		triggers:          make(map[string]chan any),
	}
	m.mentionRegex = regexp.MustCompile(`\[([^\]]*)\]\(([^)]*)\)`)
	m.slashCommandRegex = regexp.MustCompile(`\/([a-zA-Z0-9_]+)`)
	m.init()
	go m.receiveTriggers()
	return m
}

func (m *Matrix) Call(name string, data any) (any, error) {
	return nil, nil
}

func (m *Matrix) Name() string {
	return "matrix"
}

func (m *Matrix) Send(message any) error {
	if !m.config.Enabled || m.client == nil {
		m.l.Info("Matrix integration is disabled or client is not initialized")
		return nil
	}

	// Convert message to string
	var body string
	switch msg := message.(type) {
	case string:
		body = msg
	case []byte:
		body = string(msg)
	default:
		// Try to marshal to JSON
		jsonBytes, err := json.Marshal(message)
		if err != nil {
			return fmt.Errorf("failed to marshal message to JSON: %w", err)
		}
		body = string(jsonBytes)
	}
	err := m.sendMessage(body)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	m.l.Info("Successfully sent message to Matrix room", "room_id", m.config.RoomID)
	return nil
}

func (m *Matrix) GetChannel() chan any {
	return m.channel
}

func (m *Matrix) RegisterTrigger(trigger string, data any, channel chan any) {
	_, ok := events[trigger]
	if !ok {
		m.l.Error(fmt.Errorf("trigger not found"), "Trigger not found")
		return
	}
	dataStr, ok := data.(string)
	if !ok {
		m.l.Error(fmt.Errorf("data is not a string"), "Data is not a string")
		return
	}
	m.triggers[trigger+dataStr] = channel
}

func (m *Matrix) init() {
	if !m.config.Enabled {
		m.l.Info("Matrix integration is disabled")
		return
	}

	if err := m.validateConfig(); err != nil {
		m.l.Error(err, "Invalid Matrix config")
		return
	}

	if err := m.connect(); err != nil {
		m.l.Error(err, "Failed to connect to Matrix")
		return
	}
}

func (m *Matrix) validateConfig() error {
	if m.config.HomeServer == "" {
		return fmt.Errorf("HomeServer is not set")
	}

	if m.config.UserID == "" {
		return fmt.Errorf("UserID is not set")
	}

	if m.config.AccessToken == "" {
		return fmt.Errorf("AccessToken is not set")
	}

	if m.config.DeviceID == "" {
		return fmt.Errorf("DeviceID is not set")
	}

	if m.config.RecoveryKey == "" {
		return fmt.Errorf("RecoveryKey is not set")
	}

	if m.config.PickleKey == "" {
		return fmt.Errorf("PickleKey is not set")
	}

	if m.config.RoomID == "" {
		return fmt.Errorf("RoomID is not set")
	}

	return nil
}

func (m *Matrix) setupCryptoHelper() (*cryptohelper.CryptoHelper, error) {
	// remember to use a secure key for the pickle key in production
	pickleKey := []byte(m.config.PickleKey)

	// this is a path to the SQLite database you will use to store various data about your bot
	dbPath := "crypto.db"

	helper, err := cryptohelper.NewCryptoHelper(m.client, pickleKey, dbPath)
	if err != nil {
		return nil, fmt.Errorf("NewCryptoHelper failed: %w", err)
	}

	// initialize the database and other stuff *before* returning
	m.l.Info("Initializing crypto helper database within setupCryptoHelper...")
	err = helper.Init(context.Background())
	if err != nil {
		return nil, fmt.Errorf("CryptoHelper Init failed: %w", err) // This might throw the "olm account not shared" error
	}
	m.l.Info("Crypto helper database initialized within setupCryptoHelper.")

	return helper, nil
}

func (m *Matrix) verifyWithRecoveryKey(machine *crypto.OlmMachine) (err error) {
	ctx := context.Background()

	// We'll skip the identity check for now and just proceed with verification

	m.l.Info("Getting default key data from SSSS...")
	keyId, keyData, err := machine.SSSS.GetDefaultKeyData(ctx)
	if err != nil {
		m.l.Error(err, "Failed to get default key data from SSSS")
		return err
	}

	m.l.Info("Verifying recovery key...")
	key, err := keyData.VerifyRecoveryKey(keyId, m.config.RecoveryKey)
	if err != nil {
		m.l.Error(err, "Failed to verify recovery key")
		return err
	}

	m.l.Info("Fetching cross-signing keys from SSSS...")
	err = machine.FetchCrossSigningKeysFromSSSS(ctx, key)
	if err != nil {
		m.l.Error(err, "Failed to fetch cross-signing keys from SSSS")
		return err
	}

	m.l.Info("Signing own device...")
	err = machine.SignOwnDevice(ctx, machine.OwnIdentity())
	if err != nil {
		m.l.Error(err, "Failed to sign own device")
		return err
	}

	m.l.Info("Signing own master key...")
	err = machine.SignOwnMasterKey(ctx)
	if err != nil {
		m.l.Error(err, "Failed to sign own master key")
		return err
	}

	m.l.Info("Device verification with recovery key completed successfully")
	return nil
}

// Helper function to track session ID requests and avoid requesting the same one repeatedly
func (m *Matrix) shouldRequestSession(sessionID string) bool {
	m.requestedSessionMutex.Lock()
	defer m.requestedSessionMutex.Unlock()

	// Check if we've requested this session recently (within 5 minutes)
	lastRequested, exists := m.requestedSessions[sessionID]
	if exists && time.Since(lastRequested) < 5*time.Minute {
		m.l.Info("Session was requested recently, not requesting again",
			"session_id", sessionID,
			"requested_at", lastRequested,
			"elapsed", time.Since(lastRequested))
		return false
	}

	// Mark this session as requested
	m.requestedSessions[sessionID] = time.Now()

	// Clean up old entries while we're here
	for sid, t := range m.requestedSessions {
		if time.Since(t) > 10*time.Minute {
			delete(m.requestedSessions, sid)
		}
	}

	return true
}

// Helper function to attempt decryption with better error handling
func (m *Matrix) attemptDecryption(ctx context.Context, evt *event.Event) (*event.Event, error) {
	// Try direct decryption
	if evt.Content.Parsed != nil {
		decrypted, err := m.cryptoHelper.Decrypt(ctx, evt)
		if err == nil {
			return decrypted, nil
		}

		// First attempt failed
		m.l.Info("Direct decryption failed, trying alternative method", "error", err.Error())

		// Request missing session if needed (operates on original evt for IDs)
		if enc, ok := evt.Content.Parsed.(*event.EncryptedEventContent); ok {
			if strings.Contains(err.Error(), "no session with given ID found") {
				m.l.Info("Missing session detected", "session_id", enc.SessionID, "room_id", evt.RoomID)
				if m.shouldRequestSession(string(enc.SessionID)) {
					m.l.Info("Requesting missing session", "session_id", enc.SessionID, "room_id", evt.RoomID)
					machine := m.cryptoHelper.Machine()
					requestID := fmt.Sprintf("%s-%s-%d", evt.RoomID, enc.SessionID, time.Now().UnixNano())
					if keyReqErr := machine.SendRoomKeyRequest(ctx, evt.RoomID, enc.SenderKey, enc.SessionID, requestID, nil); keyReqErr != nil {
						m.l.Error(keyReqErr, "Failed to request session")
					} else {
						m.l.Info("Session request sent", "request_id", requestID)
						waitCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
						defer cancel()
						m.l.Info("Waiting for session to arrive...", "session_id", enc.SessionID, "room_id", evt.RoomID)
						if found := machine.WaitForSession(waitCtx, evt.RoomID, enc.SenderKey, enc.SessionID, 3*time.Second); found {
							m.l.Info("Session arrived after waiting", "session_id", enc.SessionID)
							// Try decryption again with the new key (on original evt)
							if decryptedAfterSess, decryptErr := m.cryptoHelper.Decrypt(ctx, evt); decryptErr == nil {
								return decryptedAfterSess, nil
							} else {
								m.l.Error(decryptErr, "Failed to decrypt even after receiving session")
							}
						} else {
							m.l.Info("Session did not arrive within timeout", "session_id", enc.SessionID)
						}
					}
				}
			}
		}
	}

	// Content might not have been parsed correctly, or initial Parsed was nil.
	// Create a temporary event copy for re-parsing and decryption attempt to avoid modifying original evt on failure.
	evtCopy := *evt // Shallow copy the event struct

	mapBytes, err := json.Marshal(evtCopy.Content.Raw) // Use original Raw content
	if err != nil {
		return nil, fmt.Errorf("marshal content for re-parse failed: %w", err)
	}

	var encContent event.EncryptedEventContent
	if err := json.Unmarshal(mapBytes, &encContent); err != nil {
		return nil, fmt.Errorf("unmarshal content for re-parse failed: %w", err)
	}

	// Update parsed content on the temporary copy
	evtCopy.Content.Parsed = &encContent

	// Try decryption again using the copy with re-parsed content
	decrypted, err := m.cryptoHelper.Decrypt(ctx, &evtCopy)
	if err == nil {
		return decrypted, nil // Decrypt returns a new clone of evtCopy's state
	}

	// If we still can't decrypt, request the session key using details from encContent (derived from original evt.Content.Raw)
	m.l.Info("Missing session detected after reparsing", "session_id", encContent.SessionID, "room_id", evtCopy.RoomID /* or evt.RoomID */)

	if m.shouldRequestSession(string(encContent.SessionID)) {
		m.l.Info("Requesting missing session after reparsing", "session_id", encContent.SessionID, "room_id", evtCopy.RoomID)
		machine := m.cryptoHelper.Machine()
		requestID := fmt.Sprintf("%s-%s-%d", evtCopy.RoomID, encContent.SessionID, time.Now().UnixNano())

		if reqErr := machine.SendRoomKeyRequest(ctx, evtCopy.RoomID, encContent.SenderKey, encContent.SessionID, requestID, nil); reqErr != nil {
			m.l.Error(reqErr, "Failed to request session after reparsing")
		} else {
			m.l.Info("Session request sent after reparsing", "request_id", requestID)
			waitCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			m.l.Info("Waiting for session to arrive after reparsing...", "session_id", encContent.SessionID, "room_id", evtCopy.RoomID)
			if found := machine.WaitForSession(waitCtx, evtCopy.RoomID, encContent.SenderKey, encContent.SessionID, 3*time.Second); found {
				m.l.Info("Session arrived after waiting (post-reparse)", "session_id", encContent.SessionID)
				// Try decryption again with the new key, using the copy that has .Content.Parsed set
				if decryptedAfterSessReparse, decryptErr := m.cryptoHelper.Decrypt(ctx, &evtCopy); decryptErr == nil {
					return decryptedAfterSessReparse, nil
				} else {
					m.l.Error(decryptErr, "Failed to decrypt even after receiving session (post-reparse)")
				}
			} else {
				m.l.Info("Session did not arrive within timeout (post-reparse)", "session_id", encContent.SessionID)
			}
		}
	}
	// If all attempts failed, return the last error. Original 'evt' was not mutated if re-parsing path was taken and failed.
	return nil, err
}

func (m *Matrix) connect() error {
	client, err := mautrix.NewClient(m.config.HomeServer, id.UserID(m.config.UserID), m.config.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to create Matrix client: %w", err)
	}
	m.client = client
	client.DeviceID = id.DeviceID(m.config.DeviceID)

	syncer := mautrix.NewDefaultSyncer()
	client.Syncer = syncer

	// Register m.processEvent to handle all events dispatched by the syncer.
	// This includes timeline events and events decrypted from to-device messages.
	syncer.OnEvent(m.processEvent)

	// Setup crypto helper (includes Init call now)
	m.l.Info("Setting up crypto helper (incl. Init)...")
	cryptoHelper, err := m.setupCryptoHelper()
	if err != nil {
		return fmt.Errorf("failed to setup crypto helper: %w", err)
	}
	client.Crypto = cryptoHelper  // Assign to interface field
	m.cryptoHelper = cryptoHelper // Store the concrete type
	m.l.Info("Crypto helper setup complete.")

	// Setup sync handler
	readyChan := make(chan bool)
	var once sync.Once
	syncer.OnSync(func(ctx context.Context, resp *mautrix.RespSync, since string) bool {
		// Log when sync cycle completes, for debugging
		// m.l.Info("Sync loop completed", "since", since)

		// Signal readiness after the first successful sync
		once.Do(func() {
			// m.l.Info("Initial sync complete.")
			close(readyChan)
		})

		// First process to-device messages (which may contain room keys)
		// This needs to be done before handling room events as these may contain keys
		// needed to decrypt messages in rooms
		// The CryptoHelper will also process these via client.Crypto.ProcessSyncResponse.
		// This loop is primarily for detailed logging of raw to-device events.
		if len(resp.ToDevice.Events) > 0 {
			m.l.Info("Processing to-device events", "count", len(resp.ToDevice.Events))
			for i, evt := range resp.ToDevice.Events {
				m.l.Info("Processing to-device event",
					"index", i,
					"type", evt.Type,
					"sender", evt.Sender)

				// Handle room key events specially to improve logging
				if evt.Type == event.ToDeviceRoomKey {
					if key, ok := evt.Content.Parsed.(*event.RoomKeyEventContent); ok {
						m.l.Info("Received room key",
							"algorithm", key.Algorithm,
							"room_id", key.RoomID,
							"session_id", key.SessionID)
					}
				} else if evt.Type == event.ToDeviceForwardedRoomKey {
					if key, ok := evt.Content.Parsed.(*event.ForwardedRoomKeyEventContent); ok {
						m.l.Info("Received forwarded room key",
							"algorithm", key.Algorithm,
							"room_id", key.RoomID,
							"session_id", key.SessionID,
							"sender_key", key.SenderKey)
					}
				} else if evt.Type == event.ToDeviceRoomKeyRequest {
					if req, ok := evt.Content.Parsed.(*event.RoomKeyRequestEventContent); ok {
						m.l.Info("Received room key request",
							"request_id", req.RequestID,
							"action", req.Action)

						// Room key requests have different fields than what we expected
						// Just log what we have for now
						details, _ := json.Marshal(req)
						m.l.Info("Room key request details", "details", string(details))
					}
				}
				// The mautrix library's CryptoHelper will automatically process these to-device events
				// (including m.room.encrypted, keys, key requests) when client.Crypto.ProcessSyncResponse is called.
			}
		}

		// Timeline events from resp.Rooms.Join will be handled by the Syncer, which will then
		// call m.processEvent via the syncer.OnEvent registration made earlier.
		// Therefore, the explicit loop here to call m.processEvent is no longer needed.
		// m.l.Info("Sync response received. Joined room count for this sync", "joined_room_count", len(resp.Rooms.Join))

		// Return true to continue syncing
		return true
	})

	// Start syncing in background
	go func() {
		if err := client.Sync(); err != nil {
			m.l.Error(err, "Failed to sync")
		}
	}()

	// Wait for first sync to complete before proceeding
	m.l.Info("Waiting for initial sync to complete...")
	select {
	case <-readyChan:
		m.l.Info("Initial sync complete, proceeding with key operations")
	case <-time.After(30 * time.Second):
		m.l.Info("Sync timeout reached, proceeding anyway")
	}

	// Important: Share keys with the server to ensure device is known
	m.l.Info("Uploading device keys...")
	ctx := context.Background()
	machine := cryptoHelper.Machine()
	err = machine.ShareKeys(ctx, 0)
	if err != nil {
		m.l.Error(err, "Failed to share keys, but continuing...")
	}

	// Verify with recovery key after initial sync and key sharing
	m.l.Info("Attempting to verify with recovery key...")
	err = m.verifyWithRecoveryKey(machine)
	if err != nil {
		m.l.Error(err, "Failed with initial verification attempt, will retry after delay")

		// Wait a bit and retry once more
		time.Sleep(5 * time.Second)

		m.l.Info("Retrying key verification...")
		err = m.verifyWithRecoveryKey(machine)
		if err != nil {
			return fmt.Errorf("failed to verify with recovery key after retry: %w", err)
		}
	}
	m.l.Info("Key verification successful.")

	// Send test message
	if m.config.MessageOnConnect {
		err = m.sendMessage("Hello world from Mule!")
		if err != nil {
			return fmt.Errorf("failed to send test message: %w", err)
		}
	}

	return nil
}

func (m *Matrix) processEvent(ctx context.Context, evt *event.Event) {
	// Now only process messages from our configured room
	if evt.RoomID != id.RoomID(m.config.RoomID) {
		return
	}

	/*
		// Log that we are processing this specific event
		m.l.Info("Processing event",
			"room_id", evt.RoomID,
			"event_id", evt.ID,
			"event_type", evt.Type,
			"sender", evt.Sender)
	*/

	// var decrypted bool

	// Try to decrypt encrypted events
	if evt.Type == event.EventEncrypted {
		// m.l.Info("Found encrypted event", "event_id", evt.ID, "room_id", evt.RoomID)

		// Make sure the RoomID is set correctly before attempting decryption
		// This is important for key requests to work correctly
		if evt.RoomID == "" {
			// m.l.Info("Event had empty RoomID, setting from config for decryption", "event_id", evt.ID, "configured_room_id", m.config.RoomID)
			evt.RoomID = id.RoomID(m.config.RoomID)
		}

		decryptedEvt, err := m.attemptDecryption(ctx, evt)
		if err != nil {
			// Log error and continue with next event
			m.l.Error(err, "Failed to decrypt event", "event_id", evt.ID)

			// Try to extract encryption details to help with debugging
			if evt.Content.Parsed != nil {
				if enc, ok := evt.Content.Parsed.(*event.EncryptedEventContent); ok {
					m.l.Info("Encryption details on failed decrypt",
						"algorithm", enc.Algorithm,
						"sender_key", enc.SenderKey,
						"session_id", enc.SessionID,
						"event_id", evt.ID)
				}
			}
			return
		}

		// Successfully decrypted
		// m.l.Info("Event decrypted successfully", "event_id", evt.ID)
		*evt = *decryptedEvt
		// decrypted = true
	}

	// Process message events (both originally unencrypted and decrypted ones)
	if evt.Type == event.EventMessage {
		messageContent := evt.Content.AsMessage()
		if messageContent == nil {
			m.l.Info("Event type is EventMessage but content is not a valid message",
				"event_id", evt.ID,
				"room_id", evt.RoomID)
			return
		}

		mentionsMe := false
		// Super visible logging for received messages
		/*
			m.l.Info("=====================================")
			m.l.Info("=== MATRIX MESSAGE RECEIVED ===",
				"sender", evt.Sender,
				"room_id", evt.RoomID,
				"body", messageContent.Body,
				"msgtype", messageContent.MsgType,
				"event_id", evt.ID,
				"was_encrypted", decrypted,
			)
		*/

		// find mentions
		if messageContent.Mentions != nil {
			mentionedUsers := make([]string, len(messageContent.Mentions.UserIDs))
			for i, userID := range messageContent.Mentions.UserIDs {
				mentionedUsers[i] = string(userID)
				if userID == id.UserID(m.config.UserID) {
					mentionsMe = true
				}
			}
		}

		// m.l.Info("=====================================")

		if !mentionsMe {
			return
		}

		body := m.mentionRegex.ReplaceAllString(messageContent.Body, "$1")
		m.messageReceived(body)
	}
}

func (m *Matrix) sendMessage(message string) error {
	content := event.MessageEventContent{
		MsgType:       event.MsgText,
		Body:          message,
		Format:        event.FormatHTML,
		FormattedBody: FormatMessage(message),
	}
	_, err := m.client.SendMessageEvent(context.Background(), id.RoomID(m.config.RoomID), event.EventMessage, content)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

func (m *Matrix) messageReceived(message string) {
	// check for slash commands
	slashCommand := m.slashCommandRegex.FindStringSubmatch(message)
	if len(slashCommand) > 1 {
		for key := range m.triggers {
			cmd := strings.TrimPrefix(key, "slashCommand")
			if strings.Contains(message, cmd) {
				m.l.Info("Slash command received", "command", cmd)
				m.triggers[key] <- message
				return
			}
		}
		m.l.Info("Slash command recognized with no trigger, processing as chat message")
	}
	select {
	case m.triggers["newMessage"] <- message:
	default:
		m.l.Info("Channel full or not ready, discarding message", "message", message)
		err := m.sendMessage("Mule is busy, please try again later")
		if err != nil {
			m.l.Error(err, "Failed to send message")
		}
	}
}

func (m *Matrix) receiveTriggers() {
	for trigger := range m.channel {
		triggerSettings, ok := trigger.(*types.TriggerSettings)
		if !ok {
			m.l.Error(fmt.Errorf("trigger is not a Trigger"), "Trigger is not a Trigger")
			continue
		}
		if triggerSettings.Integration != "matrix" {
			m.l.Error(fmt.Errorf("trigger integration is not matrix"), "Trigger integration is not matrix")
			continue
		}
		switch triggerSettings.Event {
		case "sendMessage":
			message, ok := triggerSettings.Data.(string)
			if !ok {
				m.l.Error(fmt.Errorf("trigger data is not a string"), "Trigger data is not a string")
				continue
			}
			err := m.sendMessage(message)
			if err != nil {
				m.l.Error(err, "Failed to send message")
			}
		default:
			m.l.Error(fmt.Errorf("trigger event not supported"), "Trigger event not supported")
		}
	}
}
