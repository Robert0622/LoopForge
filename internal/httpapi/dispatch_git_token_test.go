package httpapi

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/engine"
	"github.com/Tencent/LoopForge/internal/repo"
	"github.com/Tencent/LoopForge/internal/runtime"
	"github.com/Tencent/LoopForge/internal/tracker"
)

func TestDispatchStopsBeforeRunningWhenTheWorkBranchCannotBePushed(t *testing.T) {
	st, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.PutUser(domain.User{LoginName: "owner"}); err != nil {
		t.Fatal(err)
	}
	owner, _ := st.UserByLoginName("owner")
	project := domain.Project{ID: "p", Name: "P", OwnerID: owner.ID, DefaultRepositoryID: "repo"}
	if err := st.PutProject(project); err != nil {
		t.Fatal(err)
	}
	if err := st.PutRepository(domain.Repository{ID: "repo", ProjectID: "p", Name: "Hello-World", Provider: "github", CloneURL: "https://github.com/octocat/Hello-World.git", DefaultBranch: "master"}); err != nil {
		t.Fatal(err)
	}
	wf := domain.WorkflowDefinition{ID: "fix", Name: "Fix", SandboxProfileID: "local", Nodes: []domain.WorkflowNode{{ID: "implement", Type: "agent", AgentID: "a"}}}
	if err := st.PutWorkflow(wf); err != nil {
		t.Fatal(err)
	}
	if err := st.PutIssue(domain.Issue{ID: "i", ProjectID: "p", Title: "t", Kind: "requirement", WorkflowID: "fix", AssigneeUserID: owner.ID}); err != nil {
		t.Fatal(err)
	}
	d := &deps{repo: st, engine: engine.New(st, runtime.NewManager(filepath.Join(t.TempDir(), "ws"))), sync: tracker.NewService()}
	_, err = dispatchIssue(d, dispatchRequest{IssueID: "i", WorkflowID: "fix"})
	if err == nil || !strings.Contains(err.Error(), "GitHub Token") || !strings.Contains(err.Error(), "Hello-World") {
		t.Fatalf("dispatch without a git token must explain the missing token, got %v", err)
	}
	if st.IssueHasTask("i") {
		t.Fatal("no task may be created when dispatch is refused")
	}
}
