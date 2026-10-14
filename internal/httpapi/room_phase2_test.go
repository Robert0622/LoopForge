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
	"github.com/Tencent/LoopForge/internal/engine"
	"github.com/Tencent/LoopForge/internal/repo"
	"github.com/Tencent/LoopForge/internal/runtime"
	"github.com/Tencent/LoopForge/internal/tracker"
)

func TestCrewHandoffOmitsTranscript(t *testing.T) {
	prev := domain.CrewTask{
		ID: "t1", Status: "succeeded", AgentName: "甲", Title: "方案",
		Output: "SECRET TRANSCRIPT\n" + strings.Repeat("过程", 800), Summary: "一句结论",
		Handoff: &domain.CrewHandoff{Conclusion: "定价用阶梯", Artifacts: []string{"notes.md"}, OpenQuestions: []string{"税怎么算"}},
	}
	next := domain.CrewTask{ID: "t2", AgentName: "乙", Title: "实现"}
	brief := crewBrief(&domain.CrewMission{Goal: "定价", Tasks: []domain.CrewTask{prev, next}}, &next)
	if strings.Contains(brief, "SECRET") || strings.Contains(brief, "过程过程") {
		t.Fatalf("handoff leaked the transcript:\n%s", brief)
	}
	for _, want := range []string{"定价用阶梯", "notes.md", "税怎么算", "结论：", "产物：", "未决问题："} {
		if !strings.Contains(brief, want) {
			t.Fatalf("brief missing %q:\n%s", want, brief)
		}
	}
}

func TestRoomThreadLinesSkipMainTimeline(t *testing.T) {
	all := []domain.ChatMessage{
		{ID: "a", Author: "旁人", Body: "无关的主时间线"},
		{ID: "p", Author: "甲", Body: "线程里的上一条"},
		{ID: "q", Author: "乙", Body: "接着问", ParentID: "p"},
	}
	got := strings.Join(roomThreadLines(all, all[2]), "\n")
	if strings.Contains(got, "无关") {
		t.Fatalf("thread context included the main timeline: %s", got)
	}
	if !strings.Contains(got, "parent_id=p") || !strings.Contains(got, "线程里的上一条") {
		t.Fatalf("thread context: %s", got)
	}
}

func TestParseCrewHandoffFields(t *testing.T) {
	got := parseCrewHandoff("结论：可以交付\n产物：api.md\n未决问题：无\nSECRET 不该进结论以外的字段", "")
	if got.Conclusion != "可以交付" || len(got.Artifacts) != 1 || got.Artifacts[0] != "api.md" || len(got.OpenQuestions) != 0 {
		t.Fatalf("handoff: %+v", got)
	}
}

func phase2Handler(t *testing.T) (*repo.Store, http.Handler, domain.User) {
	t.Helper()
	t.Setenv("LOOPFORGE_SESSION_SECRET", "room-phase2-session-secret")
	t.Setenv("LOOPFORGE_FAKE_MODEL", "1")
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.PutAgent(domain.AgentDefinition{ID: "standin", Name: "开发实现", ProjectID: "platform", Shared: true, ExecutorID: "mock", Description: "演示执行器", Instructions: "实现"}); err != nil {
		t.Fatal(err)
	}
	h := New(s, engine.New(s, runtime.NewManager(t.TempDir())), tracker.NewService(), "", "")
	owner, _ := s.User(1)
	return s, h, owner
}

func TestBotStatusAndMentionReassign(t *testing.T) {
	_, h, owner := phase2Handler(t)
	room := requestJSONAuthed[domain.ChatRoomView](t, h, owner, http.MethodPost, "/api/rooms", map[string]any{"projectId": "platform", "name": "状态群"}, http.StatusCreated)
	rows := requestJSONAuthed[[]botStatusView](t, h, owner, http.MethodGet, "/api/bots/status?projectId=platform", nil, http.StatusOK)
	var dev, stand botStatusView
	for _, row := range rows {
		if row.AgentID == "developer" {
			dev = row
		}
		if row.AgentID == "standin" {
			stand = row
		}
		if row.HeartbeatAt.IsZero() {
			t.Fatalf("missing heartbeat: %+v", row)
		}
	}
	if dev.Status != "unavailable" || dev.Reason != "缺 Key" {
		t.Fatalf("developer status: %+v", dev)
	}
	if stand.Status != "online" {
		t.Fatalf("mock bot status: %+v", stand)
	}
	msg := requestJSONAuthed[domain.ChatMessage](t, h, owner, http.MethodPost, "/api/rooms/"+room.ID+"/messages", map[string]any{"body": "@实现工程师 帮我看一下"}, http.StatusCreated)
	if !strings.Contains(msg.Body, "@开发实现") || !strings.Contains(msg.ReassignNote, "已改由 开发实现 负责") {
		t.Fatalf("mention was not reassigned: %+v", msg)
	}
}

