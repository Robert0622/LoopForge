package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/observability"
	"github.com/gin-gonic/gin"
)

type runsHandlers struct {
	*deps
	runnerControls *runnerControlHub
	evalJobs       *runEvalHub
}

func (h *runsHandlers) analytics(c *gin.Context) {
	filter := domain.AnalyticsFilter{
		Origin:           c.Query("origin"),
		ProjectID:        c.Query("projectId"),
		WorkflowID:       c.Query("workflowId"),
		Scope:            strings.TrimSpace(c.Query("scope")),
		UserID:           strings.TrimSpace(c.Query("userId")),
		IssueSource:      strings.TrimSpace(c.Query("issueSource")),
		RequirementsOnly: c.Query("requirementsOnly") == "1",
		Grain:            strings.TrimSpace(c.Query("grain")),
	}
	if raw := c.Query("from"); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			c.JSON(400, map[string]string{"error": "invalid from timestamp"})
			return
		}
		filter.From = value
	}
	if raw := c.Query("to"); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			c.JSON(400, map[string]string{"error": "invalid to timestamp"})
			return
		}
		filter.To = value
	}
	if filter.Origin != "" && filter.Origin != "all" && filter.Origin != "internal" && filter.Origin != "external" {
		c.JSON(400, gin.H{"error": "origin 必须为 all、internal 或 external"})
		return
	}
	snap := h.repo.Analytics(filter)
	if len(snap.DataErrors) > 0 {
		c.JSON(503, gin.H{"error": snap.DataErrors[0]})
		return
	}
	c.JSON(200, snap)
}

func (h *runsHandlers) workflowMetrics(c *gin.Context) {
	c.JSON(200, h.repo.WorkflowMetricsFor(c.Query("projectId")))
}

