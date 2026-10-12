package repo

import (
	"strings"
	"testing"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
)

func TestAssistantSessionAndTaskRoundTrip(t *testing.T) {
	st, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	user, ok := st.UserByLoginName("demo-owner")
	if !ok {
		t.Fatal("seed user")
	}
	other := domain.User{LoginName: "assistant-other", DisplayName: "其他用户"}
	if err := st.PutUser(other); err != nil {
		t.Fatal(err)
	}
	other, ok = st.UserByLoginName("assistant-other")
	if !ok {
		t.Fatal("other user")
	}

	session := domain.AssistantSession{
		ID:        "af-session-1",
		Title:     "hello",
		ProjectID: "platform",
		Model:     "glm-5.3-flash",
		Messages:  []domain.AssistantMessage{{Role: "user", Content: "hello"}},
	}
	if err := st.PutAssistantSession(user.ID, session); err != nil {
		t.Fatal(err)
	}
	got, ok := st.AssistantSession(user.ID, session.ID)
	if !ok || got.Title != "hello" || len(got.Messages) != 1 || got.UserID != user.ID {
		t.Fatalf("%+v %v", got, ok)
	}
	created := got.CreatedAt
	if created.IsZero() {
		t.Fatal("created at")
	}
	list := st.ListAssistantSessions(user.ID)
	if len(list) != 1 || list[0].Messages != nil || list[0].Title != "hello" {
		t.Fatalf("%+v", list)
	}
	session.Messages = append(session.Messages, domain.AssistantMessage{Role: "assistant", Content: `{"message":"草稿","drafts":[{"tool":"agents_save","saved":true}]}`})
	if err := st.PutAssistantSession(user.ID, session); err != nil {
		t.Fatal(err)
	}
	got, ok = st.AssistantSession(user.ID, session.ID)
	if !ok || !got.CreatedAt.Equal(created) || len(got.Messages) != 2 || !strings.Contains(got.Messages[1].Content, `"saved":true`) {
		t.Fatalf("saved draft should persist: %+v", got)
	}
	if _, ok := st.AssistantSession(other.ID, session.ID); ok {
		t.Fatal("session must be scoped to owner")
	}
	if err := st.PutAssistantSession(other.ID, domain.AssistantSession{ID: session.ID, Title: "stolen"}); err == nil {
		t.Fatal("other user must not overwrite session")
	}

	task := domain.AssistantTask{ID: "at-task-1", Name: "巡检", Prompt: "看一下待办", IntervalSec: 60, Enabled: true, LastRunAt: time.Now().UTC()}
	if err := st.PutAssistantTask(user.ID, task); err != nil {
		t.Fatal(err)
	}
	stored, ok := st.AssistantTask(user.ID, task.ID)
	if !ok || stored.Name != "巡检" || stored.Prompt != "看一下待办" || !stored.Enabled {
		t.Fatalf("%+v %v", stored, ok)
	}
	if len(st.EnabledAssistantTasks()) != 1 {
		t.Fatalf("%+v", st.EnabledAssistantTasks())
	}
	if _, ok := st.AssistantTask(other.ID, task.ID); ok {
		t.Fatal("task must be scoped to owner")
	}
	if err := st.DeleteAssistantTask(user.ID, task.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.AssistantTask(user.ID, task.ID); ok {
		t.Fatal("task should be gone")
	}
	if err := st.DeleteAssistantSession(user.ID, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.AssistantSession(user.ID, session.ID); ok {
		t.Fatal("session should be gone")
	}
}
