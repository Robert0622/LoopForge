package engine

import (
	"fmt"
	"strings"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/repo"
)

func renderPrompt(wf domain.WorkflowDefinition, node domain.WorkflowNode, run domain.Run) string {
	return renderPromptWithStore(nil, wf, node, run)
}

func renderPromptSpec(wf domain.WorkflowDefinition, node domain.WorkflowNode, spec domain.TaskPromptSpec, run domain.Run) string {
	return applyPromptPlaceholders(nil, wf, node, &spec, nil, run)
}

func renderPromptWithStore(st *repo.Store, wf domain.WorkflowDefinition, node domain.WorkflowNode, run domain.Run) string {
	var spec *domain.TaskPromptSpec
	var gates []domain.VerificationGate
	if st != nil {
		if agent, ok := workflowAgent(st, wf, node.AgentID); ok {
			gates = agent.Verification
			spec = agent.TaskPrompt
		}
	}
	return applyPromptPlaceholders(st, wf, node, spec, gates, run)
}

func applyPromptPlaceholders(st *repo.Store, wf domain.WorkflowDefinition, node domain.WorkflowNode, spec *domain.TaskPromptSpec, gates []domain.VerificationGate, run domain.Run) string {
	prompt := ""
	if spec != nil {
		prompt = compileTaskPrompt(node, *spec, gates)
	}
	if input := strings.TrimSpace(run.Input); input != "" && !strings.Contains(prompt, "{{input}}") {
		// Agents without a task brief (or whose brief omits {{input}}) must still
		// receive the requirement itself, not just its identity block.
		if prompt == "" {
			prompt = "## 任务输入\n\n{{input}}\n"
			if hasUpstreamOutput(wf, node, run) {
				prompt += "\n## 上游节点结果\n\n{{upstream}}\n"
			}
		} else {
			prompt = "## 任务输入\n\n{{input}}\n\n" + prompt
		}
	}
	prompt = strings.ReplaceAll(prompt, "{{input}}", run.Input)
	repository := "未绑定代码仓库"
	if run.Repository.RepositoryID != "" {
		repository = fmt.Sprintf("%s (%s), 最新远端基线 %s, 本次工作分支 %s", run.Repository.Name, run.Repository.RepoPath, run.Repository.BaseBranch, run.Repository.WorkBranch)
	}
	prompt = strings.ReplaceAll(prompt, "{{repository}}", repository)
	pipelineURL := "未配置 Pipeline"
	if run.Repository.PipelineURLPattern != "" {
		pipelineURL = strings.ReplaceAll(run.Repository.PipelineURLPattern, "{build_id}", "current")
	}
	prompt = strings.ReplaceAll(prompt, "{{pipelineUrl}}", pipelineURL)
	prompt = strings.ReplaceAll(prompt, "{{existingMR}}", existingMRBlurb(st, run))
	rejectTo := node.RejectTo
	if rejectTo == "" {
		rejectTo = "上游实现节点"
	}
	prompt = strings.ReplaceAll(prompt, "{{rejectTo}}", rejectTo)
	subject := "本次任务"
	switch {
	case strings.Contains(wf.ID, "issue"):
		subject = "本次 Issue"
	case strings.Contains(wf.ID, "requirement"):
		subject = "本次需求"
	}
	prompt = strings.ReplaceAll(prompt, "{{subject}}", subject)
	feedback := ""
	if current := run.Nodes[node.ID]; current != nil {
		feedback = current.Feedback
	}
	if strings.TrimSpace(feedback) != "" && !strings.Contains(prompt, "{{feedback}}") {
		prompt += "\n\n## 上一轮退回意见\n\n审核人把这一步退回重做，请按以下意见修改后再提交：\n\n" + feedback + "\n"
	}
	prompt = strings.ReplaceAll(prompt, "{{feedback}}", feedback)
	humanNote := strings.TrimSpace(run.HumanContext)
	prompt = strings.ReplaceAll(prompt, "{{humanContext}}", humanNote)
	if humanNote != "" {
		prompt += "\n\n## 人工补充上下文\n\n操作员在审核或重试时补充了以下信息，请优先采信并据此行动：\n\n" + humanNote + "\n"
	}
	if st != nil {
		if issueID := strings.TrimSpace(run.ExecutionContext.IssueID); issueID != "" {
			if issue, ok := st.Issue(issueID); ok {
				if block := issue.IdentityBlock(); block != "" && !issue.HasIdentityBlock(prompt) {
					prompt += "\n\n## 需求身份\n\n" + block + "\n"
				}
			}
		}
	}
	upstream := []string{}
	for _, edge := range wf.Edges {
		if edge.Target == node.ID {
			if source := run.Nodes[edge.Source]; source != nil && source.Output != "" {
				upstream = append(upstream, fmt.Sprintf("[%s]\n%s", edge.Source, source.Output))
			}
		}
	}
	prompt = strings.ReplaceAll(prompt, "{{upstream}}", strings.Join(upstream, "\n\n"))
	for id, nodeRun := range run.Nodes {
		prompt = strings.ReplaceAll(prompt, "{{node."+id+".output}}", nodeRun.Output)
	}
	for _, artifact := range run.Artifacts {
		prompt = strings.ReplaceAll(prompt, "{{artifact."+artifact.NodeID+"."+artifact.Name+"}}", artifactPromptBody(artifact))
	}
	return prompt
}

