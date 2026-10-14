package domain

import "time"

// WorkflowStatusCrew marks the hidden single-role workflow behind one crew
// step. Catalog listings skip it.
const WorkflowStatusCrew = "crew"

// CrewMission is open-ended work handed to the bots of a room instead of a
// fixed SOP. The LoopForge captain plans which roles take which steps; every
// step runs as its own single-role run, reports back in the room and hands
// its result to the next bot. People steer it by talking in the room.
type CrewMission struct {
	ID     string `json:"id"`
	Goal   string `json:"goal"`
	Detail string `json:"detail,omitempty"`
	// Status: planning, proposed, running, waiting, done, failed, stopped.
	Status        string     `json:"status"`
	Planner       string     `json:"planner,omitempty"` // ai, rules
	IssueID       string     `json:"issueId,omitempty"`
	IssueNumber   int        `json:"issueNumber,omitempty"`
	Tasks         []CrewTask `json:"tasks"`
	Notes         []CrewNote `json:"notes,omitempty"`
	Question      string     `json:"question,omitempty"`
	QuestionAgent string     `json:"questionAgent,omitempty"`
	AutoStart     bool       `json:"autoStart,omitempty"`
	StartedBy     int64      `json:"startedBy,omitempty"`
	StartedByName string     `json:"startedByName,omitempty"`
	Source        string     `json:"source,omitempty"` // room, home
	SavedWorkflow string     `json:"savedWorkflowId,omitempty"`
	Error         string     `json:"error,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

// CrewTask is one bot's step. Status: pending, running, succeeded, failed,
// skipped.
type CrewTask struct {
	ID          string `json:"id"`
	AgentID     string `json:"agentId"`
	AgentName   string `json:"agentName"`
	Title       string `json:"title"`
	Instruction string `json:"instruction,omitempty"`
	Status      string `json:"status"`
	AddedBy     string `json:"addedBy,omitempty"`
	WorkflowID  string `json:"workflowId,omitempty"`
	RunID       string `json:"runId,omitempty"`
	Summary     string `json:"summary,omitempty"`
	Output      string `json:"output,omitempty"`
	Error       string `json:"error,omitempty"`
	// Handoff is the only context the next bot receives from this step.
	Handoff *CrewHandoff `json:"handoff,omitempty"`
	// ReassignedFrom names the bot that was skipped because it was unavailable.
	ReassignedFrom   string    `json:"reassignedFrom,omitempty"`
	ReassignedReason string    `json:"reassignedReason,omitempty"`
	StartedAt        time.Time `json:"startedAt,omitempty"`
	FinishedAt       time.Time `json:"finishedAt,omitempty"`
}

// CrewHandoff is the structured packet passed between crew steps.
// The next bot sees this, not the previous bot's full transcript.
type CrewHandoff struct {
	Conclusion    string   `json:"conclusion"`
	Artifacts     []string `json:"artifacts,omitempty"`
	OpenQuestions []string `json:"openQuestions,omitempty"`
}

// CrewNote is something a person said in the room while the mission ran;
// the next bot to start receives every note not yet handed over.
type CrewNote struct {
	Author   string    `json:"author"`
	Body     string    `json:"body"`
	At       time.Time `json:"at"`
	Consumed bool      `json:"consumed,omitempty"`
}

// CrewMissionActive reports whether the captain still has work to drive.
func CrewMissionActive(status string) bool {
	switch status {
	case "planning", "proposed", "running", "waiting":
		return true
	}
	return false
}
