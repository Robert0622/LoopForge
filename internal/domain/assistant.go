package domain

import "time"

type AssistantMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AssistantSession struct {
	ID        string             `json:"id"`
	UserID    int64              `json:"userId,omitempty"`
	Title     string             `json:"title"`
	ProjectID string             `json:"projectId,omitempty"`
	Model     string             `json:"model,omitempty"`
	Messages  []AssistantMessage `json:"messages,omitempty"`
	CreatedAt time.Time          `json:"createdAt,omitempty"`
	UpdatedAt time.Time          `json:"updatedAt"`
}

type AssistantTask struct {
	ID          string    `json:"id"`
	UserID      int64     `json:"userId,omitempty"`
	Name        string    `json:"name"`
	Prompt      string    `json:"prompt"`
	Model       string    `json:"model,omitempty"`
	ProjectID   string    `json:"projectId,omitempty"`
	SessionID   string    `json:"sessionId,omitempty"`
	Cron        string    `json:"cron,omitempty"`
	IntervalSec int       `json:"intervalSec,omitempty"`
	Enabled     bool      `json:"enabled"`
	LastStatus  string    `json:"lastStatus,omitempty"`
	LastError   string    `json:"lastError,omitempty"`
	LastMessage string    `json:"lastMessage,omitempty"`
	LastRunAt   time.Time `json:"lastRunAt,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
