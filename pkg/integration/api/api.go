package api

import (
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
)

type Config struct {
	Port int `json:"port"`
}

type API struct {
	Config   *Config
	logger   logr.Logger
	channel  chan any
	triggers map[string]chan any
}

func New(config *Config, logger logr.Logger) *API {
	api := &API{
		Config:   config,
		logger:   logger,
		channel:  make(chan any),
		triggers: make(map[string]chan any),
	}
	go api.start()
	return api
}

func (a *API) start() {
	http.HandleFunc("/", a.handleAPI)
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

func (a *API) Send(message any) error {
	a.channel <- message
	return nil
}

func (a *API) RegisterTrigger(trigger string, data any, channel chan any) {
	a.triggers[trigger] = channel
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
	body := r.Body

	// return the path, query params, and body
	_, err := w.Write([]byte(fmt.Sprintf("Path: %s, Query Params: %v, Body: %v", path, queryParams, body)))
	if err != nil {
		a.logger.Error(err, "Failed to write response")
	}
}
