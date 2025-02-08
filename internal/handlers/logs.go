package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type LogEntry struct {
	Level     string  `json:"level"`
	TimeStamp float64 `json:"ts"`
	Time      time.Time
	Logger    string `json:"logger"`
	Caller    string `json:"caller"`
	Message   string `json:"msg"`
	Content   string `json:"content,omitempty"`
	Model     string `json:"model,omitempty"`
	Error     string `json:"error,omitempty"`
	ID        string `json:"id,omitempty"`
}

type Conversation struct {
	ID           string
	StartTime    time.Time
	Messages     []LogEntry
	MessageCount int
	Status       string // Status based on last message level
}

type LogsData struct {
	Page          string
	Conversations []Conversation
}

func HandleLogs(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	level := r.URL.Query().Get("level")
	isAjax := r.Header.Get("X-Requested-With") == "XMLHttpRequest"

	// Read and parse log file
	file, err := os.Open("dev-team.log")
	if err != nil {
		if isAjax {
			http.Error(w, `{"error": "Error reading log file"}`, http.StatusInternalServerError)
		} else {
			http.Error(w, "Error reading log file", http.StatusInternalServerError)
		}
		return
	}
	defer file.Close()

	// Map to store conversations by ID
	conversations := make(map[string]*Conversation)
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		var entry LogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			log.Printf("Error unmarshalling log entry: %v", err)
			log.Printf("Log entry: %v", fmt.Sprintf("%v", string(scanner.Bytes())))
			continue // Skip invalid JSON entries
		}
		entry.Time = time.Unix(int64(entry.TimeStamp), 0)

		// Apply filters
		if level != "" && !strings.EqualFold(entry.Level, level) {
			log.Printf("Skipping log entry due to level: %v", entry)
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(entry.Message), strings.ToLower(search)) {
			log.Printf("Skipping log entry due to search: %v", entry)
			continue
		}

		// Group by conversation ID
		if entry.ID != "" {
			if conv, exists := conversations[entry.ID]; exists {
				conv.Messages = append(conv.Messages, entry)
				conv.MessageCount++
			} else {
				conversations[entry.ID] = &Conversation{
					ID:           entry.ID,
					StartTime:    entry.Time,
					Messages:     []LogEntry{entry},
					MessageCount: 1,
				}
			}
		}
	}
	err = scanner.Err()
	if err != nil {
		log.Printf("Error scanning log file: %v", err)
	}

	// Convert map to slice and sort by start time
	var sortedConversations []Conversation
	for _, conv := range conversations {
		// Sort messages within conversation by timestamp
		sort.Slice(conv.Messages, func(i, j int) bool {
			return conv.Messages[i].Time.Before(conv.Messages[j].Time)
		})

		// Set status based on last message level
		if len(conv.Messages) > 0 {
			conv.Status = conv.Messages[len(conv.Messages)-1].Level
		}

		sortedConversations = append(sortedConversations, *conv)
	}

	// Sort conversations by start time, most recent first
	sort.Slice(sortedConversations, func(i, j int) bool {
		return sortedConversations[i].StartTime.After(sortedConversations[j].StartTime)
	})

	if isAjax {
		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(sortedConversations)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	data := LogsData{
		Page:          "logs",
		Conversations: sortedConversations,
	}

	err = templates.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