func hasUpstreamOutput(wf domain.WorkflowDefinition, node domain.WorkflowNode, run domain.Run) bool {
	for _, edge := range wf.Edges {
		if edge.Target == node.ID {
			if source := run.Nodes[edge.Source]; source != nil && source.Output != "" {
				return true
			}
		}
	}
	return false
}

func existingMRBlurb(st *repo.Store, run domain.Run) string {
	const searchHint = "本轮 SOP 使用独立工作分支。只用当前分支创建或更新本轮 PR，禁止把 commit 推到上一轮 SOP 的分支，也禁止 update 上一轮 PR。"
	clip := func(content string) string {
		content = strings.TrimSpace(content)
		if len(content) > 2500 {
			return content[:2500] + "…"
		}
		return content
	}
	if art, ok := latestNamedArtifact(run, "mr", "mr-body"); ok && strings.Contains(art.Content, "/pull/") {
		return "已有本轮 PR，在本轮工作分支上更新，禁止另开分支：\n" + clip(art.Content) + "\n" + searchHint
	}
	_ = st
	return "本轮还没有 PR。对当前工作分支 git push 后创建新 PR。"
}

func latestNamedArtifact(run domain.Run, nodeID, name string) (domain.Artifact, bool) {
	var best domain.Artifact
	found := false
	for _, a := range run.Artifacts {
		if a.NodeID != nodeID || a.Name != name {
			continue
		}
		if !found || a.Attempt > best.Attempt || (a.Attempt == best.Attempt && a.CreatedAt.After(best.CreatedAt)) {
			best = a
			found = true
		}
	}
	return best, found
}

func compileTaskPrompt(node domain.WorkflowNode, spec domain.TaskPromptSpec, gates []domain.VerificationGate) string {
	var b strings.Builder
	b.WriteString("# 节点任务书\n\n")
	writePromptSection(&b, "执行角色", spec.Role)
	writePromptSection(&b, "任务目标", spec.Objective)
	writePromptSection(&b, "工作上下文", spec.Context)
	writePromptList(&b, "输入", spec.Inputs)
	writePromptList(&b, "执行动作", spec.Actions)
	deliverables := spec.Deliverables
	if len(deliverables) == 0 {
		for _, artifact := range node.ArtifactOutputs {
			deliverables = append(deliverables, artifact.Name+"：在工作区创建 `"+artifact.Path+"`")
		}
	}
	criteria := spec.AcceptanceCriteria
	if len(criteria) == 0 {
		for _, gate := range gates {
			criteria = append(criteria, gate.Name+"必须通过，并保留真实命令证据")
		}
	}
	writePromptList(&b, "交付物", deliverables)
	writePromptList(&b, "完成条件与门禁证据", criteria)
	writePromptList(&b, "约束", spec.Constraints)
	b.WriteString("## 失败反馈与重试\n\n")
	strategy := map[string]string{"full_redo": "完整重做", "targeted_fix": "仅修复失败项"}[node.RetryStrategy]
	if strategy == "" {
		strategy = "仅修复失败项"
	}
	fmt.Fprintf(&b, "- 策略：%s\n", strategy)
	if len(node.CarryOver) > 0 {
		fmt.Fprintf(&b, "- 继承上下文：%s\n", strings.Join(node.CarryOver, "、"))
	}
	if spec.RetryInstructions != "" {
		fmt.Fprintf(&b, "- 重试要求：%s\n", spec.RetryInstructions)
	}
	b.WriteString("- 上一轮反馈：{{feedback}}\n")
	return b.String()
}

func writePromptSection(b *strings.Builder, title, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	fmt.Fprintf(b, "## %s\n\n%s\n\n", title, value)
}

func writePromptList(b *strings.Builder, title string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(b, "## %s\n\n", title)
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			fmt.Fprintf(b, "- %s\n", value)
		}
	}
	b.WriteString("\n")
}

func appendArtifactContract(prompt string, node domain.WorkflowNode, repositories []domain.RepositoryContext) string {
	var b strings.Builder
	b.WriteString(prompt)
	var checkouts []domain.RepositoryContext
	for _, rc := range repositories {
		if strings.TrimSpace(rc.WorkspaceDir) != "" {
			checkouts = append(checkouts, rc)
		}
	}
	if len(checkouts) > 0 {
		b.WriteString("\n\n# Workspace layout\n\nRepository checkouts live in first-level subdirectories. Write code changes inside those directories. SOP deliverables such as `fix-plan.md` go at the run workspace root (the current working directory), never inside a repository directory.\n")
		for _, rc := range checkouts {
			name := strings.TrimSpace(rc.Name)
			if name == "" {
				name = rc.RepositoryID
			}
			fmt.Fprintf(&b, "- `./%s`: %s\n", rc.WorkspaceDir, name)
		}
	}
	if len(node.ArtifactOutputs) == 0 {
		return b.String()
	}
	b.WriteString("\n\n# Required artifact contract\n\nBefore finishing, create these files as **relative paths at the run workspace root** (the current working directory for run-scoped nodes). Do not write them inside a repository subdirectory. Never write them to a host absolute path, `localPath`, or `/data/workspace/...`.\n")
	for _, contract := range node.ArtifactOutputs {
		required := "optional"
		if contract.Required {
			required = "required"
		}
		fmt.Fprintf(&b, "- `%s`: %s (%s, %s)\n", contract.Path, contract.Name, contract.MediaType, required)
	}
	return b.String()
}
