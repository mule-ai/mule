package system

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/jbutlerdev/genai"
	"github.com/mule-ai/mule/internal/scheduler"
	"github.com/mule-ai/mule/pkg/integration/types"
)

type Config struct {
	Timers map[string]string `json:"timers"`
}

type System struct {
	logger     logr.Logger
	scheduler  *scheduler.Scheduler
	channel    chan any
	config     *Config
	handlerMap map[string]func(data any) (any, error)
	providers  map[string]*genai.Provider
}

func New(config *Config, providers map[string]*genai.Provider, logger logr.Logger) *System {
	system := &System{
		logger:     logger,
		scheduler:  scheduler.NewScheduler(logger.WithName("system-integration-scheduler")),
		channel:    make(chan any),
		config:     config,
		handlerMap: make(map[string]func(data any) (any, error)),
		providers:  providers,
	}

	system.handlerMap["models"] = system.getModels
	system.handlerMap["providers"] = system.getProviders

	system.scheduler.Start()
	go system.receiveOutputs()
	logger.Info("System integration initialized")
	return system
}

func (s *System) Call(name string, data any) (any, error) {
	handler, ok := s.handlerMap[name]
	if !ok {
		return nil, fmt.Errorf("handler not found: %s", name)
	}
	return handler(data)
}

func (s *System) Name() string {
	return "system"
}

func (s *System) GetChannel() chan any {
	return s.channel
}

func (s *System) RegisterTrigger(trigger string, data any, channel chan any) {
	timer, ok := s.config.Timers[trigger]
	if !ok {
		s.logger.Error(fmt.Errorf("timer %s not found", trigger), "Timer not found")
		return
	}
	err := s.scheduler.AddTask(uuid.New().String(), timer, func() {
		channel <- data
	})
	if err != nil {
		s.logger.Error(err, "Failed to add trigger")
	}
	s.logger.Info("Registered trigger", "trigger", trigger, "timer", timer)
}

func (s *System) GetChatHistory(channelID string, limit int) (string, error) {
	return "", nil
}

func (s *System) ClearChatHistory(channelID string) error {
	return nil
}

func (s *System) receiveOutputs() {
	for trigger := range s.channel {
		triggerSettings, ok := trigger.(*types.TriggerSettings)
		if !ok {
			s.logger.Error(fmt.Errorf("trigger is not a Trigger"), "Trigger is not a Trigger")
			continue
		}
		switch triggerSettings.Event {
		case "models":
			response, err := s.getModels(triggerSettings.Data)
			if err != nil {
				s.logger.Error(err, "Failed to get models")
				continue
			}
			s.channel <- response
		default:
			s.logger.Error(fmt.Errorf("trigger event not supported: %s", triggerSettings.Event), "Unsupported trigger event")
		}
	}
}
