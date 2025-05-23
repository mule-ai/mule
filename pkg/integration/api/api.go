package api

import (
	"fmt"
	"io"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/mule-ai/mule/pkg/integration/types"
)

type Config struct {
	Port int `json:"port"`
}

type API struct {
	Config       *Config
	logger       logr.Logger
	channel      chan any
	responseChan chan string
	triggers     map[string]chan any
}

func New(config *Config, logger logr.Logger) *API {
	api := &API{
		Config:       config,
		logger:       logger,
		channel:      make(chan any),
		triggers:     make(map[string]chan any),
		responseChan: make(chan string),
	}
	go api.start()
	go api.receiveOutputs()
	return api
}

func (a *API) start() {
	http.HandleFunc("/api/", a.handleAPI)
	a.logger.Info("Starting API", "port", a.Config.Port)
	err := http.ListenAndServe(fmt.Sprintf(":%d", a.Config.Port), nil)
	if err != nil {
		a.logger.Error(err, "Failed to start API")
	}
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

// create an api that accepts any path, query params, and body
// and returns a response
func (a *API) handleAPI(w http.ResponseWriter, r *http.Request) {
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
