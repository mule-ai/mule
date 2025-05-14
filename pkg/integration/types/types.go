package types

type TriggerSettings struct {
	Integration string `json:"integration"`
	Event       string `json:"event"`
	Data        any    `json:"data"`
}
