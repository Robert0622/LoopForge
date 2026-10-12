package repo

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
)

func TestStore_UserMemory(t *testing.T) {
	st, err := OpenTest()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	defer st.Close()

	u1 := domain.User{
		LoginName:    "alice",
		DisplayName:  "Alice",
		PasswordHash: "hash-1",
	}

	if err := st.PutUser(u1); err != nil {
		t.Fatalf("PutUser failed: %v", err)
	}

	// 验证按 LoginName 读取及自动生成的自增 ID
	got, ok := st.UserByLoginName("alice")
	if !ok {
		t.Fatalf("expected user 'alice' to be found by login name")
	}
	if got.ID <= 0 {
		t.Errorf("expected auto-generated ID > 0, got %d", got.ID)
	}
	if got.DisplayName != "Alice" || got.PasswordHash != "hash-1" || got.CreatedAt.IsZero() {
		t.Errorf("unexpected user data: %+v", got)
	}

	// 验证按主键 ID 读取
	gotByID, ok := st.User(got.ID)
	if !ok || gotByID.LoginName != "alice" {
		t.Fatalf("expected user to be found by ID %d", got.ID)
	}

	// 验证按 LoginName 读取
	gotByLogin, ok := st.UserByLoginName("alice")
	if !ok || gotByLogin.LoginName != "alice" {
		t.Errorf("UserByLoginName failed: %+v", gotByLogin)
	}

	// 验证大小写不敏感检索
	gotByLoginUpper, ok := st.UserByLoginName("ALICE")
	if !ok || gotByLoginUpper.LoginName != "alice" {
		t.Errorf("UserByLoginName case-insensitive failed: %+v", gotByLoginUpper)
	}

	// 验证更新保留 CreatedAt 并更新 UpdatedAt
	time.Sleep(10 * time.Millisecond)
	u1Update := got
	u1Update.DisplayName = "Alice New"
	u1Update.PasswordHash = ""
	if err := st.PutUser(u1Update); err != nil {
		t.Fatalf("PutUser update failed: %v", err)
	}

	updated, ok := st.User(got.ID)
	if !ok || updated.DisplayName != "Alice New" || updated.PasswordHash != "hash-1" {
		t.Fatalf("update failed: %+v", updated)
	}
	if !updated.CreatedAt.Equal(got.CreatedAt) {
		t.Errorf("expected CreatedAt to be preserved: got %v, want %v", updated.CreatedAt, got.CreatedAt)
	}
	if !updated.UpdatedAt.After(got.UpdatedAt) && !updated.UpdatedAt.Equal(got.UpdatedAt) {
		t.Errorf("expected UpdatedAt >= prev UpdatedAt: got %v, prev %v", updated.UpdatedAt, got.UpdatedAt)
	}

	// 验证列表
	u2 := domain.User{
		LoginName:   "bob",
		DisplayName: "Bob",
	}
	if err := st.PutUser(u2); err != nil {
		t.Fatalf("PutUser u2 failed: %v", err)
	}

	all := st.Users()
	if len(all) != 3 {
		t.Fatalf("expected seeded owner plus 2 users, got %d", len(all))
	}
}

