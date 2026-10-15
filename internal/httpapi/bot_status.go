package httpapi

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/gin-gonic/gin"
)

// botStatusView is the heartbeat-ish snapshot the room polls at most every 30s.
// Status is online, busy (with queue), degraded, or unavailable (with a reason).
type botStatusView struct {
	AgentID     string    `json:"agentId"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Queue       int       `json:"queue"`
	Reason      string    `json:"reason,omitempty"`
	HeartbeatAt time.Time `json:"heartbeatAt"`
}

type botHealth struct {
	mu     sync.Mutex
	fail   map[string]string
	failAt map[string]time.Time
}

func (h *roomsHandlers) health() *botHealth {
	h.pulseOnce.Do(func() {
		h.pulse = &botHealth{fail: map[string]string{}, failAt: map[string]time.Time{}}
	})
	return h.pulse
}

func (h *roomsHandlers) markBotDegraded(agentID, reason string) {
	if agentID == "" {
		return
	}
	p := h.health()
	p.mu.Lock()
	p.fail[agentID] = roomTrim(reason, 160)
	p.failAt[agentID] = time.Now().UTC()
	p.mu.Unlock()
}

func (h *roomsHandlers) clearBotDegraded(agentID string) {
	p := h.health()
	p.mu.Lock()
	delete(p.fail, agentID)
	delete(p.failAt, agentID)
	p.mu.Unlock()
}

func (h *roomsHandlers) botStatus(c *gin.Context) {
	projectID := strings.TrimSpace(c.Query("projectId"))
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "projectId is required"})
		return
	}
	if _, ok := h.repo.Project(projectID); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}
	if !h.roomAccess(c, projectID, false) {
		return
	}
	userID, _ := h.roomActor(c)
	c.JSON(http.StatusOK, h.botStatuses(projectID, userID))
}

func (h *roomsHandlers) botStatuses(projectID string, userID int64) []botStatusView {
	now := time.Now().UTC()
	hasKey := h.userHasModelKey(userID)
	queue := h.botQueues()
	p := h.health()
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []botStatusView
	for _, a := range h.repo.Agents() {
		if !catalogVisible(a.ProjectID, a.Shared, projectID) {
			continue
		}
		view := botStatusView{AgentID: a.ID, Name: crewAgentName(a), Queue: queue[a.ID], HeartbeatAt: now, Status: "online"}
		switch {
		case agentNeedsKey(a) && !hasKey:
			view.Status = "unavailable"
			view.Reason = "缺 Key"
		case view.Queue > 0:
			view.Status = "busy"
			view.Reason = "排队"
		default:
			if reason := p.fail[a.ID]; reason != "" && now.Sub(p.failAt[a.ID]) < 30*time.Minute {
				view.Status = "degraded"
				view.Reason = reason
			}
		}
		out = append(out, view)
	}
	if out == nil {
		out = []botStatusView{}
	}
	return out
}

func (h *roomsHandlers) userHasModelKey(userID int64) bool {
	user, ok := h.repo.User(userID)
	if !ok {
		return false
	}
	return strings.TrimSpace(user.CodeBuddyAPIKey) != "" || strings.TrimSpace(user.ClaudeCodeAPIKey) != "" || strings.TrimSpace(user.CodexAPIKey) != ""
}

func agentNeedsKey(a domain.AgentDefinition) bool {
	return a.ExecutorID != "mock"
}

func (h *roomsHandlers) botQueues() map[string]int {
	out := map[string]int{}
	for _, msg := range h.repo.CrewMessages("") {
		if msg.Crew == nil || msg.Crew.Status != "running" {
			continue
		}
		for _, t := range msg.Crew.Tasks {
			if t.AgentID != "" && (t.Status == "running" || t.Status == "pending") {
				out[t.AgentID]++
			}
		}
	}
	return out
}

// rewriteRoomMentions rejects an @ of an unavailable bot when nobody else can
// take it, and otherwise rewrites the mention onto the first bot that can.
func (h *roomsHandlers) rewriteRoomMentions(projectID string, userID int64, body string) (string, string, error) {
	statuses := h.botStatuses(projectID, userID)
	var fallback *botStatusView
	for i := range statuses {
		switch statuses[i].Status {
		case "online", "busy", "degraded":
			if fallback == nil {
				fb := statuses[i]
				fallback = &fb
			}
		}
	}
	note := ""
	for _, st := range statuses {
		if st.Status != "unavailable" || st.Name == "" {
			continue
		}
		token := "@" + st.Name
		if !strings.Contains(body, token) {
			continue
		}
		if fallback == nil {
			return body, "", HTTPError{http.StatusConflict, st.Name + " 不可用：" + st.Reason}
		}
		body = strings.ReplaceAll(body, token, "@"+fallback.Name)
		note = "已改由 " + fallback.Name + " 负责（" + st.Name + " " + st.Reason + "）"
	}
	return body, note, nil
}
