package repo

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
)

const assistantSessionListLimit = 40

func assistantRecordID(id string) string {
	id = filepath.Base(strings.TrimSpace(id))
	if id == "" || id == "." || id == ".." {
		return ""
	}
	return id
}

func (r *AssistantRepo) AssistantSession(userID int64, id string) (domain.AssistantSession, bool) {
	id = assistantRecordID(id)
	if userID <= 0 || id == "" {
		return domain.AssistantSession{}, false
	}
	var row assistantSessionRow
	if err := r.DB.Table("assistant_sessions").Where("id = ? AND user_id = ?", id, userID).Take(&row).Error; err != nil {
		return domain.AssistantSession{}, false
	}
	return decodeAssistantSession(row), true
}

func (r *AssistantRepo) ListAssistantSessions(userID int64) []domain.AssistantSession {
	if userID <= 0 {
		return []domain.AssistantSession{}
	}
	var rows []assistantSessionRow
	if err := r.DB.Table("assistant_sessions").Where("user_id = ?", userID).Order("updated_at DESC").Limit(assistantSessionListLimit).Find(&rows).Error; err != nil {
		return []domain.AssistantSession{}
	}
	out := make([]domain.AssistantSession, 0, len(rows))
	for _, row := range rows {
		item := decodeAssistantSession(row)
		item.Messages = nil
		out = append(out, item)
	}
	return out
}

func (r *AssistantRepo) PutAssistantSession(userID int64, v domain.AssistantSession) error {
	if userID <= 0 {
		return errors.New("assistant session user is required")
	}
	v.ID = assistantRecordID(v.ID)
	if v.ID == "" {
		return errors.New("assistant session id is required")
	}
	if existing, ok := r.sessionByID(v.ID); ok && existing.UserID != userID {
		return errors.New("assistant session belongs to another user")
	}
	now := time.Now().UTC()
	if old, ok := r.AssistantSession(userID, v.ID); ok && !old.CreatedAt.IsZero() {
		v.CreatedAt = old.CreatedAt
	} else if v.CreatedAt.IsZero() {
		v.CreatedAt = now
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = now
	}
	v.UserID = userID
	return upsert(r.DB, "assistant_sessions", map[string]any{
		"id":         v.ID,
		"user_id":    userID,
		"project_id": strings.TrimSpace(v.ProjectID),
		"title":      strings.TrimSpace(v.Title),
		"model":      strings.TrimSpace(v.Model),
		"payload":    mustJSON(v),
		"created_at": timePtr(v.CreatedAt),
		"updated_at": timePtr(v.UpdatedAt),
	})
}

func (r *AssistantRepo) DeleteAssistantSession(userID int64, id string) error {
	id = assistantRecordID(id)
	if userID <= 0 || id == "" {
		return fmt.Errorf("assistant session not found")
	}
	res := r.DB.Table("assistant_sessions").Where("id = ? AND user_id = ?", id, userID).Delete(nil)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("assistant session not found")
	}
	return nil
}

func (r *AssistantRepo) AssistantTask(userID int64, id string) (domain.AssistantTask, bool) {
	id = assistantRecordID(id)
	if userID <= 0 || id == "" {
		return domain.AssistantTask{}, false
	}
	var row assistantTaskRow
	if err := r.DB.Table("assistant_tasks").Where("id = ? AND user_id = ?", id, userID).Take(&row).Error; err != nil {
		return domain.AssistantTask{}, false
	}
	return decodeAssistantTask(row), true
}