func TestIssueResponsibilityFallsBackToProjectOwner(t *testing.T) {
	st, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.PutUser(domain.User{ID: 42, LoginName: "zhangsan", DisplayName: "张三", GitToken: "git-user", CodeBuddyAPIKey: "ck-user"}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutProject(domain.Project{ID: "owned", Name: "Owned", OwnerID: 42, Members: []domain.ProjectMember{{UserID: 42, Role: "maintainer"}}}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutIssue(domain.Issue{ID: "req-owner", ProjectID: "owned", Title: "fallback"}); err != nil {
		t.Fatal(err)
	}
	issue, _ := st.Issue("req-owner")
	if issue.AssigneeUserID != 42 {
		t.Fatalf("assigneeUserId=%d, want project owner 42", issue.AssigneeUserID)
	}
	responsible, err := st.ResponsibleUser(issue)
	if err != nil || responsible.ID != 42 || responsible.GitToken != "git-user" {
		t.Fatalf("responsible user=%+v err=%v", responsible, err)
	}
}

func TestResponsibleUserReportsMissingOwnerAndAssignee(t *testing.T) {
	st, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.PutProject(domain.Project{ID: "ownerless", Name: "Ownerless"}); err != nil {
		t.Fatal(err)
	}
	_, err = st.ResponsibleUser(domain.Issue{ID: "req", ProjectID: "ownerless"})
	if err == nil || !strings.Contains(err.Error(), "项目未配置 Owner") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStore_UserMySQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("LOOPFORGE_TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("LOOPFORGE_TEST_MYSQL_DSN not set, skipping MySQL integration test")
	}
	st, err := Open(dsn)
	if err != nil {
		t.Skipf("cannot connect to MySQL: %v", err)
	}
	defer st.Close()

	testLogin := "test_ci_user_1"
	st.DB.Exec(`DELETE FROM users WHERE login_name=?`, testLogin)
	defer func() {
		st.DB.Exec(`DELETE FROM users WHERE login_name=?`, testLogin)
	}()

	u := domain.User{
		LoginName:   testLogin,
		DisplayName: "CI测试员",
	}

	if err := st.PutUser(u); err != nil {
		t.Fatalf("PutUser mysql failed: %v", err)
	}

	gotByLogin, ok := st.UserByLoginName(testLogin)
	if !ok {
		t.Fatalf("expected user to be found by login name")
	}
	if gotByLogin.ID <= 0 {
		t.Fatalf("expected auto-increment ID > 0, got %d", gotByLogin.ID)
	}
	if gotByLogin.DisplayName != "CI测试员" {
		t.Errorf("unexpected user data: %+v", gotByLogin)
	}

	gotByID, ok := st.User(gotByLogin.ID)
	if !ok || gotByID.LoginName != testLogin {
		t.Fatalf("expected user to be found by ID: %d", gotByLogin.ID)
	}

	// 更新
	gotByLogin.DisplayName = "CI测试员修改"
	if err := st.PutUser(gotByLogin); err != nil {
		t.Fatalf("PutUser update mysql failed: %v", err)
	}

	updated, ok := st.User(gotByLogin.ID)
	if !ok || updated.DisplayName != "CI测试员修改" {
		t.Fatalf("expected updated DisplayName, got %+v", updated)
	}
}

func TestMySQLUserOwnershipRelations(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("LOOPFORGE_TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("LOOPFORGE_TEST_MYSQL_DSN not set")
	}
	st, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	suffix := time.Now().UnixNano()
	login := fmt.Sprintf("owner_%d", suffix)
	projectID := fmt.Sprintf("owned-%d", suffix)
	issueID := fmt.Sprintf("issue-%d", suffix)
	defer func() {
		st.DB.Exec(`DELETE FROM issues WHERE id=?`, issueID)
		st.DB.Exec(`DELETE FROM projects WHERE id=?`, projectID)
		st.DB.Exec(`DELETE FROM users WHERE login_name=?`, login)
	}()
	if err := st.PutUser(domain.User{LoginName: login, DisplayName: "关系测试", GitToken: "git-secret", CodeBuddyAPIKey: "ck-secret"}); err != nil {
		t.Fatal(err)
	}
	owner, _ := st.UserByLoginName(login)
	if err := st.PutProject(domain.Project{ID: projectID, Name: "Owned", OwnerID: owner.ID, Members: []domain.ProjectMember{{UserID: owner.ID, Role: "maintainer"}}}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutIssue(domain.Issue{ID: issueID, ProjectID: projectID, Title: "Fallback"}); err != nil {
		t.Fatal(err)
	}
	project, ok := st.Project(projectID)
	if !ok || project.OwnerID != owner.ID || len(project.Members) != 1 || project.Members[0].UserID != owner.ID {
		t.Fatalf("project relations not hydrated: %+v", project)
	}
	issue, ok := st.Issue(issueID)
	if !ok || issue.AssigneeUserID != owner.ID {
		t.Fatalf("issue responsibility not persisted: %+v", issue)
	}
	var memberRows, assignedRows int
	if err := st.DB.Raw(`SELECT COUNT(*) FROM project_members WHERE project_id=? AND user_id=?`, projectID, owner.ID).Scan(&memberRows).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.DB.Raw(`SELECT COUNT(*) FROM issues WHERE id=? AND assignee_user_id=?`, issueID, owner.ID).Scan(&assignedRows).Error; err != nil {
		t.Fatal(err)
	}
	if memberRows != 1 || assignedRows != 1 {
		t.Fatalf("relation rows: members=%d assigned=%d", memberRows, assignedRows)
	}
}