func (h *runsHandlers) runs(c *gin.Context) {
	r := c.Request
	if r.Method == http.MethodGet {
		runs, err := h.repo.RunSummariesFor(domain.AnalyticsFilter{ProjectID: c.Query("projectId")}, "")
		if err != nil {
			respond(c, err)
			return
		}
		c.JSON(200, runSummaries(runs))
		return
	}
	if r.Method == http.MethodPost {
		var req struct {
			WorkflowID string `json:"workflowId"`
			Input      string `json:"input"`
			ProjectID  string `json:"projectId"`
		}
		if !decode(c, &req) {
			return
		}
		req.ProjectID = strings.TrimSpace(req.ProjectID)
		var project domain.Project
		if req.ProjectID != "" {
			var ok bool
			project, ok = h.repo.Project(req.ProjectID)
			if !ok {
				respond(c, fmt.Errorf("project %q not found", req.ProjectID))
				return
			}
		}
		var responsibleUserID int64
		if current, ok := h.requestUser(r, true); ok {
			responsibleUserID = current.ID
		} else if project.OwnerID > 0 {
			responsibleUserID = project.OwnerID
		}
		if responsibleUserID == 0 {
			c.JSON(http.StatusConflict, map[string]string{"error": "手动运行需要登录用户或已配置 Owner 的项目"})
			return
		}
		var repositories []domain.RepositoryContext
		if wf, ok := h.repo.Workflow(req.WorkflowID); ok && wf.Generic {
			repositoryID := strings.TrimSpace(project.DefaultRepositoryID)
			if repositoryID == "" {
				c.JSON(http.StatusConflict, map[string]string{"error": "直接运行通用交付模板前，请先配置项目默认代码仓库"})
				return
			}
			repoDef, found := h.repo.Repository(repositoryID)
			if !found || repoDef.ProjectID != req.ProjectID {
				c.JSON(http.StatusConflict, map[string]string{"error": "项目默认代码仓库无效，请重新配置"})
				return
			}
			repositories = []domain.RepositoryContext{{
				RepositoryID: repoDef.ID, Provider: repoDef.Provider, Name: repoDef.Name,
				RepoPath: repoDef.RepoPath, WebURL: repoDef.WebURL, CloneURL: repoDef.CloneURL,
				BaseBranch: repoDef.DefaultBranch, DefaultBranch: repoDef.DefaultBranch, LocalPath: repoDef.LocalPath,
				PipelineURLPattern: repoDef.PipelineURLPattern,
			}}
		}
		v, err := h.engine.StartWithRepositoriesContext(req.WorkflowID, req.Input, repositories, domain.TelemetryContext{ProjectID: req.ProjectID, ResponsibleUserID: responsibleUserID})
		if err != nil {
			respond(c, err)
			return
		}
		c.JSON(201, v)
		return
	}
}
func (h *runsHandlers) run(c *gin.Context) {
	id := c.Param("id")
	switch c.FullPath() {
	case "/api/runs/:id":
		if c.Request.Method == http.MethodGet {
			v, ok := h.repo.RunView(id)
			if !ok {
				c.JSON(404, map[string]string{"error": "run not found"})
				return
			}
			c.JSON(200, publicRun(v, false))
			return
		}
		if err := h.engine.DeleteRun(id); err != nil {
			respond(c, err)
			return
		}
		c.JSON(200, map[string]string{"status": "deleted"})
	case "/api/runs/:id/telemetry":
		if !h.repo.RunExists(id) {
			c.JSON(404, map[string]string{"error": "run not found"})
			return
		}
		nodeID := strings.TrimSpace(c.Query("nodeId"))
		adapter := strings.TrimSpace(c.Query("adapter"))
		page, err := h.repo.QueryRunTelemetry(id, nodeID, adapter, parseQueryInt64(c.Request, "after"), parseQueryInt64(c.Request, "before"), c.Request.URL.Query().Has("after"), c.Request.URL.Query().Has("before"), parsePageLimit(c.Request))
		if err != nil {
			respond(c, err)
			return
		}
		events := make([]domain.TelemetryEvent, 0, len(page.Events))
		for _, event := range page.Events {
			events = append(events, slimTelemetryEvent(event))
		}
		c.JSON(200, map[string]any{"runId": id, "sequence": page.Sequence, "events": events, "nextAfter": page.NextAfter, "nextBefore": page.NextBefore, "hasMore": page.HasMore})
	case "/api/runs/:id/logs":
		if !h.repo.RunExists(id) {
			c.JSON(404, map[string]string{"error": "run not found"})
			return
		}
		nodeID := strings.TrimSpace(c.Query("nodeId"))
		page, err := h.repo.QueryRunLogs(id, nodeID, parseQueryInt64(c.Request, "after"), parseQueryInt64(c.Request, "before"), c.Request.URL.Query().Has("after"), c.Request.URL.Query().Has("before"), parsePageLimit(c.Request))
		if err != nil {
			respond(c, err)
			return
		}
		logs := make([]domain.LogRecord, 0, len(page.Logs))
		for _, rec := range page.Logs {
			logs = append(logs, slimLogRecord(rec))
		}
		c.JSON(200, map[string]any{"runId": id, "sequence": page.Sequence, "logs": logs, "nextAfter": page.NextAfter, "nextBefore": page.NextBefore, "hasMore": page.HasMore})
	case "/api/runs/:id/logs.txt":
		if !h.repo.RunExists(id) {
			c.JSON(404, map[string]string{"error": "run not found"})
			return
		}
		nodeID := strings.TrimSpace(c.Query("nodeId"))
		recs, err := h.repo.DumpRunLogs(id, nodeID)
		if err != nil {
			respond(c, err)
			return
		}
		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.Header("Content-Disposition", `attachment; filename="`+id+`-worker-logs.txt"`)
		c.Status(http.StatusOK)
		fmt.Fprintf(c.Writer, "# LoopForge worker logs\n# run=%s\n# nodes=%s\n# count=%d\n\n", id, logDumpNodeLabel(nodeID), len(recs))
		for _, rec := range recs {
			stamp := rec.Timestamp.Local().Format("15:04:05")
			node := rec.Context.NodeID
			if node == "" {
				node = "run"
			}
			fmt.Fprintf(c.Writer, "%s\t%s\t%s\n%s\n\n", stamp, node, rec.Stream, rec.Body)
		}
	case "/api/runs/:id/signals":
		var envelope domain.WorkerEnvelope
		if !decode(c, &envelope) {
			return
		}
		if err := h.engine.IngestWorkerEnvelope(id, envelope); err != nil {
			respond(c, err)
			return
		}
		c.JSON(202, map[string]bool{"accepted": true})
	case "/api/runs/:id/artifacts/:name":
		artifact, ok := h.repo.Artifact(id, c.Param("name"))
		if !ok {
			c.JSON(404, map[string]string{"error": "artifact not found"})
			return
		}
		c.Header("Content-Type", artifact.MediaType+"; charset=utf-8")
		c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", artifact.Name+artifactExtension(artifact.MediaType)))
		c.Header("ETag", `"`+artifact.SHA256+`"`)
		_, _ = c.Writer.Write([]byte(artifact.Content))
	case "/api/runs/:id/approve":
		var req domain.HumanDecision
		if decode(c, &req) {
			if rejectBlockedIssueRun(h.deps, c, id) {
				return
			}
			respond(c, h.engine.Approve(id, req.NodeID, req.Context))
		}
	case "/api/runs/:id/reject":
		var req domain.RejectRequest
		if decode(c, &req) {
			if rejectBlockedIssueRun(h.deps, c, id) {
				return
			}
			respond(c, h.engine.Reject(id, req))
		}
	case "/api/runs/:id/retry":
		var req domain.HumanDecision
		if decode(c, &req) {
			if rejectBlockedIssueRun(h.deps, c, id) {
				return
			}
			respond(c, h.engine.Retry(id, req.NodeID, req.Reason, req.Context))
		}
	case "/api/runs/:id/decline-retry":
		var req struct {
			NodeID string `json:"nodeId"`
		}
		if decode(c, &req) {
			respond(c, h.engine.DeclineRetry(id, req.NodeID))
		}
	case "/api/runs/:id/cancel":
		respond(c, h.engine.Cancel(id))
	case "/api/runs/:id/pause":
		respond(c, h.engine.Pause(id))
	case "/api/runs/:id/resume":
		var req domain.HumanDecision
		if decode(c, &req) {
			if rejectBlockedIssueRun(h.deps, c, id) {
				return
			}
			respond(c, h.engine.ResumePaused(id, req.Context))
		}
	case "/api/runs/:id/refresh-sop":
		respond(c, h.engine.RefreshRunSOP(id))
	case "/api/runs/:id/pass":
		var req domain.HumanDecision
		if decode(c, &req) {
			if rejectBlockedIssueRun(h.deps, c, id) {
				return
			}
			respond(c, h.engine.Pass(id, req.NodeID, req.Context))
		}
	default:
		c.Status(http.StatusNotFound)
	}
}

