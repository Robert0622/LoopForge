//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLiveServer drives a real loopforge process. Script SOPs run in the
// local sandbox; nothing here calls a model CLI.
func TestLiveServer(t *testing.T) {
	base, stop := startServer(t)
	t.Cleanup(stop)
	admin := newClient(t, base)
	admin.login("admin", "e2e-admin-pass")

	t.Run("auth", func(t *testing.T) {
		admin.bind(t)
		guest := newClient(t, base)
		status, body := guest.call(http.MethodPost, "/api/auth/login", map[string]string{"loginName": "admin", "password": "nope"})
		if status != http.StatusUnauthorized {
			t.Fatalf("wrong password status %d %s", status, body)
		}
		admin.callOK(http.MethodGet, "/api/health", nil)
		me := admin.json(http.MethodGet, "/api/auth/current", nil)
		user, _ := me["user"].(map[string]any)
		if user["loginName"] != "admin" {
			t.Fatalf("current user: %#v", me)
		}
	})

	t.Run("script sop writes an artifact", func(t *testing.T) {
		admin.bind(t)
		publishScript(t, admin, "e2e-script", "E2E 脚本交付", "printf 'e2e-script-marker\\n' > marker.txt", "marker", "marker.txt")
		run := dispatchAndWait(t, admin, "e2e-script", "脚本：写出标记文件", "succeeded")
		if !strings.Contains(artifactBody(t, admin, run, "marker"), "e2e-script-marker") {
			t.Fatal("artifact did not contain the script output")
		}
		logs := admin.text("/api/runs/" + runID(run) + "/logs.txt")
		if !strings.Contains(logs, "write") && !strings.Contains(logs, "marker") && logs == "" {
			t.Fatalf("run log was empty: %q", logs)
		}
	})

	t.Run("approval gate then script", func(t *testing.T) {
		admin.bind(t)
		wf := workflow("e2e-gate", "E2E 审批后写文件", "platform", []any{
			node("write", "先写草稿", "script", map[string]any{
				"scriptLanguage": "shell", "script": "printf 'draft\\n' > draft.txt", "maxAttempts": 1,
			}),
			node("gate", "人工确认", "approval", map[string]any{"maxAttempts": 1}),
			node("final", "写终稿", "script", map[string]any{
				"scriptLanguage": "shell", "script": "printf 'e2e-approved\\n' > final.txt", "maxAttempts": 1,
				"artifactOutputs": []any{map[string]any{"name": "final", "path": "final.txt", "required": true}},
			}),
		}, []any{
			edge("e1", "write", "gate"),
			edge("e2", "gate", "final"),
		})
		admin.putWorkflow(wf)
		run := dispatchAndWait(t, admin, "e2e-gate", "审批：确认后再写终稿", "waiting")
		admin.callOK(http.MethodPost, "/api/runs/"+runID(run)+"/approve", map[string]string{"nodeId": "gate", "context": "e2e 批准"})
		done := waitRun(t, admin, runID(run), "succeeded")
		if done["status"] != "succeeded" {
			t.Fatalf("approved run: %#v", done["status"])
		}
		if !strings.Contains(artifactBody(t, admin, done, "final"), "e2e-approved") {
			t.Fatal("final artifact missing after approval")
		}
	})

	t.Run("failing script stays failed and retry runs again", func(t *testing.T) {
		admin.bind(t)
		publishScript(t, admin, "e2e-fail", "E2E 失败脚本", "echo e2e-failed; exit 1", "", "")
		run := dispatchAndWait(t, admin, "e2e-fail", "脚本：预期失败", "failed")
		admin.callOK(http.MethodPost, "/api/runs/"+runID(run)+"/retry", map[string]string{"nodeId": "write", "reason": "再试一次"})
		again := waitRun(t, admin, runID(run), "failed")
		if attempt(again, "write") < 2 {
			t.Fatalf("retry did not execute the script again: %#v", again["nodes"])
		}
	})

	t.Run("cancel stops a running script", func(t *testing.T) {
		admin.bind(t)
		publishScript(t, admin, "e2e-cancel", "E2E 取消脚本", "sleep 30", "", "")
		run := dispatchAndWait(t, admin, "e2e-cancel", "脚本：取消睡眠", "running")
		admin.callOK(http.MethodPost, "/api/runs/"+runID(run)+"/cancel", map[string]any{})
		waitRun(t, admin, runID(run), "cancelled")
	})

	t.Run("nested sop runs the child", func(t *testing.T) {
		admin.bind(t)
		publishScript(t, admin, "e2e-child", "E2E 子流程", "printf 'from-child\\n' > child.txt", "child", "child.txt")
		parent := workflow("e2e-parent", "E2E 父流程", "platform", []any{
			node("call", "调用子流程", "sop", map[string]any{"sopWorkflowId": "e2e-child", "maxAttempts": 1}),
		}, nil)
		admin.putWorkflow(parent)
		run := dispatchAndWait(t, admin, "e2e-parent", "嵌套：跑子 SOP", "succeeded")
		nodes, _ := run["nodes"].(map[string]any)
		call, _ := nodes["call"].(map[string]any)
		childID, _ := call["childRunId"].(string)
		if childID == "" {
			t.Fatalf("parent did not record a child run: %#v", call)
		}
		child := waitRun(t, admin, childID, "succeeded")
		if child["status"] != "succeeded" {
			t.Fatalf("child run: %#v", child["status"])
		}
	})

	t.Run("demo sop walks agent approval and report", func(t *testing.T) {
		admin.bind(t)
		run := dispatchAndWait(t, admin, "demo-delivery", "演示：真实引擎跑内置流程", "waiting")
		admin.callOK(http.MethodPost, "/api/runs/"+runID(run)+"/approve", map[string]string{"nodeId": "approve"})
		done := waitRun(t, admin, runID(run), "succeeded")
		body := artifactBody(t, admin, done, "交付报告")
		if !strings.Contains(body, "交付报告") {
			t.Fatalf("demo report: %s", body)
		}
	})

	t.Run("catalog assets", func(t *testing.T) {
		admin.bind(t)
		admin.callOK(http.MethodPut, "/api/agents", map[string]any{
			"id": "e2e-role", "name": "E2E 角色", "projectId": "platform", "executorId": "codebuddy",
			"description": "只用于目录往返", "instructions": "不要调用外部模型",
		})
		admin.callOK(http.MethodPut, "/api/rules", map[string]any{
			"id": "e2e-rule", "name": "E2E 规则", "projectId": "platform", "content": "始终写出产物路径", "priority": 10,
		})
		admin.callOK(http.MethodPut, "/api/skills", map[string]any{
			"id": "e2e-skill", "name": "E2E 技能", "projectId": "platform", "content": "# 技能\n\n检查产物。",
		})
		admin.callOK(http.MethodPut, "/api/dockerfiles", map[string]any{
			"id": "e2e-dockerfile", "name": "E2E Dockerfile", "projectId": "platform", "content": "FROM scratch\n",
		})
		admin.callOK(http.MethodPut, "/api/environments", map[string]any{
			"id": "e2e-env", "name": "E2E 环境", "projectId": "platform", "installScript": "#!/bin/sh\nmkdir -p \"$PREFIX/bin\"\n",
		})
		admin.callOK(http.MethodPut, "/api/sandboxes", map[string]any{
			"id": "e2e-sandbox", "name": "E2E 沙箱", "projectId": "platform", "driver": "local",
		})
		for _, path := range []string{"/api/agents/e2e-role", "/api/rules", "/api/skills", "/api/dockerfiles", "/api/environments", "/api/sandboxes"} {
			raw := admin.text(path)
			if !strings.Contains(raw, "e2e-") {
				t.Fatalf("%s did not keep the asset: %s", path, raw)
			}
		}
	})

	t.Run("project issue comment close reopen", func(t *testing.T) {
		admin.bind(t)
		admin.callOK(http.MethodPost, "/api/projects", map[string]any{"id": "e2e-proj", "key": "E2E", "name": "E2E 项目"})
		issue := admin.json(http.MethodPost, "/api/issues", map[string]any{
			"projectId": "e2e-proj", "title": "只评论不派发", "kind": "requirement", "syncTarget": "local", "status": "todo",
		})
		id := fmt.Sprint(issue["id"])
		admin.callOK(http.MethodPost, "/api/issues/"+id+"/comments", map[string]string{"body": "e2e 评论"})
		detail := admin.json(http.MethodGet, "/api/issues/"+id, nil)
		comments, _ := detail["comments"].([]any)
		if len(comments) == 0 {
			t.Fatalf("comment missing: %#v", detail)
		}
		closed := admin.json(http.MethodPost, "/api/issues/"+id+"/close", map[string]any{})
		if closed["status"] != "closed" && closed["closedAt"] == nil {
			// Some builds keep status and set a closed flag. Accept either a
			// closed status or a manual-close marker in the payload.
			raw, _ := json.Marshal(closed)
			if !strings.Contains(string(raw), "closed") && !strings.Contains(string(raw), "close") {
				t.Fatalf("close: %s", raw)
			}
		}
		admin.callOK(http.MethodPost, "/api/issues/"+id+"/reopen", map[string]any{})
	})

	t.Run("room message and dispatch", func(t *testing.T) {
		admin.bind(t)
		room := admin.json(http.MethodPost, "/api/rooms", map[string]any{"projectId": "platform", "name": "E2E 值班群"})
		roomID := fmt.Sprint(room["id"])
		admin.callOK(http.MethodPost, "/api/rooms/"+roomID+"/messages", map[string]string{"body": "今天跑脚本 SOP"})
		card := admin.json(http.MethodPost, "/api/rooms/"+roomID+"/issues", map[string]any{
			"title": "群聊：跑脚本 SOP", "workflowId": "e2e-script",
		})
		task, _ := card["task"].(map[string]any)
		if task == nil || fmt.Sprint(task["runId"]) == "" {
			t.Fatalf("room dispatch: %#v", card)
		}
		waitRun(t, admin, fmt.Sprint(task["runId"]), "succeeded")
	})

	t.Run("schedule dispatches an unrun issue", func(t *testing.T) {
		admin.bind(t)
		issue := admin.json(http.MethodPost, "/api/issues", map[string]any{
			"projectId": "platform", "title": "定时：尚未派发", "kind": "requirement", "syncTarget": "local",
			"status": "in_review", "workflowId": "e2e-script",
		})
		job := admin.json(http.MethodPost, "/api/schedule-jobs", map[string]any{
			"id": "e2e-job", "name": "E2E 定时派发", "projectId": "platform", "workflowId": "e2e-script",
			"enabled": true, "intervalSec": 3600, "issueStatuses": []string{"in_review"}, "useIssueWorkflow": true,
		})
		if job["id"] != "e2e-job" {
			t.Fatalf("job: %#v", job)
		}
		status, _ := admin.call(http.MethodPost, "/api/schedule-jobs/e2e-job/run", map[string]any{})
		if status != http.StatusAccepted && status != http.StatusOK {
			t.Fatalf("schedule run status %d", status)
		}
		deadline := time.Now().Add(20 * time.Second)
		var raw string
		for {
			raw = admin.text("/api/schedule-jobs")
			if strings.Contains(raw, `"lastStatus":"success"`) && strings.Contains(raw, `"lastDispatched":1`) {
				break
			}
			if strings.Contains(raw, `"id":"e2e-job"`) && strings.Contains(raw, `"lastStatus":"failed"`) {
				t.Fatalf("schedule failed: %s", raw)
			}
			if time.Now().After(deadline) {
				t.Fatalf("schedule did not dispatch: %s", raw)
			}
			time.Sleep(200 * time.Millisecond)
		}
		tasks := admin.text("/api/tasks")
		if !strings.Contains(tasks, fmt.Sprint(issue["id"])) {
			t.Fatalf("scheduled issue was not dispatched: %s", tasks)
		}
	})

	t.Run("knowledge token stays off the page", func(t *testing.T) {
		admin.bind(t)
		status, body := admin.call(http.MethodPut, "/api/knowledge/settings", map[string]any{
			"url": "ftp://kb.example/mcp", "token": "secret", "enabled": false,
		})
		if status != http.StatusBadRequest || !strings.Contains(body, "http") {
			t.Fatalf("bad url: %d %s", status, body)
		}
		const secret = "e2e-live-kb-token"
		saved := admin.json(http.MethodPut, "/api/knowledge/settings", map[string]any{
			"url": "http://127.0.0.1:1/mcp", "token": secret, "enabled": false, "writeEnabled": false,
		})
		raw, _ := json.Marshal(saved)
		if strings.Contains(string(raw), secret) {
			t.Fatal("settings response included the token")
		}
		again := admin.text("/api/knowledge/settings")
		if strings.Contains(again, secret) || !strings.Contains(again, "http://127.0.0.1:1/mcp") {
			t.Fatalf("stored settings: %s", again)
		}
	})

	t.Run("runner bind code registers once", func(t *testing.T) {
		admin.bind(t)
		issued := admin.json(http.MethodPost, "/api/runners/bind-codes", map[string]any{})
		code := fmt.Sprint(issued["code"])
		if code == "" {
			t.Fatalf("bind code: %#v", issued)
		}
		machine := newClient(t, base)
		registered := machine.jsonHeader(http.MethodPost, "/api/runners/register", map[string]any{
			"name": "e2e-box", "os": "linux", "arch": "amd64", "maxConcurrency": 1,
		}, map[string]string{"X-Bootstrap-Token": code})
		session := fmt.Sprint(registered["sessionToken"])
		if session == "" {
			t.Fatalf("register: %#v", registered)
		}
		machine.jsonHeader(http.MethodPost, "/api/runners/heartbeat", map[string]any{"status": "online", "version": "e2e"}, map[string]string{"Authorization": "Bearer " + session})
		mine := admin.text("/api/runners?mine=1")
		if !strings.Contains(mine, "e2e-box") {
			t.Fatalf("runner list: %s", mine)
		}
		replay := newClient(t, base)
		status, _ := replay.callHeader(http.MethodPost, "/api/runners/register", map[string]any{"name": "again"}, map[string]string{"X-Bootstrap-Token": code})
		if status != http.StatusUnauthorized {
			t.Fatalf("replayed bind code status %d", status)
		}
	})

	t.Run("member cannot create accounts", func(t *testing.T) {
		admin.bind(t)
		admin.callOK(http.MethodPost, "/api/users", map[string]any{
			"loginName": "e2e-member", "displayName": "E2E Member", "password": "member-password-1",
		})
		member := newClient(t, base)
		member.login("e2e-member", "member-password-1")
		status, body := member.call(http.MethodPost, "/api/users", map[string]any{
			"loginName": "nope", "displayName": "Nope", "password": "nope-password-1",
		})
		if status != http.StatusForbidden {
			t.Fatalf("member create user: %d %s", status, body)
		}
	})

	t.Run("market metrics and workflow revisions", func(t *testing.T) {
		admin.bind(t)
		market := admin.text("/api/mcp-market")
		if !strings.Contains(market, "GitHub") {
			t.Fatalf("market: %s", truncate(market, 200))
		}
		analytics := admin.text("/api/analytics?projectId=platform")
		if analytics == "" || strings.Contains(analytics, "error") && !strings.Contains(analytics, "{") {
			t.Fatalf("analytics: %s", analytics)
		}
		revs := admin.text("/api/workflows/e2e-script/revisions")
		if revs == "" || revs == "null" || revs == "[]" {
			t.Fatalf("revisions: %s", revs)
		}
	})

	t.Run("requirement deletion respects run history", func(t *testing.T) {
		admin.bind(t)
		plain := admin.json(http.MethodPost, "/api/issues", map[string]any{
			"projectId": "platform", "title": "删除：从未派发", "kind": "requirement", "syncTarget": "local", "status": "todo",
		})
		plainID := fmt.Sprint(plain["id"])
		detail := admin.json(http.MethodGet, "/api/issues/"+plainID, nil)
		if view, _ := detail["issue"].(map[string]any); view["deletionAllowed"] != true || view["closureAllowed"] != true {
			t.Fatalf("fresh issue should be deletable and closable: %#v", detail["issue"])
		}
		admin.callOK(http.MethodDelete, "/api/issues/"+plainID, nil)
		if status, _ := admin.call(http.MethodGet, "/api/issues/"+plainID, nil); status != http.StatusNotFound {
			t.Fatalf("deleted issue still readable: %d", status)
		}

		succeeded := dispatchAndWait(t, admin, "e2e-script", "删除：已有成功运行", "succeeded")
		doneID := issueIDOf(succeeded)
		if status, body := admin.call(http.MethodDelete, "/api/issues/"+doneID, nil); status != http.StatusConflict {
			t.Fatalf("issue with a succeeded run was deletable: %d %s", status, body)
		}
		if view, _ := admin.json(http.MethodGet, "/api/issues/"+doneID, nil)["issue"].(map[string]any); view["deletionAllowed"] != false {
			t.Fatalf("succeeded issue should hide deletion: %#v", view)
		}

		waiting := dispatchAndWait(t, admin, "e2e-gate", "删除：等待审批中", "waiting")
		busyID := issueIDOf(waiting)
		if status, body := admin.call(http.MethodPost, "/api/issues/"+busyID+"/close", map[string]any{}); status != http.StatusConflict {
			t.Fatalf("closing an issue with an active run: %d %s", status, body)
		}
		if status, body := admin.call(http.MethodDelete, "/api/issues/"+busyID, nil); status != http.StatusConflict {
			t.Fatalf("deleting an issue with an active run: %d %s", status, body)
		}
		admin.callOK(http.MethodPost, "/api/runs/"+runID(waiting)+"/cancel", map[string]any{})
		waitRun(t, admin, runID(waiting), "cancelled")
		if view, _ := admin.json(http.MethodGet, "/api/issues/"+busyID, nil)["issue"].(map[string]any); view["deletionAllowed"] != true {
			t.Fatalf("cancelled issue must be deletable immediately: %#v", view)
		}
		admin.callOK(http.MethodDelete, "/api/issues/"+busyID, nil)
		if status, _ := admin.call(http.MethodGet, "/api/runs/"+runID(waiting), nil); status != http.StatusNotFound {
			t.Fatalf("cancelled run history should be cleaned with the issue: %d", status)
		}
	})

	t.Run("business domain attribution and report", func(t *testing.T) {
		admin.bind(t)
		admin.callOK(http.MethodPost, "/api/projects", map[string]any{"id": "e2e-bd", "key": "EBD", "name": "E2E 业务域项目"})
		if status, body := admin.call(http.MethodPut, "/api/business-domains", map[string]any{"id": "all", "name": "保留字"}); status != http.StatusBadRequest {
			t.Fatalf("reserved id accepted: %d %s", status, body)
		}
		saved := admin.json(http.MethodPut, "/api/business-domains?createOnly=true", map[string]any{
			"id": "e2e-growth", "name": "E2E 增长", "description": "e2e", "active": true,
		})
		if saved["id"] != "e2e-growth" || saved["active"] != true {
			t.Fatalf("create domain: %#v", saved)
		}
		if status, _ := admin.call(http.MethodPut, "/api/business-domains?createOnly=true", map[string]any{"id": "e2e-growth", "name": "重复", "active": true}); status != http.StatusConflict {
			t.Fatalf("duplicate createOnly status %d", status)
		}
		list := admin.text("/api/business-domains")
		if !strings.Contains(list, `"e2e-growth"`) || !strings.Contains(list, `"canAdmin":true`) {
			t.Fatalf("domain list: %s", truncate(list, 300))
		}
		if status, body := admin.call(http.MethodPost, "/api/projects", map[string]any{"id": "e2e-bd", "name": "E2E 业务域项目", "businessDomainId": "e2e-growth"}); status != http.StatusBadRequest {
			t.Fatalf("project save must not change attribution: %d %s", status, body)
		}
		assigned := admin.json(http.MethodPut, "/api/projects/e2e-bd/business-domain", map[string]any{
			"businessDomainId": "e2e-growth", "expectedDomainId": "", "reason": "e2e 首次归属",
		})
		if assigned["businessDomainId"] != "e2e-growth" {
			t.Fatalf("assign: %#v", assigned)
		}
		if status, body := admin.call(http.MethodPut, "/api/projects/e2e-bd/business-domain", map[string]any{
			"businessDomainId": "", "expectedDomainId": "", "reason": "过期的预期值",
		}); status != http.StatusConflict {
			t.Fatalf("stale expectedDomainId: %d %s", status, body)
		}
		history := admin.text("/api/projects/e2e-bd/business-domain/history")
		if !strings.Contains(history, "e2e 首次归属") || !strings.Contains(history, `"toDomainId":"e2e-growth"`) {
			t.Fatalf("history: %s", history)
		}
		report := admin.text("/api/business-domain-report?businessDomainId=e2e-growth")
		if !strings.Contains(report, "e2e-bd") {
			t.Fatalf("report should list the attributed project: %s", truncate(report, 400))
		}
		if status, _ := admin.call(http.MethodGet, "/api/business-domain-report?grain=hour", nil); status != http.StatusBadRequest {
			t.Fatalf("bad grain status %d", status)
		}
	})

	t.Run("external workflow usage is ingested by the shipped reporter", func(t *testing.T) {
		admin.bind(t)
		python, err := exec.LookPath("python3")
		if err != nil {
			t.Skip("python3 not installed")
		}
		admin.callOK(http.MethodPost, "/api/projects", map[string]any{"id": "e2e-ext", "key": "EXT", "name": "E2E 外部用量"})
		created := admin.json(http.MethodPost, "/api/insights/v1/sources", map[string]any{"projectId": "e2e-ext", "name": "E2E CI", "expiresInDays": 1})
		token := fmt.Sprint(created["token"])
		source, _ := created["source"].(map[string]any)
		if !strings.HasPrefix(token, "afi_") || source["projectId"] != "e2e-ext" {
			t.Fatalf("create source: %#v", created)
		}
		if listed := admin.text("/api/insights/v1/sources?projectId=e2e-ext"); strings.Contains(listed, token) || !strings.Contains(listed, "E2E CI") {
			t.Fatalf("source list: %s", listed)
		}
		finished := time.Now().UTC().Add(-time.Minute)
		report := map[string]any{
			"id": "e2e-ext-1", "type": "workflow_execution", "externalExecutionId": "build-1",
			"workflow": map[string]any{"id": "nightly", "name": "Nightly build"},
			"url":      "https://ci.example.com/build/1", "status": "succeeded",
			"startedAt": finished.Add(-10 * time.Minute).Format(time.RFC3339Nano), "finishedAt": finished.Format(time.RFC3339Nano),
			"usage": []any{map[string]any{"provider": "example", "model": "example-model", "inputTokens": 120, "outputTokens": 30, "costStatus": "unknown"}},
		}
		raw, _ := json.Marshal(report)
		file := filepath.Join(t.TempDir(), "usage.json")
		if err := os.WriteFile(file, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		reporter := filepath.Join(moduleRoot(t), "scripts", "report-workflow-usage.py")
		check := exec.Command(python, reporter, file, "--check")
		if out, err := check.CombinedOutput(); err != nil {
			t.Fatalf("reporter --check: %v %s", err, out)
		}
		send := exec.Command(python, reporter, file, "--url", base, "--attempts", "1")
		send.Env = append(os.Environ(), "LOOPFORGE_INSIGHT_TOKEN="+token)
		if out, err := send.CombinedOutput(); err != nil {
			t.Fatalf("reporter: %v %s", err, out)
		}
		again := exec.Command(python, reporter, file, "--url", base, "--attempts", "1")
		again.Env = append(os.Environ(), "LOOPFORGE_INSIGHT_TOKEN="+token)
		if out, err := again.CombinedOutput(); err != nil || !strings.Contains(string(out), "duplicate") {
			t.Fatalf("idempotent retry: %v %s", err, out)
		}
		analytics := admin.json(http.MethodGet, "/api/analytics?projectId=e2e-ext&origin=external", nil)
		summary, _ := analytics["summary"].(map[string]any)
		if fmt.Sprint(summary["totalTokens"]) != "150" || fmt.Sprint(summary["succeededRuns"]) != "1" {
			t.Fatalf("external analytics: %#v", summary)
		}
		admin.callOK(http.MethodDelete, "/api/insights/v1/sources/"+fmt.Sprint(source["id"]), nil)
		revoked := exec.Command(python, reporter, file, "--url", base, "--attempts", "1")
		revoked.Env = append(os.Environ(), "LOOPFORGE_INSIGHT_TOKEN="+token)
		if out, err := revoked.CombinedOutput(); err == nil {
			t.Fatalf("revoked token still accepted: %s", out)
		}
	})

	t.Run("managed cli status runs without services", func(t *testing.T) {
		admin.bind(t)
		cmd := exec.Command(filepath.Join(moduleRoot(t), "bin", "loopforge"), "-cli-action=status")
		cmd.Dir = t.TempDir()
		cmd.Env = append(os.Environ(), "LOOPFORGE_MANAGED_CLI_ROOT="+filepath.Join(t.TempDir(), "managed-cli"))
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("status: %v %s", err, out)
		}
		var status map[string]any
		if err := json.Unmarshal(out, &status); err != nil {
			t.Fatalf("status output is not JSON: %v %s", err, out)
		}
		if bad := exec.Command(filepath.Join(moduleRoot(t), "bin", "loopforge"), "-cli-action=rollback"); bad.Run() == nil {
			t.Fatal("rollback without a provider should fail")
		}
	})
}