func (r *AssistantRepo) ListAssistantTasks(userID int64) []domain.AssistantTask {
	if userID <= 0 {
		return []domain.AssistantTask{}
	}
	var rows []assistantTaskRow
	if err := r.DB.Table("assistant_tasks").Where("user_id = ?", userID).Find(&rows).Error; err != nil {
		return []domain.AssistantTask{}
	}
	out := make([]domain.AssistantTask, 0, len(rows))
	for _, row := range rows {
		out = append(out, decodeAssistantTask(row))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

func (r *AssistantRepo) EnabledAssistantTasks() []domain.AssistantTask {
	var rows []assistantTaskRow
	if err := r.DB.Table("assistant_tasks").Where("enabled = ?", 1).Find(&rows).Error; err != nil {
		return []domain.AssistantTask{}
	}
	out := make([]domain.AssistantTask, 0, len(rows))
	for _, row := range rows {
		out = append(out, decodeAssistantTask(row))
	}
	return out
}

func (r *AssistantRepo) PutAssistantTask(userID int64, v domain.AssistantTask) error {
	if userID <= 0 {
		return errors.New("assistant task user is required")
	}
	v.ID = assistantRecordID(v.ID)
	if v.ID == "" {
		return errors.New("assistant task id is required")
	}
	if existing, ok := r.taskByID(v.ID); ok && existing.UserID != userID {
		return errors.New("assistant task belongs to another user")
	}
	now := time.Now().UTC()
	if old, ok := r.AssistantTask(userID, v.ID); ok && !old.CreatedAt.IsZero() {
		v.CreatedAt = old.CreatedAt
	} else if v.CreatedAt.IsZero() {
		v.CreatedAt = now
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = now
	}
	v.UserID = userID
	enabled := 0
	if v.Enabled {
		enabled = 1
	}
	return upsert(r.DB, "assistant_tasks", map[string]any{
		"id":           v.ID,
		"user_id":      userID,
		"project_id":   strings.TrimSpace(v.ProjectID),
		"name":         strings.TrimSpace(v.Name),
		"enabled":      enabled,
		"interval_sec": v.IntervalSec,
		"last_run_at":  timePtr(v.LastRunAt),
		"payload":      mustJSON(v),
		"created_at":   timePtr(v.CreatedAt),
		"updated_at":   timePtr(v.UpdatedAt),
	})
}

func (r *AssistantRepo) DeleteAssistantTask(userID int64, id string) error {
	id = assistantRecordID(id)
	if userID <= 0 || id == "" {
		return fmt.Errorf("assistant task not found")
	}
	res := r.DB.Table("assistant_tasks").Where("id = ? AND user_id = ?", id, userID).Delete(nil)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("assistant task not found")
	}
	return nil
}

func (r *AssistantRepo) sessionByID(id string) (domain.AssistantSession, bool) {
	var row assistantSessionRow
	if err := r.DB.Table("assistant_sessions").Where("id = ?", id).Take(&row).Error; err != nil {
		return domain.AssistantSession{}, false
	}
	return decodeAssistantSession(row), true
}

func (r *AssistantRepo) taskByID(id string) (domain.AssistantTask, bool) {
	var row assistantTaskRow
	if err := r.DB.Table("assistant_tasks").Where("id = ?", id).Take(&row).Error; err != nil {
		return domain.AssistantTask{}, false
	}
	return decodeAssistantTask(row), true
}

func decodeAssistantSession(row assistantSessionRow) domain.AssistantSession {
	var item domain.AssistantSession
	_ = json.Unmarshal(row.Payload, &item)
	item.ID = row.ID
	item.UserID = row.UserID
	if item.ProjectID == "" {
		item.ProjectID = row.ProjectID
	}
	if item.Title == "" {
		item.Title = row.Title
	}
	if item.Model == "" {
		item.Model = row.Model
	}
	if item.CreatedAt.IsZero() && row.Created != nil {
		item.CreatedAt = row.Created.UTC()
	}
	if row.Updated != nil {
		item.UpdatedAt = row.Updated.UTC()
	}
	return item
}

func decodeAssistantTask(row assistantTaskRow) domain.AssistantTask {
	var item domain.AssistantTask
	_ = json.Unmarshal(row.Payload, &item)
	item.ID = row.ID
	item.UserID = row.UserID
	if item.ProjectID == "" {
		item.ProjectID = row.ProjectID
	}
	if item.Name == "" {
		item.Name = row.Name
	}
	if item.IntervalSec == 0 {
		item.IntervalSec = row.IntervalSec
	}
	item.Enabled = row.Enabled == 1
	if item.LastRunAt.IsZero() && row.LastRunAt != nil {
		item.LastRunAt = row.LastRunAt.UTC()
	}
	if item.CreatedAt.IsZero() && row.Created != nil {
		item.CreatedAt = row.Created.UTC()
	}
	if row.Updated != nil {
		item.UpdatedAt = row.Updated.UTC()
	}
	return item
}
