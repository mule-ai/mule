package api

import (
	"fmt"
	"io"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/mule-ai/mule/pkg/types"
)

type Config struct {
	Enabled bool   `json:"enabled,omitempty"`
	Path    string `json:"path,omitempty"` // Root path for API endpoints (e.g. "/integration-api")
}

type API struct {
	Config       *Config
	logger       logr.Logger
	channel      chan any
	responseChan chan string
	triggers     map[string]chan any
}

func New(config *Config, logger logr.Logger) *API {
	// Set defaults
	if config.Path == "" {
		config.Path = "/integration-api"
	}

	api := &API{
		Config:       config,
		logger:       logger,
		channel:      make(chan any),
		triggers:     make(map[string]chan any),
		responseChan: make(chan string),
	}
	// Don't start a server - handlers will be registered with main server
	go api.receiveOutputs()
	logger.Info("API integration initialized - handlers will be registered with main server", "path", config.Path)
	return api
}

func (a *API) Call(name string, data any) (any, error) {
	return nil, nil
}

func (a *API) GetChannel() chan any {
	return a.channel
}

func (a *API) Name() string {
	return "api"
}

func (a *API) RegisterTrigger(trigger string, data any, channel chan any) {
	endpoint, ok := data.(string)
	if !ok {
		a.logger.Error(fmt.Errorf("data is not a string"), "Data is not a string")
		return
	}
	triggerName := trigger + "." + endpoint
	a.triggers[triggerName] = channel
	a.logger.Info("Registered trigger", "trigger", triggerName)
}

func (a *API) GetChatHistory(channelID string, limit int) (string, error) {
	return "", nil
}

func (a *API) ClearChatHistory(channelID string) error {
	return nil
}

// HandleAPI creates an api that accepts any path, query params, and body
// and returns a response
func (a *API) HandleAPI(w http.ResponseWriter, r *http.Request) {
	// get the path, query params, and body
	path := r.URL.Path
	queryParams := r.URL.Query()
	method := r.Method

	triggerName := method + "." + path
	trigger, ok := a.triggers[triggerName]
	if !ok {
		a.logger.Error(fmt.Errorf("api path received but no trigger registered"), "Trigger not found")
		http.Error(w, "unregistered endpoint", http.StatusNotFound)
		return
	}

	prompt := ""
	switch method {
	case "GET":
		prompt = queryParams.Get("data")
	case "POST":
		body, err := io.ReadAll(r.Body)
		if err != nil {
			a.logger.Error(err, "Failed to read body")
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		prompt = string(body)
	}
	trigger <- prompt

	response := <-a.responseChan
	_, err := w.Write([]byte(response + "\n"))
	if err != nil {
		a.logger.Error(err, "Failed to write response")
	}
}

func (a *API) receiveOutputs() {
	for trigger := range a.channel {
		triggerSettings, ok := trigger.(*types.TriggerSettings)
		if !ok {
			a.logger.Error(fmt.Errorf("trigger is not a Trigger"), "Trigger is not a Trigger")
			continue
		}
		if triggerSettings.Integration != "api" {
			a.logger.Error(fmt.Errorf("trigger integration is not api"), "Trigger integration is not api")
			continue
		}
		switch triggerSettings.Event {
		case "response":
			message, ok := triggerSettings.Data.(string)
			if !ok {
				a.logger.Error(fmt.Errorf("trigger data is not a string"), "Trigger data is not a string")
				continue
			}
			a.responseChan <- message
		default:
			a.logger.Error(fmt.Errorf("trigger event not supported: %s", triggerSettings.Event), "Unsupported trigger event")
		}
	}
}