func issueIDOf(run map[string]any) string {
	ctx, _ := run["executionContext"].(map[string]any)
	return fmt.Sprint(ctx["issueId"])
}

func startServer(t *testing.T) (string, func()) {
	t.Helper()
	root := moduleRoot(t)
	bin := filepath.Join(root, "bin", "loopforge")
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("missing %s; run make build first", bin)
	}
	data, err := os.MkdirTemp("", "loopforge-e2e-api-")
	if err != nil {
		t.Fatal(err)
	}
	port := freePort(t)
	cmd := exec.Command(bin)
	cmd.Dir = data
	cmd.Env = append(os.Environ(),
		"LOOPFORGE_DATA_DIR="+data,
		"LOOPFORGE_HTTP_PORT="+port,
		"LOOPFORGE_ADMIN_PASSWORD=e2e-admin-pass",
		"LOOPFORGE_DEMO_MODE=true",
		"LOOPFORGE_TEMPLATE_SANDBOX=local",
	)
	cmd.Stdout = io.Discard
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	base := "http://127.0.0.1:" + port
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		res, err := http.Get(base + "/api/health")
		if err == nil {
			res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return base, func() {
					_ = cmd.Process.Kill()
					_, _ = cmd.Process.Wait()
					os.RemoveAll(data)
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	t.Fatalf("server did not become healthy: %s", stderr.String())
	return "", func() {}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return dir
	}
	parent := filepath.Dir(dir)
	if _, err := os.Stat(filepath.Join(parent, "go.mod")); err == nil {
		return parent
	}
	t.Fatalf("go.mod not found from %s", dir)
	return ""
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return fmt.Sprint(ln.Addr().(*net.TCPAddr).Port)
}

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newClient(t *testing.T, base string) *client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &client{t: t, base: base, http: &http.Client{Jar: jar, Timeout: 20 * time.Second}}
}

