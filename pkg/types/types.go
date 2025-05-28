package types

type TriggerSettings struct {
	Integration string `json:"integration"`
	Event       string `json:"event"`
	Data        any    `json:"data"`
}

type Integration interface {
	Call(name string, data any) (any, error)
	GetChannel() chan any
	Name() string
	RegisterTrigger(trigger string, data any, channel chan any)

	// Chat memory methods
	GetChatHistory(channelID string, limit int) (string, error)
	ClearChatHistory(channelID string) error
}
