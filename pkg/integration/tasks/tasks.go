package tasks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
)

// Note defines the structure for a note.
type Note struct {
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Task defines the structure for a task.
type Task struct {
	CreatedAt   time.Time  `json:"created_at"`
	Description string     `json:"description,omitempty"`
	DueDate     *time.Time `json:"due_date,omitempty"`
	ID          string     `json:"id"`
	ListID      string     `json:"list_id"`
	Notes       []Note     `json:"notes,omitempty"`
	State       string     `json:"state"`
	StateTime   time.Time  `json:"state_time"`
	SubTasks    []Task     `json:"sub_tasks,omitempty"`
	Title       string     `json:"title"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TaskList defines the structure for a task list.
type TaskList struct {
	CreatedAt   time.Time `json:"created_at"`
	Description string    `json:"description,omitempty"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Config holds the configuration for the Tasks integration.
type Config struct {
	Enabled bool   `json:"enabled,omitempty"`
	APIURL  string `json:"apiUrl,omitempty"` // Base URL for the tasks API
}

// Tasks integration implementation.
type Tasks struct {
	config     *Config
	l          logr.Logger
	channel    chan any
	client     *http.Client
	handlerMap map[string]func(data any) (any, error)
}

// New creates a new Tasks integration.
func New(config *Config, l logr.Logger) *Tasks {
	t := &Tasks{
		config:     config,
		l:          l,
		channel:    make(chan any),
		client:     &http.Client{Timeout: 10 * time.Second},
		handlerMap: make(map[string]func(data any) (any, error)),
	}

	t.handlerMap["getTasks"] = t.GetAllTasks

	t.init()
	return t
}

func (t *Tasks) init() {
	if !t.config.Enabled {
		t.l.Info("Tasks integration is disabled")
		return
	}
	if t.config.APIURL == "" {
		t.l.Error(fmt.Errorf("APIURL is not set for Tasks integration"), "APIURL is not set")
		// Potentially disable the integration or handle this error more gracefully
		return
	}
	t.l.Info("Tasks integration initialized", "apiUrl", t.config.APIURL)
}

func (t *Tasks) Call(name string, data any) (any, error) {
	handler, ok := t.handlerMap[name]
	if !ok {
		return nil, fmt.Errorf("handler not found for %s", name)
	}
	return handler(data)
}

// Name returns the name of the integration.
func (t *Tasks) Name() string {
	return "tasks"
}

// GetChannel returns the communication channel for the integration.
func (t *Tasks) GetChannel() chan any {
	return t.channel
}

// RegisterTrigger is a placeholder for registering triggers.
func (t *Tasks) RegisterTrigger(trigger string, data any, channel chan any) {
	t.l.Info("RegisterTrigger method called, but not implemented for tasks integration", "trigger", trigger)
}

// GetAllTasks fetches all tasks from the API.
func (t *Tasks) GetAllTasks(data any) (any, error) {
	if !t.config.Enabled || t.config.APIURL == "" {
		return nil, fmt.Errorf("tasks integration is disabled or APIURL is not configured")
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/tasks", t.config.APIURL), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get tasks, status code: %d", resp.StatusCode)
	}

	var tasks []Task
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return nil, fmt.Errorf("failed to decode tasks response: %w", err)
	}

	t.l.Info("Successfully fetched all tasks", "count", len(tasks))
	// return should be any but castable to string
	jsonTasks, err := json.Marshal(tasks)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tasks: %w", err)
	}

	// set data
	dataString, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("data is not a string")
	}
	if dataString == "" {
		dataString = "Show me all tasks as a markdown table.\n" +
			"Do not include any tasks where the state is done. Do not show the ID in the table, make sure to include the Title, Description, State, and Due Date. \n" +
			"Do not include any tasks where the due date is in the past. \n" +
			"Make the Due Date as human readable as possible."
	}
	return string(jsonTasks) + "\n" + dataString, nil
}