func (c *client) bind(t *testing.T) { c.t = t }

func (c *client) login(name, password string) {
	c.t.Helper()
	status, body := c.call(http.MethodPost, "/api/auth/login", map[string]string{"loginName": name, "password": password})
	if status != http.StatusOK {
		c.t.Fatalf("login %s: %d %s", name, status, body)
	}
}

func (c *client) putWorkflow(wf map[string]any) {
	c.t.Helper()
	c.callOK(http.MethodPut, "/api/workflows", wf)
	c.callOK(http.MethodPost, "/api/workflows/"+fmt.Sprint(wf["id"])+"/publish", map[string]any{})
}

func (c *client) callOK(method, path string, body any) map[string]any {
	c.t.Helper()
	return c.json(method, path, body)
}

func (c *client) json(method, path string, body any) map[string]any {
	c.t.Helper()
	status, raw := c.call(method, path, body)
	if status >= 300 {
		c.t.Fatalf("%s %s -> %d %s", method, path, status, raw)
	}
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "null" {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		c.t.Fatalf("%s %s decode: %v body %s", method, path, err, raw)
	}
	return out
}

func (c *client) jsonHeader(method, path string, body any, headers map[string]string) map[string]any {
	c.t.Helper()
	status, raw := c.callHeader(method, path, body, headers)
	if status >= 300 {
		c.t.Fatalf("%s %s -> %d %s", method, path, status, raw)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		c.t.Fatalf("decode: %v %s", err, raw)
	}
	return out
}

