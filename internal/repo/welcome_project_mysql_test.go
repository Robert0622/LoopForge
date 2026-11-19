package repo

import (
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"agentflow/internal/domain"
)

func TestMySQLConcurrentWelcomeEnrollment(t *testing.T) {
	dsn := os.Getenv("AGENTFLOW_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("requires disposable MySQL")
	}
	store, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	id := "onboard-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	t.Cleanup(func() {
		store.DB.Where("project_id = ?", id).Delete(&issueRow{})
		store.DB.Where("project_id = ?", id).Delete(&projectMemberRow{})
		store.DB.Where("id = ?", id).Delete(&projectRow{})
	})
	// Both admins already joined as ordinary members on the old version.
	p := domain.Project{ID: id, Name: "Existing project", Members: []domain.ProjectMember{{UserID: 100, Role: "member"}, {UserID: 101, Role: "viewer"}}}
	if err := store.PutProject(p); err != nil {
		t.Fatal(err)
	}
	for _, i := range []domain.Issue{
		{ID: id + "-unassigned", ProjectID: id, Title: "Needs an owner"},
		{ID: id + "-assigned", ProjectID: id, Title: "Keep assignee", AssigneeUserID: 101},
	} {
		if err := store.PutIssue(i); err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	var workers sync.WaitGroup
	for uid := int64(100); uid < 112; uid++ {
		workers.Add(1)
		go func(userID int64) {
			defer workers.Done()
			<-start
			event := WelcomeMemberSignup
			if userID <= 101 {
				event = WelcomeAdminLogin
			}
			if err := store.EnrollWelcomeAccount(id, userID, event); err != nil {
				t.Error(err)
			}
		}(uid)
	}
	close(start)
	workers.Wait()
	p, ok := store.Project(id)
	if !ok || (p.OwnerID != 100 && p.OwnerID != 101) || len(p.Members) != 12 {
		t.Fatalf("owner=%d members=%v", p.OwnerID, p.Members)
	}
	for _, m := range p.Members {
		want := "member"
		if m.UserID == 101 {
			want = "viewer"
		}
		if m.UserID == p.OwnerID {
			want = "maintainer"
		}
		if m.Role != want {
			t.Fatalf("member %d: got %q, want %q", m.UserID, m.Role, want)
		}
	}
	unassigned, _ := store.Issue(id + "-unassigned")
	assigned, _ := store.Issue(id + "-assigned")
	if unassigned.AssigneeUserID != p.OwnerID || assigned.AssigneeUserID != 101 {
		t.Fatalf("assignees after claim: %d, %d", unassigned.AssigneeUserID, assigned.AssigneeUserID)
	}
}
