package engine

import (
	"strings"

	"github.com/Tencent/LoopForge/internal/domain"
)

// HumanWaitHook is called when a run parks on an approval gate (not user pause).
type HumanWaitHook func(run domain.Run, nodeID, nodeName string)

// RunFailedHook is called when a run settles as failed.
type RunFailedHook func(run domain.Run, message string)

// SetHumanWaitHook wires outbound notify when a run needs a human decision.
func (e *Engine) SetHumanWaitHook(fn HumanWaitHook) {
	e.mu.Lock()
	e.humanWaitHook = fn
	e.mu.Unlock()
}

// SetRunFailedHook wires outbound notify when a run fails.
func (e *Engine) SetRunFailedHook(fn RunFailedHook) {
	e.mu.Lock()
	e.runFailedHook = fn
	e.mu.Unlock()
}

func (e *Engine) fireHumanWait(runID string) {
	e.mu.Lock()
	fn := e.humanWaitHook
	e.mu.Unlock()
	if fn == nil {
		return
	}
	run, ok := e.repo.RunState(runID)
	if !ok || run.PauseRequested {
		return
	}
	for id, nr := range run.Nodes {
		if nr == nil || nr.Status != "waiting_approval" {
			continue
		}
		name := id
		for _, n := range run.WorkflowSnapshot.Nodes {
			if n.ID == id {
				name = n.Name
				break
			}
		}
		fn(run, id, name)
		return
	}
}

func (e *Engine) fireRunFailed(runID, message string) {
	e.mu.Lock()
	fn := e.runFailedHook
	e.mu.Unlock()
	if fn == nil {
		return
	}
	run, ok := e.repo.RunState(runID)
	if !ok {
		return
	}
	fn(run, strings.TrimSpace(message))
}
