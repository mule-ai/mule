package system

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/mule-ai/mule/internal/scheduler"
)

type Config struct {
	Timers map[string]string `json:"timers"`
}

type System struct {
	logger    logr.Logger
	scheduler *scheduler.Scheduler
	channel   chan any
	config    *Config
}

func New(config *Config, logger logr.Logger) *System {
	system := &System{
		logger:    logger,
		scheduler: scheduler.NewScheduler(logger.WithName("system-integration-scheduler")),
		channel:   make(chan any),
		config:    config,
	}
	system.scheduler.Start()
	logger.Info("System integration initialized")
	return system
}

func (s *System) Call(name string, data any) (any, error) {
	return nil, nil
}

func (s *System) Name() string {
	return "system"
}

func (s *System) Send(message any) error {
	s.channel <- message
	return nil
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
