package domain

import (
	"encoding/json"
)

const (
	RunnerWorkspaceOperationInfo   = "info"
	RunnerWorkspaceOperationTree   = "tree"
	RunnerWorkspaceOperationFile   = "file"
	RunnerWorkspaceOperationDiff   = "diff"
	RunnerWorkspaceOperationSearch = "search"
	RunnerWorkspaceOperationLog    = "log"
	RunnerWorkspaceOperationCommit = "commit"
)

// RunnerWorkspaceRequest is a bounded, read-only request sent over the
// runner's outbound control socket. Responses are transient and are never
// persisted in runner_tasks or the run record.
type RunnerWorkspaceRequest struct {
	RunID           string              `json:"runId"`
	RunStatus       string              `json:"runStatus,omitempty"`
	WorkspaceLayout string              `json:"workspaceLayout,omitempty"`
	Operation       string              `json:"operation"`
	Path            string              `json:"path,omitempty"`
	Query           string              `json:"query,omitempty"`
	Ref             string              `json:"ref,omitempty"`
	Repositories    []RepositoryContext `json:"repositories,omitempty"`
}

type RunnerWorkspaceResponse struct {
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   string          `json:"error,omitempty"`
	Status  int             `json:"status,omitempty"`
}