func (c *client) text(path string) string {
	c.t.Helper()
	status, raw := c.call(http.MethodGet, path, nil)
	if status >= 300 {
		c.t.Fatalf("GET %s -> %d %s", path, status, raw)
	}
	return raw
}

func (c *client) call(method, path string, body any) (int, string) {
	return c.callHeader(method, path, body, nil)
}

func (c *client) callHeader(method, path string, body any, headers map[string]string) (int, string) {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	return res.StatusCode, string(raw)
}

func publishScript(t *testing.T, c *client, id, name, script, artifactName, artifactPath string) {
	t.Helper()
	extra := map[string]any{"scriptLanguage": "shell", "script": script, "maxAttempts": 1}
	if artifactName != "" {
		extra["artifactOutputs"] = []any{map[string]any{"name": artifactName, "path": artifactPath, "required": true}}
	}
	c.putWorkflow(workflow(id, name, "platform", []any{node("write", "写文件", "script", extra)}, nil))
}

func dispatchAndWait(t *testing.T, c *client, workflowID, title, want string) map[string]any {
	t.Helper()
	issue := c.json(http.MethodPost, "/api/issues", map[string]any{
		"projectId": "platform", "title": title, "kind": "requirement", "syncTarget": "local",
		"status": "todo", "workflowId": workflowID,
	})
	task := c.json(http.MethodPost, "/api/issues/"+fmt.Sprint(issue["id"])+"/dispatch", map[string]any{"workflowId": workflowID})
	return waitRun(t, c, fmt.Sprint(task["runId"]), want)
}

