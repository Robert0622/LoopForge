package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// selfTestOptions configures `loopforge --self-test`, the end-to-end check a
// fresh deployment runs against itself: health, admin login, environment
// doctor, and (in demo mode) one demo requirement through dispatch, an
// approval via the public share link, and the delivery report artifact.
type selfTestOptions struct {
	baseURL  string
	user     string
	password string
	timeout  time.Duration
}

// demoAdminPassword is the fixed admin login seeded in demo mode so the README
// and teammates can sign in without reading the logs.
const demoAdminPassword = "demo1234"

type selfTestClient struct {
	base string
	http *http.Client
}

func defaultSelfTestURL() string {
	port := strings.TrimSpace(os.Getenv("LOOPFORGE_HTTP_PORT"))
	if port == "" {
		port = "8080"
	}
	return "http://127.0.0.1:" + port
}

func defaultSelfTestPassword() string {
	if password := strings.TrimSpace(os.Getenv("LOOPFORGE_ADMIN_PASSWORD")); password != "" {
		return password
	}
	if demo, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("LOOPFORGE_DEMO_MODE"))); demo {
		return demoAdminPassword
	}
	return ""
}

func runSelfTest(ctx context.Context, opts selfTestOptions, out io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()
	jar, _ := cookiejar.New(nil)
	c := &selfTestClient{base: strings.TrimRight(opts.baseURL, "/"), http: &http.Client{Timeout: 30 * time.Second, Jar: jar}}
	line := func(label, result string) { fmt.Fprintf(out, "%-14s %s\n", label, result) }

	if err := c.waitHealthy(ctx); err != nil {
		return fmt.Errorf("服务健康检查失败（%s）：%w", c.base, err)
	}
	line("服务健康", "ok")

	if opts.password == "" {
		return errors.New("缺少管理员密码：设置 LOOPFORGE_ADMIN_PASSWORD，或用 --self-test-password 传入")
	}
	if err := c.do(ctx, http.MethodPost, "/api/auth/login", map[string]string{"loginName": opts.user, "password": opts.password}, nil); err != nil {
		return fmt.Errorf("管理员 %s 登录失败：%w（数据卷是旧的？首次创建后密码不会被 .env 覆盖，用 ./start.sh reset 清空重来）", opts.user, err)
	}
	line("管理员登录", opts.user)

	var doctor struct {
		Ready  bool `json:"ready"`
		Checks []struct {
			Label  string `json:"label"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/doctor", nil, &doctor); err != nil {
		return fmt.Errorf("环境自检接口失败：%w", err)
	}
	for _, check := range doctor.Checks {
		mark := map[string]string{"ok": "✓", "warn": "!", "error": "✗", "info": "·"}[check.Status]
		fmt.Fprintf(out, "  %s %s：%s\n", mark, check.Label, check.Detail)
	}
	if !doctor.Ready {
		return errors.New("环境自检有阻塞项（见上面 ✗）")
	}

	var workflows []struct {
		ID string `json:"id"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/workflows", nil, &workflows); err != nil {
		return fmt.Errorf("读取 SOP 列表失败：%w", err)
	}
	hasDemo := false
	for _, w := range workflows {
		hasDemo = hasDemo || w.ID == "demo-delivery"
	}
	if !hasDemo {
		line("演示流程", "跳过（LOOPFORGE_DEMO_MODE 未开启）")
		return nil
	}
	return c.runDemo(ctx, opts, line)
}

func (c *selfTestClient) runDemo(ctx context.Context, opts selfTestOptions, line func(string, string)) error {
	var projects []struct {
		ID         string `json:"id"`
		WorkflowID string `json:"workflowId"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/projects", nil, &projects); err != nil || len(projects) == 0 {
		return fmt.Errorf("没有可用项目：%v", err)
	}
	projectID := projects[0].ID
	for _, p := range projects {
		if p.WorkflowID == "demo-delivery" {
			projectID = p.ID
			break
		}
	}
	var issue struct {
		ID string `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/issues", map[string]any{
		"projectId": projectID, "title": "自检：演示需求", "description": "loopforge --self-test 创建的演示需求，跑完后自动关闭。",
		"kind": "requirement", "priority": "low", "status": "todo", "syncTarget": "local", "workflowId": "demo-delivery",
	}, &issue); err != nil {
		return fmt.Errorf("创建演示需求失败：%w", err)
	}
	var task struct {
		RunID string `json:"runId"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/issues/"+url.PathEscape(issue.ID)+"/dispatch", map[string]string{"workflowId": "demo-delivery"}, &task); err != nil || task.RunID == "" {
		return fmt.Errorf("派发演示需求失败：%v", err)
	}
	line("派发演示需求", task.RunID)
	started := time.Now()

	if _, err := c.waitRun(ctx, task.RunID, "waiting"); err != nil {
		return fmt.Errorf("等待审批节点失败：%w", err)
	}
	line("停在审批节点", "waiting")

	var share struct {
		URL string `json:"url"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/runs/"+url.PathEscape(task.RunID)+"/share-gate", map[string]string{}, &share); err != nil {
		return fmt.Errorf("生成审批分享链接失败：%w", err)
	}
	parsed, err := url.Parse(share.URL)
	if err != nil || parsed.Query().Get("token") == "" {
		return fmt.Errorf("分享链接格式不对：%q", share.URL)
	}
	anonymous := &selfTestClient{base: c.base, http: &http.Client{Timeout: 30 * time.Second}}
	if err := anonymous.do(ctx, http.MethodGet, "/share/gate?token="+url.QueryEscape(parsed.Query().Get("token")), nil, nil); err != nil {
		return fmt.Errorf("免登录审批页打不开：%w", err)
	}
	if err := anonymous.do(ctx, http.MethodPost, "/api/share/gate/decide", map[string]string{"token": parsed.Query().Get("token"), "decision": "approve", "name": "自检"}, nil); err != nil {
		return fmt.Errorf("通过分享链接批准失败：%w", err)
	}
	line("免登录批准", "approved")

	run, err := c.waitRun(ctx, task.RunID, "succeeded")
	if err != nil {
		return fmt.Errorf("等待运行完成失败：%w", err)
	}
	line("运行完成", fmt.Sprintf("succeeded（%s）", time.Since(started).Round(time.Second)))

	for _, artifact := range run.Artifacts {
		if strings.HasSuffix(artifact.Path, "delivery-report.md") || artifact.Name == "交付报告" {
			line("交付报告", artifact.Path)
			// Succeeded requirements cannot be deleted; close it so the list stays clean.
			if err := c.do(ctx, http.MethodPost, "/api/issues/"+url.PathEscape(issue.ID)+"/close", map[string]string{}, nil); err == nil {
				line("自检需求", "已关闭（保留在需求列表的已关闭里，可作为证据）")
			}
			return nil
		}
	}
	return errors.New("运行成功但没有找到 delivery-report.md 产物")
}