func TestCrewPlanCardNamesReassignment(t *testing.T) {
	_, h, owner := phase2Handler(t)
	room := requestJSONAuthed[domain.ChatRoomView](t, h, owner, http.MethodPost, "/api/rooms", map[string]any{"projectId": "platform", "name": "分工群"}, http.StatusCreated)
	requestJSONAuthed[domain.ChatMessage](t, h, owner, http.MethodPost, "/api/rooms/"+room.ID+"/messages", map[string]any{"body": "@LoopForge 实现登录页"}, http.StatusCreated)
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		messages := requestJSONAuthed[[]domain.ChatMessage](t, h, owner, http.MethodGet, "/api/rooms/"+room.ID+"/messages", nil, http.StatusOK)
		for _, m := range messages {
			if m.Crew == nil {
				continue
			}
			for _, task := range m.Crew.Tasks {
				if task.ReassignedFrom != "" && strings.Contains(task.AgentName, "开发实现") {
					return
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("crew plan did not record 已改由")
}

func TestFakeModelStreamsFirstTokenAndStop(t *testing.T) {
	_, h, owner := phase2Handler(t)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	token, err := auth.NewClientFromEnv().SignUser(&auth.User{ID: owner.ID, LoginName: owner.LoginName, AuthTime: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	room := requestJSONAuthed[domain.ChatRoomView](t, h, owner, http.MethodPost, "/api/rooms", map[string]any{"projectId": "platform", "name": "流式群"}, http.StatusCreated)
	requestJSONAuthed[domain.ChatMessage](t, h, owner, http.MethodPost, "/api/rooms/"+room.ID+"/messages", map[string]any{
		"body":      "@LoopForge 你好",
		"fakeModel": map[string]any{"text": "甲乙丙", "firstTokenMs": 40, "tokenMs": 20, "stallAfter": 1, "stallMs": 4000},
	}, http.StatusCreated)
	var pending domain.ChatMessage
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		messages := requestJSONAuthed[[]domain.ChatMessage](t, h, owner, http.MethodGet, "/api/rooms/"+room.ID+"/messages", nil, http.StatusOK)
		for _, m := range messages {
			if m.Pending && m.Kind == domain.ChatKindCrewPost {
				pending = m
			}
		}
		if pending.ID != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if pending.ID == "" {
		t.Fatal("pending reply was not created")
	}
	ctxReq, err := http.NewRequest(http.MethodGet, ts.URL+"/api/rooms/"+room.ID+"/messages/"+pending.ID+"/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctxReq.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	started := time.Now()
	type result struct {
		body string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		res, err := http.DefaultClient.Do(ctxReq)
		if err != nil {
			ch <- result{err: err}
			return
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		ch <- result{body: string(raw), err: err}
	}()
	time.Sleep(120 * time.Millisecond)
	stopReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/rooms/"+room.ID+"/messages/"+pending.ID+"/stop", strings.NewReader(`{}`))
	stopReq.Header.Set("Content-Type", "application/json")
	stopReq.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	stopRes, err := http.DefaultClient.Do(stopReq)
	if err != nil {
		t.Fatal(err)
	}
	stopRes.Body.Close()
	if stopRes.StatusCode != http.StatusOK {
		t.Fatalf("stop status %d", stopRes.StatusCode)
	}
	select {
	case got := <-ch:
		if got.err != nil {
			t.Fatal(got.err)
		}
		if time.Since(started) > 1500*time.Millisecond {
			t.Fatalf("first token path took %s", time.Since(started))
		}
		if !strings.Contains(got.body, "event: token") || !strings.Contains(got.body, "甲") {
			t.Fatalf("stream body:\n%s", got.body)
		}
		if !strings.Contains(got.body, "event: stopped") {
			t.Fatalf("stop did not end the stream:\n%s", got.body)
		}
		if strings.Contains(got.body, "丙") {
			t.Fatalf("stall token leaked after stop:\n%s", got.body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not finish after stop")
	}
}

func TestFakeModelGate(t *testing.T) {
	t.Setenv("LOOPFORGE_FAKE_MODEL", "1")
	t.Setenv("LOOPFORGE_DEMO_MODE", "")
	if !fakeModelAllowed() {
		t.Fatal("LOOPFORGE_FAKE_MODEL should enable the mock")
	}
	t.Setenv("LOOPFORGE_FAKE_MODEL", "")
	t.Setenv("LOOPFORGE_DEMO_MODE", "true")
	if !fakeModelAllowed() {
		t.Fatal("demo mode should enable the mock for e2e")
	}
	t.Setenv("LOOPFORGE_DEMO_MODE", "")
	if fakeModelAllowed() {
		t.Fatal("fake model must stay off in production")
	}
}
