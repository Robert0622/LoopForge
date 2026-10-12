package domain

import (
	"strconv"
	"strings"
	"time"
)

type Issue struct {
	ID             string `json:"id"`
	ProjectID      string `json:"projectId"`
	Number         int    `json:"number"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	Status         string `json:"status"`                // backlog, todo, in_progress, in_review, done, closed, blocked, cancelled
	CloseSource    string `json:"closeSource,omitempty"` // manual, external; empty on older manually closed issues
	Priority       string `json:"priority"`
	AssigneeType   string `json:"assigneeType,omitempty"` // member, agent
	AssigneeID     string `json:"assigneeId,omitempty"`
	AssigneeUserID int64  `json:"assigneeUserId,omitempty"`
	WorkflowID     string `json:"workflowId,omitempty"`
	// PlaybookID is which named project combo this requirement uses.
	PlaybookID string `json:"playbookId,omitempty"`
	// SOPBindings is an optional per-issue override of the chosen playbook.
	// Nil means inherit that playbook's steps. AssigneeUserID on the issue
	// stays the overall owner.
	SOPBindings  []IssueSOPBinding `json:"sopBindings,omitempty"`
	RepositoryID string            `json:"repositoryId,omitempty"`
	// RepositoryIDs lists every repository the issue touches. RepositoryID
	// stays the legacy single-repository field (kept in sync with the first
	// entry for display and external sync).
	RepositoryIDs   []string `json:"repositoryIds,omitempty"`
	BaseBranch      string   `json:"baseBranch,omitempty"`
	LatestTaskID    string   `json:"latestTaskId,omitempty"`
	AutomationState string   `json:"automationState,omitempty"` // ignored means scheduled SOPs must not claim it
	DeliveryState   string   `json:"deliveryState,omitempty"`   // merged, reopened
	DeliveryMRURL   string   `json:"deliveryMrUrl,omitempty"`
	DeliveryMRState string   `json:"deliveryMrState,omitempty"` // merged (default branch), integrated (baseline), opened, closed, reopened, unknown
	// DeliveryMRs lists one PR per delivered repository. DeliveryMRURL is its
	// first entry and DeliveryMRState the aggregate (see SetDeliveryMRs).
	DeliveryMRs       []DeliveryMR            `json:"deliveryMrs,omitempty"`
	DeliveryUpdatedAt time.Time               `json:"deliveryUpdatedAt,omitempty"`
	Kind              string                  `json:"kind,omitempty"` // requirement, issue
	Clarification     ClarificationAssessment `json:"clarification"`
	Source            string                  `json:"source,omitempty"` // local, github
	// TrackingID is an optional metrics-only link to an external requirement
	// (for example a Jira key). It does not create a tracker binding.
	TrackingID  string       `json:"trackingId,omitempty"`
	ExternalRef *ExternalRef `json:"externalRef,omitempty"`
	CreatedAt   time.Time    `json:"createdAt"`
	UpdatedAt   time.Time    `json:"updatedAt"`
	// Operations is the requirement's own audit trail: create, edit, close, reopen, dispatch.
	Operations []IssueOperation `json:"operations,omitempty"`
}

// RequirementKey returns the authoritative external requirement key for
// analytics: the linked tracker issue, else the optional manual tracking ID.
func (i Issue) RequirementKey() string {
	if i.ExternalRef != nil {
		if u := strings.TrimSpace(i.ExternalRef.ExternalURL); u != "" {
			return u
		}
		return strings.TrimSpace(i.ExternalRef.ExternalID)
	}
	return strings.TrimSpace(i.TrackingID)
}

// IssueOperation is one user-visible action on a requirement.
type IssueOperation struct {
	At          time.Time `json:"at"`
	Actor       string    `json:"actor,omitempty"`
	ActorUserID int64     `json:"actorUserId,omitempty"`
	Action      string    `json:"action"`
	Detail      string    `json:"detail,omitempty"`
}

// AppendOperation records an action and keeps the newest 200 entries.
func (i *Issue) AppendOperation(op IssueOperation) {
	if i == nil {
		return
	}
	op.Action = strings.TrimSpace(op.Action)
	if op.Action == "" {
		return
	}
	if op.At.IsZero() {
		op.At = time.Now().UTC()
	}
	i.Operations = append(i.Operations, op)
	const maxOps = 200
	if len(i.Operations) > maxOps {
		i.Operations = append([]IssueOperation(nil), i.Operations[len(i.Operations)-maxOps:]...)
	}
}

// EffectiveRepositoryIDs returns the deduplicated, non-empty list of
// repositories bound to the issue. RepositoryIDs is authoritative;
// RepositoryID (the legacy single-repository field) is appended as fallback.
func (i Issue) EffectiveRepositoryIDs() []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(i.RepositoryIDs)+1)
	for _, id := range append(append([]string(nil), i.RepositoryIDs...), i.RepositoryID) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

type ClarificationAssessment struct {
	Score                int                     `json:"score"`
	Passed               bool                    `json:"passed"`
	Answers              map[string]string       `json:"answers,omitempty"`
	Dimensions           map[string]int          `json:"dimensions,omitempty"`
	Missing              []string                `json:"missing,omitempty"`
	Summary              string                  `json:"summary,omitempty"`
	Status               string                  `json:"status,omitempty"` // analyzing, in_progress, ready, needs_user
	History              []ClarificationMessage  `json:"history,omitempty"`
	Questions            []ClarificationQuestion `json:"questions,omitempty"`
	SessionID            string                  `json:"sessionId,omitempty"`
	SessionUserID        int64                   `json:"sessionUserId,omitempty"`
	WriteBackStatus      string                  `json:"writeBackStatus,omitempty"` // pending, succeeded, failed, skipped
	WriteBackError       string                  `json:"writeBackError,omitempty"`
	WriteBackFingerprint string                  `json:"writeBackFingerprint,omitempty"`
	WriteBackAt          time.Time               `json:"writeBackAt,omitempty"`
	// StartedAt is the first observed start of clarification, including
	// pre-issue composer analysis when its server timestamp is carried over.
	StartedAt  time.Time `json:"startedAt,omitempty"`
	AssessedAt time.Time `json:"assessedAt,omitempty"`
	// Gated is set when the issue was scored on a manual intake path
	// (新建需求 / 导入 Issue). External sync leaves this false so those
	// tickets can be dispatched without the import-time score gate.
	Gated bool `json:"gated,omitempty"`
}

type ClarificationMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ClarificationQuestion struct {
	ID          string   `json:"id"`
	Question    string   `json:"question"`
	Options     []string `json:"options,omitempty"`
	MultiSelect bool     `json:"multiSelect,omitempty"`
	FreeText    bool     `json:"freeText,omitempty"`
}

// ExternalRef links a local Issue to an external tracker issue.
type ExternalRef struct {
	Source        string    `json:"source"`        // github
	ExternalID    string    `json:"externalId"`    // forge issue id
	ExternalIID   int       `json:"externalIid"`   // issue number (for URL)
	ExternalURL   string    `json:"externalUrl"`   // clickable URL
	ExternalState string    `json:"externalState"` // opened, closed
	ExternalType  string    `json:"externalType"`  // issue, story
	SyncedAt      time.Time `json:"syncedAt"`
	// Imported is set for issues imported by URL or created from LoopForge.
	Imported bool `json:"imported,omitempty"`
}

// ForgeClosed reports whether this item is a Forge issue already closed on Forge.
// Local status can lag after a failed sync, so dispatch must use ExternalState, not Status.
func (i Issue) ForgeClosed() bool {
	if !strings.EqualFold(strings.TrimSpace(i.Source), "github") {
		return false
	}
	if i.ExternalRef == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(i.ExternalRef.ExternalState), "closed")
}

// ManuallyClosed distinguishes an explicit local close from a tracker close.
// Before CloseSource existed, only manual closes used Issue.Status=closed.
func (i Issue) ManuallyClosed() bool {
	return strings.EqualFold(strings.TrimSpace(i.Status), "closed") &&
		!strings.EqualFold(strings.TrimSpace(i.CloseSource), "external")
}

// TrackerClosed reads the tracker's raw state so a lagging local status cannot
// make an externally closed work item executable.
func (i Issue) TrackerClosed() bool {
	if i.ExternalRef == nil || i.Source != "github" {
		return false
	}
	state := strings.ToLower(strings.TrimSpace(i.ExternalRef.ExternalState))
	return state == "closed"
}

// DeliveryLocked reports whether a merged PR or completed delivery has made this work item final.
// A user can explicitly reopen it to allow a follow-up run.
func (i Issue) DeliveryLocked() bool {
	return strings.EqualFold(strings.TrimSpace(i.DeliveryState), "merged")
}

// DeliveryLockedLabel is the user-facing stage label for a delivery-locked issue.
// PR wording requires a real PR link. Historical manual close stored
// deliveryMrState=closed without a link, and that is not an PR.
func (i Issue) DeliveryLockedLabel() string {
	if i.ManuallyClosed() {
		return "关闭"
	}
	state := strings.ToLower(strings.TrimSpace(i.DeliveryMRState))
	if strings.TrimSpace(i.DeliveryMRURL) == "" {
		if state == "closed" {
			return "已关闭"
		}
		return "交付闭环"
	}
	switch state {
	case "merged":
		return "PR 已合入"
	case "integrated":
		return "已合入基线"
	case "closed":
		return "PR 已关闭"
	default:
		return "交付闭环"
	}
}

const issueIdentityMarker = "需求身份（平台权威，禁止忽略）"

// IdentityBlock is the authoritative tracker identity injected into SOP input
// and the compiled task prompt. Agents must not guess an issue ID from
// branch names when this block is present.
func (i Issue) IdentityBlock() string {
	id := strings.TrimSpace(i.ID)
	if id == "" {
		return ""
	}
	source := strings.ToLower(strings.TrimSpace(i.Source))
	if source == "" {
		source = "local"
	}
	var b strings.Builder
	b.WriteString(issueIdentityMarker)
	b.WriteString("：\n")
	b.WriteString("- 平台工作项：")
	b.WriteString(id)
	b.WriteString("\n- 来源：")
	switch source {
	case "github":
		b.WriteString("GitHub")
	default:
		b.WriteString("本地")
	}
	b.WriteByte('\n')
	ref := i.ExternalRef
	if ref == nil {
		if trackingID := i.RequirementKey(); trackingID != "" {
			b.WriteString("- 统计关联 ID：")
			b.WriteString(trackingID)
			b.WriteString("（仅用于指标归集，不代表外部同步关系）\n")
		} else {
			b.WriteString("- 外部 ID：无\n")
		}
		return b.String()
	}
	if kind := strings.TrimSpace(ref.ExternalType); kind != "" {
		b.WriteString("- 类型：")
		b.WriteString(kind)
		b.WriteByte('\n')
	}
	if extID := strings.TrimSpace(ref.ExternalID); extID != "" {
		b.WriteString("- 外部 ID：")
		b.WriteString(extID)
		b.WriteByte('\n')
	}
	if ref.ExternalIID != 0 {
		b.WriteString("- 外部编号：")
		b.WriteString(strconv.Itoa(ref.ExternalIID))
		b.WriteByte('\n')
	}
	if url := strings.TrimSpace(ref.ExternalURL); url != "" {
		b.WriteString("- 链接：")
		b.WriteString(url)
		b.WriteByte('\n')
	}
	return b.String()
}

func (i Issue) HasIdentityBlock(text string) bool {
	return strings.Contains(text, issueIdentityMarker)
}

func (i Issue) ExecutionBlockedReason() string {
	if i.ManuallyClosed() {
		return "工作项已关闭，请先重新打开后再触发 SOP"
	}
	if i.TrackerClosed() || (i.Status == "closed" && i.CloseSource == "external") {
		return "外部工作项已关闭，请先在外部系统重新打开"
	}
	if i.DeliveryLocked() {
		switch i.DeliveryLockedLabel() {
		case "已关闭":
			return "工作项已关闭，请先重新打开后再触发 SOP"
		case "PR 已合入":
			return "关联 PR 已合入，请先重新打开工作项再触发 SOP"
		case "已合入基线":
			return "已合入基线，请先重新打开工作项再触发 SOP"
		case "PR 已关闭":
			return "关联 PR 已关闭，请先重新打开工作项再触发 SOP"
		default:
			return "工作项已交付闭环，请先重新打开后再触发 SOP"
		}
	}
	return ""
}

type IssueComment struct {
	ID         string    `json:"id"`
	IssueID    string    `json:"issueId"`
	AuthorType string    `json:"authorType"`
	AuthorID   string    `json:"authorId"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"createdAt"`
}

type RejectRequest struct {
	NodeID   string `json:"nodeId"`
	TargetID string `json:"targetId"`
	Reason   string `json:"reason"`
	Context  string `json:"context,omitempty"`
}

// HumanDecision is the shared payload for approve / retry so operators can
// attach extra facts (failed test names, links) that later nodes will see.
type HumanDecision struct {
	NodeID  string `json:"nodeId"`
	Reason  string `json:"reason,omitempty"`
	Context string `json:"context,omitempty"`
}