type selfTestRun struct {
	Status    string `json:"status"`
	Artifacts []struct {
		Name string `json:"name"`
		Path string `json:"path"`
	} `json:"artifacts"`
	Events []struct {
		Message string `json:"message"`
	} `json:"events"`
}

func (c *selfTestClient) waitRun(ctx context.Context, runID, want string) (selfTestRun, error) {
	for {
		var run selfTestRun
		if err := c.do(ctx, http.MethodGet, "/api/runs/"+url.PathEscape(runID), nil, &run); err != nil {
			return run, err
		}
		if run.Status == want {
			return run, nil
		}
		if run.Status == "failed" || run.Status == "canceled" {
			last := ""
			if n := len(run.Events); n > 0 {
				last = "：" + run.Events[n-1].Message
			}
			return run, fmt.Errorf("运行状态 %s%s", run.Status, last)
		}
		select {
		case <-ctx.Done():
			return run, fmt.Errorf("超时，当前状态 %q", run.Status)
		case <-time.After(time.Second):
		}
	}
}

func (c *selfTestClient) waitHealthy(ctx context.Context) error {
	var last error
	for {
		if last = c.do(ctx, http.MethodGet, "/api/health", nil, nil); last == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return last
		case <-time.After(time.Second):
		}
	}
}

func (c *selfTestClient) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
		return fmt.Errorf("%s %s → HTTP %d %s", method, path, resp.StatusCode, msg)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}
