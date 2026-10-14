package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/LoopForge/internal/auth"
	"github.com/Tencent/LoopForge/internal/domain"
)

func TestCrewStepStreamsFailRetryAndSOP(t *testing.T) {
	s, h, owner := phase2Handler(t)
	if err := s.PutAgent(domain.AgentDefinition{ID: "analyst", Name: "需求分析", ProjectID: "platform", Shared: true, ExecutorID: "mock", Description: "拆解需求", Instructions: "分析"}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	token, err := auth.NewClientFromEnv().SignUser(&auth.User{ID: owner.ID, LoginName: owner.LoginName, AuthTime: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	room := requestJSONAuthed[domain.ChatRoomView](t, h, owner, http.MethodPost, "/api/rooms", map[string]any{"projectId": "platform", "name": "协作流"}, http.StatusCreated)
	started := time.Now()
	requestJSONAuthed[domain.ChatMessage](t, h, owner, http.MethodPost, "/api/rooms/"+room.ID+"/messages", map[string]any{
		"body":      "@LoopForge 实现登录页",
		"fakeModel": map[string]any{"crewStep": true, "failStep": 2, "firstTokenMs": 40, "tokenMs": 4},
	}, http.StatusCreated)

	var mission domain.ChatMessage
	var first domain.ChatMessage
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		messages := requestJSONAuthed[[]domain.ChatMessage](t, h, owner, http.MethodGet, "/api/rooms/"+room.ID+"/messages", nil, http.StatusOK)
		for _, m := range messages {
			if m.Kind == domain.ChatKindCrew && m.Crew != nil {
				mission = m
			}
			if m.Kind == domain.ChatKindCrewPost && m.AgentID != "" && strings.Contains(m.Body, "结论") {
				first = m
			}
		}
		if mission.ID != "" && first.ID != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if mission.ID == "" || first.ID == "" {
		t.Fatalf("crew step did not stream a handoff within %s", time.Since(started))
	}
	if time.Since(started) > 1500*time.Millisecond {
		t.Fatalf("first crew token took %s", time.Since(started))
	}

	ctxReq, err := http.NewRequest(http.MethodGet, ts.URL+"/api/rooms/"+room.ID+"/messages/"+first.ID+"/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctxReq.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	res, err := http.DefaultClient.Do(ctxReq)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "event: token") && !strings.Contains(string(raw), "event: done") {
		t.Fatalf("crew stream:\n%s", raw)
	}

	var failed domain.ChatMessage
	deadline = time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		messages := requestJSONAuthed[[]domain.ChatMessage](t, h, owner, http.MethodGet, "/api/rooms/"+room.ID+"/messages", nil, http.StatusOK)
		for _, m := range messages {
			if m.Kind == domain.ChatKindCrewPost && m.Error != "" {
				failed = m
			}
			if m.ID == mission.ID && m.Crew != nil {
				mission = m
			}
		}
		if failed.ID != "" && mission.Crew != nil && mission.Crew.Status == "waiting" {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if failed.ID == "" || mission.Crew == nil || mission.Crew.Status != "waiting" {
		t.Fatalf("step 2 did not fail open: post=%q mission=%+v", failed.Error, mission.Crew)
	}

	requestJSONAuthed[domain.ChatMessage](t, h, owner, http.MethodPost, "/api/rooms/"+room.ID+"/crew/"+mission.ID, map[string]any{"action": "retry"}, http.StatusOK)
	deadline = time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		messages := requestJSONAuthed[[]domain.ChatMessage](t, h, owner, http.MethodGet, "/api/rooms/"+room.ID+"/messages", nil, http.StatusOK)
		for _, m := range messages {
			if m.ID == mission.ID && m.Crew != nil && m.Crew.Status == "done" {
				mission = m
			}
		}
		if mission.Crew != nil && mission.Crew.Status == "done" {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if mission.Crew == nil || mission.Crew.Status != "done" {
		t.Fatalf("crew did not finish after retry: %+v", mission.Crew)
	}
	for _, task := range mission.Crew.Tasks {
		if task.Status == "succeeded" && !strings.HasPrefix(task.RunID, "stream:") {
			t.Fatalf("succeeded step was not streamed: %+v", task)
		}
	}
	saved := requestJSONAuthed[map[string]any](t, h, owner, http.MethodPost, "/api/rooms/"+room.ID+"/crew/"+mission.ID, map[string]any{"action": "sop"}, http.StatusOK)
	if saved["workflowId"] == "" {
		t.Fatalf("sop was not saved: %+v", saved)
	}
}
