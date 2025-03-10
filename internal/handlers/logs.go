package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mule-ai/mule/pkg/log"
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
	limitStr := r.URL.Query().Get("limit")
	isAjax := r.Header.Get("X-Requested-With") == "XMLHttpRequest"

	// Parse limit parameter, default to 10 if not specified or invalid
	limit := 10
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil {
			limit = parsedLimit
		}
	}

	// Read and parse log file
	file, err := os.Open(log.LogFile)
	if err != nil {
		errString := fmt.Sprintf("Error reading log file: %v", err)
		if isAjax {
			http.Error(w, `{"error": "`+errString+`"}`, http.StatusInternalServerError)
		} else {
			http.Error(w, errString, http.StatusInternalServerError)
		}
		return
	}
	defer file.Close()

	// Map to store conversations by ID
	conversations := make(map[string]*Conversation)
	reader := bufio.NewReader(file)

	const maxLineLength = 1024 * 1024 // 1MB

	for {
		// ReadLine returns line, isPrefix, error
		var fullLine []byte
		isLineTooLong := false
		for {
			line, isPrefix, err := reader.ReadLine()
			if err != nil {
				if err.Error() == "EOF" {
					break
				}
				errString := fmt.Sprintf("Error reading line: %v", err)
				if isAjax {
					http.Error(w, `{"error": "`+errString+`"}`, http.StatusInternalServerError)
				} else {
					http.Error(w, errString, http.StatusInternalServerError)
				}
				return
			}

			fullLine = append(fullLine, line...)
			if len(fullLine) > maxLineLength {
				isLineTooLong = true
				// Discard the rest of the line
				for isPrefix {
					_, isPrefix, err = reader.ReadLine()
					if err != nil {
						if err.Error() == "EOF" {
							break
						}
						errString := fmt.Sprintf("Error reading line: %v", err)
						if isAjax {
							http.Error(w, `{"error": "`+errString+`"}`, http.StatusInternalServerError)
						} else {
							http.Error(w, errString, http.StatusInternalServerError)
						}
						return
					}
				}
				break
			}
			if !isPrefix {
				break
			}
		}

		// Break the outer loop if we've reached EOF
		if len(fullLine) == 0 {
			break
		}

		var entry LogEntry
		if isLineTooLong {
			// Try to parse the JSON we have to get the metadata
			if err := json.Unmarshal(fullLine[:maxLineLength], &entry); err != nil {
				continue // Skip if we can't even parse the metadata
			}
			// Replace the content with a message about the length
			entry.Content = "[Content exceeds 1MB and has been truncated]"
		} else {
			if err := json.Unmarshal(fullLine, &entry); err != nil {
				continue // Skip invalid JSON entries
			}
		}
		entry.Time = time.Unix(int64(entry.TimeStamp), 0)

		// Apply filters
		if level != "" && !strings.EqualFold(entry.Level, level) {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(entry.Message), strings.ToLower(search)) {
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

	// Apply conversation limit if greater than 0 (0 means no limit)
	if limit > 0 && len(sortedConversations) > limit {
		sortedConversations = sortedConversations[:limit]
	}

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