func artifactExtension(mediaType string) string {
	if mediaType == "text/markdown" {
		return ".md"
	}
	if mediaType == "application/json" {
		return ".json"
	}
	return ".txt"
}

func runSummaries(runs []domain.Run) []domain.Run {
	for i := range runs {
		runs[i] = publicRun(runs[i], true)
	}
	return runs
}

func publicRun(run domain.Run, summary bool) domain.Run {
	if run.ArtifactCount == 0 {
		run.ArtifactCount = len(run.Artifacts)
	}
	if run.Timing == nil {
		timing := domain.ComputeRunTiming(run, run.Events, time.Now().UTC())
		run.Timing = &timing
	}
	if summary {
		run.Artifacts = nil
	}
	run.Artifacts = append([]domain.Artifact(nil), run.Artifacts...)
	for i := range run.Artifacts {
		run.Artifacts[i].Content = ""
	}
	newNodes := make(map[string]*domain.NodeRun, len(run.Nodes))
	for k, n := range run.Nodes {
		if n == nil {
			newNodes[k] = nil
			continue
		}
		cp := *n
		if summary {
			cp.Output = ""
			cp.Error = clipRunText(cp.Error, 160)
			cp.Feedback = ""
			cp.Activity = ""
		} else {
			cp.Output = clipRunText(cp.Output, 2500)
			cp.Error = clipRunText(cp.Error, 1200)
			cp.Feedback = clipRunText(cp.Feedback, 800)
			cp.Activity = clipRunText(cp.Activity, 300)
		}
		newNodes[k] = &cp
	}
	run.Nodes = newNodes
	if summary {
		run.Telemetry = nil
		run.Events = nil
		run.Logs = nil
		run.WorkflowSnapshot = domain.WorkflowDefinition{ID: run.WorkflowSnapshot.ID, Name: run.WorkflowSnapshot.Name}
		run.Input = clipRunText(run.Input, 500)
		return run
	}
	run.WorkflowSnapshot = slimWorkflowSnapshot(run.WorkflowSnapshot)
	run.Telemetry = nil
	run.Logs = nil
	if len(run.Events) > 80 {
		run.Events = append([]domain.RunEvent(nil), run.Events[len(run.Events)-80:]...)
	}
	return run
}

