package engine

import (
	"errors"
	"testing"

	"github.com/Tencent/LoopForge/internal/domain"
)

func TestDeliveryMRsFromRunFallsBackWhenLookupFails(t *testing.T) {
	e := &Engine{FindBranchMR: func(project, branch string) (domain.DeliveryMR, bool, error) {
		if project == "g/a" {
			return domain.DeliveryMR{IID: 21, ForgeState: "opened"}, true, nil
		}
		return domain.DeliveryMR{}, false, errors.New("forge unavailable")
	}}
	run := domain.Run{
		ID: "run-1",
		Repositories: []domain.RepositoryContext{
			{RepositoryID: "a", RepoPath: "g/a", WorkBranch: "feature/x"},
			{RepositoryID: "b", RepoPath: "g/b", WorkBranch: "feature/x"},
		},
		Artifacts: []domain.Artifact{{Name: "mr-body", Content: "https://github.com/g/a/pull/1 https://github.com/g/b/pull/6"}},
	}
	got := e.deliveryMRsFromRun(run)
	if len(got) != 2 || got[0].IID != 21 || got[1].IID != 6 || got[1].RepositoryID != "b" {
		t.Fatalf("deliveryMRsFromRun() = %+v", got)
	}
}