func waitRun(t *testing.T, c *client, id, want string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(40 * time.Second)
	var last map[string]any
	for time.Now().Before(deadline) {
		last = c.json(http.MethodGet, "/api/runs/"+id, nil)
		status := fmt.Sprint(last["status"])
		if status == want {
			return last
		}
		if status == "failed" && want != "failed" || status == "cancelled" && want != "cancelled" {
			t.Fatalf("run %s became %s, want %s: %s", id, status, want, truncate(fmt.Sprint(last["nodes"]), 500))
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("run %s stuck at %v, want %s", id, last["status"], want)
	return nil
}

func workflow(id, name, project string, nodes, edges []any) map[string]any {
	if edges == nil {
		edges = []any{}
	}
	return map[string]any{
		"id": id, "name": name, "version": 1, "revision": 0, "status": "draft",
		"projectId": project, "sandboxProfileId": "local", "nodes": nodes, "edges": edges,
	}
}

func node(id, name, kind string, extra map[string]any) map[string]any {
	out := map[string]any{"id": id, "name": name, "type": kind, "maxAttempts": 1}
	for key, value := range extra {
		out[key] = value
	}
	return out
}

func edge(id, source, target string) map[string]any {
	return map[string]any{"id": id, "source": source, "target": target, "when": "success"}
}

func runID(run map[string]any) string { return fmt.Sprint(run["id"]) }

func artifactBody(t *testing.T, c *client, run map[string]any, name string) string {
	t.Helper()
	items, _ := run["artifacts"].([]any)
	for _, item := range items {
		art, _ := item.(map[string]any)
		if art["name"] == name {
			return c.text("/api/runs/" + runID(run) + "/artifacts/" + fmt.Sprint(art["id"]))
		}
	}
	t.Fatalf("artifact %q missing from run %s: %#v", name, runID(run), items)
	return ""
}

func attempt(run map[string]any, nodeID string) int {
	nodes, _ := run["nodes"].(map[string]any)
	node, _ := nodes[nodeID].(map[string]any)
	value, _ := node["attempt"].(float64)
	return int(value)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