// publicLiveRun is the SSE payload: keep finished-node output and artifact
// metadata so the inspector can update without a full refetch, but still drop
// log/telemetry bodies (those arrive as deltas).
func publicLiveRun(run domain.Run) domain.Run {
	run = publicRun(run, false)
	run.Events = nil
	run.Input = clipRunText(run.Input, 500)
	return run
}

func parseQueryInt64(r *http.Request, key string) int64 {
	n, _ := strconv.ParseInt(r.URL.Query().Get(key), 10, 64)
	return n
}

func parsePageLimit(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	return n
}

func slimWorkflowSnapshot(wf domain.WorkflowDefinition) domain.WorkflowDefinition {
	nodes := make([]domain.WorkflowNode, len(wf.Nodes))
	for i, n := range wf.Nodes {
		n.Script = ""
		n.Command = ""
		nodes[i] = n
	}
	wf.Nodes = nodes
	wf.Description = clipRunText(wf.Description, 200)
	return wf
}

func slimTelemetryEvent(e domain.TelemetryEvent) domain.TelemetryEvent {
	e.Message = clipRunText(e.Message, 800)
	if observability.MatchAdapter(e, "hooks") {
		safe := map[string]any{}
		for _, key := range []string{"hook", "hookId", "hookName", "tool", "executionPhase", "matcher", "language", "durationMs", "exitCode", "stdout", "stderr"} {
			if value, ok := e.Attributes[key]; ok {
				safe[key] = value
			}
		}
		e.Attributes = safe
		e.Source = domain.TelemetrySource{Layer: e.Source.Layer, Adapter: e.Source.Adapter, Provider: e.Source.Provider, Protocol: e.Source.Protocol}
	} else {
		e.Attributes = nil
		e.Source = domain.TelemetrySource{Adapter: e.Source.Adapter, Protocol: e.Source.Protocol}
	}
	e.Metrics = nil
	e.Tool = nil
	e.ModelOperation = nil
	e.SpecVersion = ""
	e.Context = domain.TelemetryContext{NodeID: e.Context.NodeID, RunID: e.Context.RunID}
	return e
}

func slimLogRecord(l domain.LogRecord) domain.LogRecord {
	l.Body = clipRunText(l.Body, 2000)
	l.SpecVersion = ""
	return l
}

func logDumpNodeLabel(nodeID string) string {
	if strings.TrimSpace(nodeID) == "" {
		return "all"
	}
	return nodeID
}

func clipRunText(value string, n int) string {
	if len(value) <= n {
		return value
	}
	return value[:n] + "…"
}
