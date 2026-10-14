package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/gin-gonic/gin"
)

// A crew mission runs at most this many bot steps, including steps people add
// by @-ing a bot while it runs.
const crewMaxTasks = 8

const crewCaptain = "LoopForge"

type crewRequest struct {
	Goal      string `json:"goal"`
	Detail    string `json:"detail"`
	AutoStart bool   `json:"autoStart"`
	Source    string `json:"source"`
}

type crewTaskInput struct {
	ID          string `json:"id"`
	AgentID     string `json:"agentId"`
	Title       string `json:"title"`
	Instruction string `json:"instruction"`
}

type crewActionRequest struct {
	Action string          `json:"action"` // start, stop, retry, skip, plan, sop
	Tasks  []crewTaskInput `json:"tasks"`
}

// createCrew hands open-ended work to the room's bots. The captain plans in
// the background; the mission card shows "planning" until the plan lands.
func (h *roomsHandlers) createCrew(c *gin.Context) {
	room, ok := h.repo.Room(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return
	}
	if !h.roomAccess(c, room.ProjectID, true) {
		return
	}
	var req crewRequest
	if !decode(c, &req) {
		return
	}
	userID, author := h.roomActor(c)
	msg, err := h.startCrewMission(room, userID, author, req)
	if err != nil {
		respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, msg)
}

func (h *roomsHandlers) startCrewMission(room domain.ChatRoom, userID int64, author string, req crewRequest) (domain.ChatMessage, error) {
	goal := strings.TrimSpace(req.Goal)
	detail := strings.TrimSpace(req.Detail)
	if goal == "" {
		return domain.ChatMessage{}, HTTPError{http.StatusBadRequest, "说一下要做什么"}
	}
	if len([]rune(goal)) > 200 {
		goal = string([]rune(goal)[:200])
	}
	if len([]rune(detail)) > 8000 {
		return domain.ChatMessage{}, HTTPError{http.StatusBadRequest, "描述过长"}
	}
	if len(h.crewRoster(room.ProjectID, userID)) == 0 {
		return domain.ChatMessage{}, HTTPError{http.StatusBadRequest, "这个项目还没有能干活的角色 Bot，先去「构建 · 角色」建一个"}
	}
	issue := domain.Issue{
		ID: newChatID("iss"), ProjectID: room.ProjectID, Title: goal, Description: detail,
		Status: "todo", Priority: "medium", Kind: "issue", Source: "local",
	}
	if userID > 0 {
		issue.AssigneeUserID = userID
	}
	issue.AppendOperation(domain.IssueOperation{Actor: author, ActorUserID: userID, Action: "create", Detail: "交给协作群「" + room.Name + "」的 Bot 团队自由协作"})
	if err := h.repo.PutIssue(issue); err != nil {
		return domain.ChatMessage{}, err
	}
	stored, _ := h.repo.Issue(issue.ID)
	source := req.Source
	if source != "home" && source != "chat" {
		source = "room"
	}
	now := time.Now().UTC()
	mission := domain.CrewMission{
		ID: newChatID("crew"), Goal: goal, Detail: detail, Status: "planning",
		IssueID: stored.ID, IssueNumber: stored.Number, AutoStart: req.AutoStart,
		StartedBy: userID, StartedByName: author, Source: source, CreatedAt: now, UpdatedAt: now,
	}
	msg := domain.ChatMessage{
		ID: mission.ID, RoomID: room.ID, UserID: userID, Author: author, Kind: domain.ChatKindCrew,
		Body: goal, IssueID: stored.ID, IssueNumber: stored.Number, IssueTitle: stored.Title,
		Crew: &mission, CreatedAt: now,
	}
	if err := h.repo.PutMessage(msg); err != nil {
		return domain.ChatMessage{}, err
	}
	h.touchRoom(room)
	go h.planCrew(room, msg.ID)
	return msg, nil
}

// crewRoster lists the roles a crew may use: visible to the project and
// runnable by this user (the mock executor always is; real CLIs need a key,
// the engine falls back between built-in CLIs).
func (h *roomsHandlers) crewVisible(projectID string) []domain.AgentDefinition {
	var out []domain.AgentDefinition
	for _, a := range h.repo.Agents() {
		if catalogVisible(a.ProjectID, a.Shared, projectID) {
			out = append(out, a)
		}
	}
	return out
}

func (h *roomsHandlers) reassignUnavailableTasks(projectID string, userID int64, tasks []domain.CrewTask) []domain.CrewTask {
	statuses := h.botStatuses(projectID, userID)
	byID := map[string]botStatusView{}
	var fallback *botStatusView
	for i := range statuses {
		byID[statuses[i].AgentID] = statuses[i]
		if fallback == nil && statuses[i].Status != "unavailable" {
			fb := statuses[i]
			fallback = &fb
		}
	}
	for i := range tasks {
		st, ok := byID[tasks[i].AgentID]
		if !ok || st.Status != "unavailable" || fallback == nil || fallback.AgentID == tasks[i].AgentID {
			continue
		}
		tasks[i].ReassignedFrom = tasks[i].AgentName
		tasks[i].ReassignedReason = st.Reason
		tasks[i].AgentID = fallback.AgentID
		tasks[i].AgentName = fallback.Name
	}
	return tasks
}

// annotateIdealSkip records when the best-named bot for a step cannot run,
// even if the planner already picked someone who can. The card then says
// 「已改由 yy 负责」.
func (h *roomsHandlers) annotateIdealSkip(goal string, tasks []domain.CrewTask, visible []domain.AgentDefinition, userID int64, projectID string) {
	_ = goal
	statuses := map[string]botStatusView{}
	for _, st := range h.botStatuses(projectID, userID) {
		statuses[st.AgentID] = st
	}
	for i := range tasks {
		if tasks[i].ReassignedFrom != "" {
			continue
		}
		k := -1
		for n, kind := range crewRoleKinds {
			if kind.title == tasks[i].Title {
				k = n
				break
			}
		}
		if k < 0 {
			continue
		}
		best, bestScore := -1, 0
		for j, a := range visible {
			score := crewRoleScore(a, k)
			if score == 0 {
				continue
			}
			better := score > bestScore
			tieReal := score == bestScore && best >= 0 && visible[best].ExecutorID == "mock" && a.ExecutorID != "mock"
			if better || tieReal {
				best, bestScore = j, score
			}
		}
		if best < 0 {
			continue
		}
		ideal := visible[best]
		if ideal.ID == tasks[i].AgentID {
			continue
		}
		st := statuses[ideal.ID]
		if st.Status != "unavailable" {
			continue
		}
		tasks[i].ReassignedFrom = crewAgentName(ideal)
		tasks[i].ReassignedReason = firstNonEmpty(st.Reason, "不可用")
	}
}

func (h *roomsHandlers) crewRoster(projectID string, userID int64) []domain.AgentDefinition {
	hasKey := false
	if user, ok := h.repo.User(userID); ok {
		hasKey = strings.TrimSpace(user.CodeBuddyAPIKey) != "" || strings.TrimSpace(user.ClaudeCodeAPIKey) != "" || strings.TrimSpace(user.CodexAPIKey) != ""
	}
	var out []domain.AgentDefinition
	for _, a := range h.repo.Agents() {
		if !catalogVisible(a.ProjectID, a.Shared, projectID) {
			continue
		}
		if a.ExecutorID != "mock" && !hasKey {
			continue
		}
		out = append(out, a)
	}
	return out
}

func crewAgentName(a domain.AgentDefinition) string {
	name := strings.TrimSpace(a.Name)
	for _, prefix := range []string{"演示 · ", "演示·", "演示 "} {
		name = strings.TrimPrefix(name, prefix)
	}
	if name == "" {
		name = a.ID
	}
	return name
}

var crewRoleKinds = []struct {
	kind  string
	re    *regexp.Regexp
	title string
	brief string
}{
	{"analyze", regexp.MustCompile(`(?i)分析|需求|拆解|方案|analy|requirement|prd|shape|plan`), "拆解需求、定方案", "先把目标拆成可执行的要点，列出范围、风险和验收标准，给出建议方案。"},
	{"build", regexp.MustCompile(`(?i)开发|实现|编码|修复|develop|implement|build|code|engineer`), "动手实现", "按前面的方案完成实现，说明改了什么、为什么这样改。"},
	{"review", regexp.MustCompile(`(?i)审查|评审|review|audit`), "独立审查", "独立审查前面的产出，指出问题和风险，给出是否可以交付的结论。"},
	{"test", regexp.MustCompile(`(?i)测试|验证|test|verify|qa`), "测试验证", "设计并执行验证，列出覆盖的场景和结果。"},
	{"docs", regexp.MustCompile(`(?i)文档|说明|doc|write`), "整理文档", "把结论整理成可以直接给人看的文档。"},
}

// crewRoleScore rates how well a role fits a kind: its name says the most,
// the id a little less, the description only breaks ties.
func crewRoleScore(a domain.AgentDefinition, k int) int {
	re := crewRoleKinds[k].re
	score := 0
	if re.MatchString(crewAgentName(a)) {
		score += 4
	}
	if re.MatchString(a.ID) {
		score += 2
	}
	if re.MatchString(a.Description) {
		score++
	}
	return score
}

// crewPlanRules is the captain's plan when no model is available: walk the
// kinds in analyze → build → review → test → docs order and give each to the
// best-fitting role not used yet. Docs only join when the goal asks for them.
func crewPlanRules(goal string, roster []domain.AgentDefinition) []domain.CrewTask {
	used := map[string]bool{}
	var tasks []domain.CrewTask
	wantsDocs := regexp.MustCompile(`(?i)文档|说明|doc`).MatchString(goal)
	for k := range crewRoleKinds {
		if crewRoleKinds[k].kind == "docs" && !wantsDocs {
			continue
		}
		best, bestScore := -1, 0
		for i, a := range roster {
			if used[a.ID] {
				continue
			}
			score := crewRoleScore(a, k) * 2
			if score > 0 && a.ExecutorID == "mock" {
				score++
			}
			if score > bestScore {
				best, bestScore = i, score
			}
		}
		if best < 0 {
			continue
		}
		a := roster[best]
		used[a.ID] = true
		tasks = append(tasks, domain.CrewTask{AgentID: a.ID, AgentName: crewAgentName(a), Title: crewRoleKinds[k].title, Instruction: crewRoleKinds[k].brief, AddedBy: crewCaptain})
		if len(tasks) == 4 {
			break
		}
	}
	if len(tasks) == 0 {
		for i, a := range roster {
			if i == 2 {
				break
			}
			tasks = append(tasks, domain.CrewTask{AgentID: a.ID, AgentName: crewAgentName(a), Title: map[bool]string{true: "先做一版", false: "接着完善"}[i == 0], Instruction: "围绕目标完成你职责范围内的部分。", AddedBy: crewCaptain})
		}
	}
	return tasks
}

const crewPlanPrompt = `你是 LoopForge 协作群里的队长 Bot。用户把一件没有固定流程的事交给了群里的 Bot 团队。
请从下面的角色里挑人，排出 2 到 5 步的分工，每一步由一个角色完成，前一步的产出会交给后一步。
只输出一个 JSON 对象，不要任何其它文字：
{"tasks":[{"agentId":"角色 id","title":"这一步做什么（10 字以内）","instruction":"给这个 Bot 的具体要求（1-3 句）"}]}

目标：%s
补充说明：%s

可用角色：
%s`

func (h *roomsHandlers) crewPlanAI(goal, detail string, roster []domain.AgentDefinition, userID int64) ([]domain.CrewTask, error) {
	user, ok := h.repo.User(userID)
	if !ok || strings.TrimSpace(user.CodeBuddyAPIKey) == "" {
		return nil, fmt.Errorf("no model key")
	}
	byID := map[string]domain.AgentDefinition{}
	var lines []string
	for _, a := range roster {
		byID[a.ID] = a
		lines = append(lines, fmt.Sprintf("- %s：%s。%s", a.ID, crewAgentName(a), roomTrim(a.Description, 120)))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	raw, err := generateAssistantReply(ctx, fmt.Sprintf(crewPlanPrompt, goal, firstNonEmpty(detail, "无"), strings.Join(lines, "\n")), user)
	if err != nil {
		return nil, err
	}
	start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("plan is not JSON")
	}
	var plan struct {
		Tasks []crewTaskInput `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), &plan); err != nil {
		return nil, err
	}
	var tasks []domain.CrewTask
	for _, t := range plan.Tasks {
		a, ok := byID[strings.TrimSpace(t.AgentID)]
		if !ok || strings.TrimSpace(t.Title) == "" {
			continue
		}
		tasks = append(tasks, domain.CrewTask{AgentID: a.ID, AgentName: crewAgentName(a), Title: roomTrim(strings.TrimSpace(t.Title), 40), Instruction: roomTrim(strings.TrimSpace(t.Instruction), 600), AddedBy: crewCaptain})
		if len(tasks) == 5 {
			break
		}
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("empty plan")
	}
	return tasks, nil
}

func (h *roomsHandlers) planCrew(room domain.ChatRoom, msgID string) {
	msg, ok := h.repo.Message(msgID)
	if !ok || msg.Crew == nil {
		return
	}
	m := msg.Crew
	roster := h.crewRoster(room.ProjectID, m.StartedBy)
	tasks, err := h.crewPlanAI(m.Goal, m.Detail, roster, m.StartedBy)
	planner := "ai"
	if err != nil {
		tasks, planner = crewPlanRules(m.Goal+" "+m.Detail, roster), "rules"
	}
	tasks = h.reassignUnavailableTasks(room.ProjectID, m.StartedBy, tasks)
	h.annotateIdealSkip(m.Goal+" "+m.Detail, tasks, h.crewVisible(room.ProjectID), m.StartedBy, room.ProjectID)
	h.crewMu.Lock()
	defer h.crewMu.Unlock()
	msg, ok = h.repo.Message(msgID)
	if !ok || msg.Crew == nil || msg.Crew.Status != "planning" {
		return
	}
	m = msg.Crew
	for i := range tasks {
		tasks[i].ID = fmt.Sprintf("t%d", i+1)
		tasks[i].Status = "pending"
	}
	m.Tasks, m.Planner = tasks, planner
	if len(tasks) == 0 {
		m.Status, m.Error = "failed", "没有可用的角色 Bot"
		h.saveCrew(msg)
		return
	}
	m.Status = "proposed"
	if m.AutoStart {
		m.Status = "running"
	}
	h.saveCrew(msg)
	names := make([]string, len(tasks))
	for i, t := range tasks {
		names[i] = t.AgentName + "「" + t.Title + "」"
	}
	lead := "收到。我拉了 " + fmt.Sprint(len(crewDistinctAgents(tasks))) + " 个 Bot 一起做：" + strings.Join(names, " → ") + "。"
	for _, t := range tasks {
		if t.ReassignedFrom != "" {
			lead += "\n已改由 " + t.AgentName + " 负责（" + t.ReassignedFrom + " " + t.ReassignedReason + "）。"
		}
	}
	if m.AutoStart {
		lead += "已经开工，做完一步我会在群里交接。"
	} else {
		lead += "分工可以先改，确认后点「开工」。"
	}
	h.crewSay(msg, crewCaptain, "", lead, "", "")
	if m.AutoStart {
		h.ensureCrewLoop()
	}
}

func crewDistinctAgents(tasks []domain.CrewTask) map[string]bool {
	out := map[string]bool{}
	for _, t := range tasks {
		out[t.AgentID] = true
	}
	return out
}

func (h *roomsHandlers) saveCrew(msg domain.ChatMessage) {
	msg.Crew.UpdatedAt = time.Now().UTC()
	_ = h.repo.PutMessage(msg)
	if room, ok := h.repo.Room(msg.RoomID); ok {
		h.touchRoom(room)
	}
}

// crewSay posts a line in the room as a bot (agentID set) or the captain.
// handoffTo / handoffTitle mark "handing over to X for Y".
func (h *roomsHandlers) crewSay(mission domain.ChatMessage, author, agentID, body, handoffTo, handoffTitle string) {
	post := domain.ChatMessage{
		ID: newChatID("msg"), RoomID: mission.RoomID, Author: author, AgentID: agentID,
		Kind: domain.ChatKindCrewPost, Body: body, CrewID: mission.ID,
		IssueID: mission.IssueID, IssueNumber: mission.IssueNumber, IssueTitle: mission.IssueTitle,
		TargetName: handoffTo, NodeName: handoffTitle, CreatedAt: time.Now().UTC(),
	}
	_ = h.repo.PutMessage(post)
}

// ensureCrewLoop runs the captain while any mission is active. It stops after
// a few idle ticks; every create, action and room poll restarts it.
func (h *roomsHandlers) ensureCrewLoop() {
	if !h.crewLoopOn.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer h.crewLoopOn.Store(false)
		idle := 0
		for idle < 5 {
			time.Sleep(3 * time.Second)
			if h.tickCrews() == 0 {
				idle++
			} else {
				idle = 0
			}
		}
	}()
}

func (h *roomsHandlers) tickCrews() int {
	active := 0
	for _, msg := range h.repo.CrewMessages("") {
		if !domain.CrewMissionActive(msg.Crew.Status) {
			continue
		}
		active++
		if msg.Crew.Status == "running" {
			h.advanceCrew(msg.ID)
		}
	}
	return active
}

var crewQuestionRE = regexp.MustCompile(`(?m)^\s*(?:[-*>]\s*)?(?:\*\*)?(?:需要确认|需要你确认|NEED_CONFIRM)(?:\*\*)?\s*[:：]\s*(.+)$`)

func crewQuestion(output string) string {
	if m := crewQuestionRE.FindStringSubmatch(output); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// crewSummary keeps a bot's headline plus a few points for the room post.
func crewSummary(output string) string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "```") || strings.HasPrefix(line, "|") {
			continue
		}
		line = strings.TrimSpace(strings.TrimLeft(line, "#>"))
		line = strings.ReplaceAll(line, "**", "")
		if line == "" {
			continue
		}
		lines = append(lines, line)
		if len(lines) == 6 {
			break
		}
	}
	return roomTrim(strings.Join(lines, "\n"), 700)
}

func (h *roomsHandlers) advanceCrew(msgID string) {
	h.crewMu.Lock()
	defer h.crewMu.Unlock()
	msg, ok := h.repo.Message(msgID)
	if !ok || msg.Crew == nil || msg.Crew.Status != "running" {
		return
	}
	m := msg.Crew
	changed := false
	busy := false
	for i := range m.Tasks {
		t := &m.Tasks[i]
		if t.Status != "running" {
			continue
		}
		run, found := h.repo.Run(t.RunID)
		if !found || !roomRunTerminal(run.Status) {
			busy = true
			continue
		}
		changed = true
		t.FinishedAt = time.Now().UTC()
		node := run.Nodes["work"]
		if run.Status == "succeeded" {
			output := ""
			if node != nil {
				output = node.Output
			}
			t.Status, t.Output, t.Summary = "succeeded", roomTrim(output, 6000), crewSummary(output)
			t.Handoff = parseCrewHandoff(output, t.Summary)
			h.clearBotDegraded(t.AgentID)
			if q := crewQuestion(output); q != "" {
				m.Status, m.Question, m.QuestionAgent = "waiting", q, t.AgentID
				h.crewSay(msg, t.AgentName, t.AgentID, firstNonEmpty(t.Summary, "做完了「"+t.Title+"」。"), "", "")
				h.crewSay(msg, crewCaptain, "", "@"+crewMention(m.StartedByName)+" "+t.AgentName+" 需要你确认："+q+"\n直接在群里回复，我会交回给它继续。", "", "")
				continue
			}
			next := crewNextPending(m)
			to, title := "", ""
			if next != nil {
				to, title = next.AgentName, next.Title
			}
			h.crewSay(msg, t.AgentName, t.AgentID, firstNonEmpty(t.Summary, "做完了「"+t.Title+"」。"), to, title)
			continue
		}
		reason := run.Status
		if node != nil && strings.TrimSpace(node.Error) != "" {
			reason = node.Error
		}
		t.Status, t.Error = "failed", roomTrim(reason, 400)
		h.markBotDegraded(t.AgentID, t.Error)
		m.Status, m.Question, m.QuestionAgent = "waiting", "", t.AgentID
		h.crewSay(msg, crewCaptain, "", "@"+crewMention(m.StartedByName)+" "+t.AgentName+" 在「"+t.Title+"」卡住了："+t.Error+"\n回复「重试」「跳过」或「停止」，也可以直接说怎么改，我交给它重做。", "", "")
	}
	if m.Status == "running" && !busy {
		next := crewNextPending(m)
		if next == nil {
			m.Status = "done"
			h.crewSay(msg, crewCaptain, "", crewWrapUp(m), "", "")
		} else if err := h.startCrewTask(msg, next); err != nil {
			next.Status, next.Error = "failed", roomTrim(err.Error(), 400)
			m.Status, m.QuestionAgent = "waiting", next.AgentID
			h.crewSay(msg, crewCaptain, "", next.AgentName+" 没能开工："+next.Error+"\n回复「重试」「跳过」或「停止」。", "", "")
		}
		changed = true
	}
	if changed {
		h.saveCrew(msg)
	}
}

func parseCrewHandoff(output, summary string) *domain.CrewHandoff {
	h := &domain.CrewHandoff{}
	cut := func(line, a, b string) (string, bool) {
		if strings.HasPrefix(line, a) {
			return strings.TrimSpace(strings.TrimPrefix(line, a)), true
		}
		if strings.HasPrefix(line, b) {
			return strings.TrimSpace(strings.TrimPrefix(line, b)), true
		}
		return "", false
	}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(strings.Trim(line, "-* "))
		if rest, ok := cut(line, "结论：", "结论:"); ok && h.Conclusion == "" {
			h.Conclusion = rest
			continue
		}
		if rest, ok := cut(line, "产物：", "产物:"); ok && rest != "" && rest != "无" {
			h.Artifacts = append(h.Artifacts, rest)
			continue
		}
		if rest, ok := cut(line, "未决问题：", "未决问题:"); ok && rest != "" && rest != "无" {
			h.OpenQuestions = append(h.OpenQuestions, rest)
		}
	}
	if h.Conclusion == "" {
		head := strings.TrimSpace(strings.SplitN(firstNonEmpty(summary, output), "\n", 2)[0])
		h.Conclusion = roomTrim(head, 200)
	}
	if q := crewQuestion(output); q != "" {
		h.OpenQuestions = append(h.OpenQuestions, q)
	}
	return h
}

func crewHandoffBlock(t domain.CrewTask) string {
	h := t.Handoff
	if h == nil {
		h = parseCrewHandoff("", t.Summary)
	}
	arts := "无"
	if len(h.Artifacts) > 0 {
		arts = strings.Join(h.Artifacts, "；")
	}
	open := "无"
	if len(h.OpenQuestions) > 0 {
		open = strings.Join(h.OpenQuestions, "；")
	}
	return "### " + t.AgentName + "「" + t.Title + "」\n结论：" + roomTrim(h.Conclusion, 400) + "\n产物：" + arts + "\n未决问题：" + open
}

func (h *roomsHandlers) fallbackBot(projectID string, userID int64, skipID string) (domain.AgentDefinition, string) {
	reason := "不可用"
	for _, st := range h.botStatuses(projectID, userID) {
		if st.AgentID == skipID && st.Reason != "" {
			reason = st.Reason
		}
	}
	for _, a := range h.crewRoster(projectID, userID) {
		if a.ID != skipID {
			return a, reason
		}
	}
	return domain.AgentDefinition{}, reason
}

func crewMention(name string) string {
	if i := strings.Index(name, "("); i > 0 {
		return name[:i]
	}
	return name
}

func crewNextPending(m *domain.CrewMission) *domain.CrewTask {
	for i := range m.Tasks {
		if m.Tasks[i].Status == "pending" {
			return &m.Tasks[i]
		}
	}
	return nil
}

func crewWrapUp(m *domain.CrewMission) string {
	var lines []string
	for _, t := range m.Tasks {
		if t.Status != "succeeded" {
			continue
		}
		head := strings.SplitN(t.Summary, "\n", 2)[0]
		lines = append(lines, "- "+t.AgentName+"「"+t.Title+"」："+roomTrim(head, 80))
	}
	return "搞定了，" + fmt.Sprint(len(lines)) + " 步都交付了：\n" + strings.Join(lines, "\n") + "\n这套分工要是好用，可以一键沉淀成 SOP，下次直接按流程跑。"
}

// crewBrief is what one bot receives on top of the issue: its part, what
// teammates already delivered and what people said in the room.
func crewBrief(m *domain.CrewMission, t *domain.CrewTask) string {
	var b strings.Builder
	b.WriteString("## 群聊协作（LoopForge 队长分派）\n")
	b.WriteString("你是协作群里的「" + t.AgentName + "」Bot，和其它 Bot 一起完成上面的目标。\n")
	b.WriteString("你这一步：" + t.Title + "\n")
	if t.Instruction != "" {
		b.WriteString("具体要求：" + t.Instruction + "\n")
	}
	var done []string
	for _, prev := range m.Tasks {
		if prev.Status == "succeeded" && prev.ID != t.ID {
			done = append(done, crewHandoffBlock(prev))
		}
	}
	if len(done) > 0 {
		b.WriteString("\n## 交接单（只含结论、产物、未决问题，不含完整过程）\n" + strings.Join(done, "\n\n") + "\n")
	}
	var notes []string
	for i := range m.Notes {
		notes = append(notes, "- "+m.Notes[i].Author+"："+m.Notes[i].Body)
		m.Notes[i].Consumed = true
	}
	if len(notes) > 0 {
		b.WriteString("\n## 群里同事的要求（必须遵守）\n" + strings.Join(notes, "\n") + "\n")
	}
	b.WriteString("\n## 回复方式\n用交接单收尾，下一步只能看到这三项，不要贴完整过程：\n结论：一句话\n产物：文件或结果，没有就写无\n未决问题：没有就写无\n缺关键信息时另起一行写「需要确认：你的问题」。\n")
	return b.String()
}

func (h *roomsHandlers) startCrewTask(msg domain.ChatMessage, t *domain.CrewTask) error {
	m := msg.Crew
	room, ok := h.repo.Room(msg.RoomID)
	if !ok {
		return fmt.Errorf("room not found")
	}
	if _, ok := h.repo.Agent(t.AgentID); !ok {
		return fmt.Errorf("角色 %s 不存在", t.AgentID)
	}
	// Each finished step closes the delivery loop on the issue; the mission
	// is not done yet, so reopen it for the next bot. A person closing the
	// issue by hand still blocks the mission.
	if issue, ok := h.repo.Issue(m.IssueID); ok && issue.DeliveryLocked() && !issue.ManuallyClosed() && !issue.TrackerClosed() {
		if _, err := h.repo.ReopenDeliveredIssue(issue.ID); err != nil {
			return err
		}
	}
	wf := domain.WorkflowDefinition{
		ID:   "crew-" + strings.TrimPrefix(m.ID, "crew-") + "-" + t.ID + "-" + fmt.Sprint(time.Now().Unix()%100000),
		Name: "群聊协作 · " + t.Title, Status: domain.WorkflowStatusCrew, ProjectID: room.ProjectID,
		Description: m.Goal, SandboxProfileID: h.crewSandbox(t.AgentID),
		Nodes: []domain.WorkflowNode{{ID: "work", Name: t.Title, Type: "agent", AgentID: t.AgentID, MaxAttempts: 1, Position: domain.Position{X: 240, Y: 160}}},
	}
	if err := h.repo.PutWorkflow(wf); err != nil {
		return err
	}
	task, err := dispatchIssue(h.deps, dispatchRequest{IssueID: m.IssueID, WorkflowID: wf.ID, TriggerUserID: m.StartedBy, TriggerOrigin: "crew", ExtraInput: crewBrief(m, t)})
	if err != nil {
		return err
	}
	t.Status, t.RunID, t.WorkflowID, t.StartedAt, t.Error = "running", task.RunID, wf.ID, time.Now().UTC(), ""
	return nil
}

// crewSandbox reuses the sandbox of an SOP that already runs this role.
func (h *roomsHandlers) crewSandbox(agentID string) string {
	fallback := ""
	for _, wf := range h.repo.Workflows() {
		for _, n := range wf.Nodes {
			if n.AgentID == agentID && wf.SandboxProfileID != "" {
				return wf.SandboxProfileID
			}
		}
		if fallback == "" && wf.Status == "published" {
			fallback = wf.SandboxProfileID
		}
	}
	return firstNonEmpty(fallback, "local")
}

// crewAction: start / stop a mission, retry or skip a stuck step, replace the
// pending part of the plan, or save the finished division of work as a SOP.
func (h *roomsHandlers) crewAction(c *gin.Context) {
	room, ok := h.repo.Room(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return
	}
	if !h.roomAccess(c, room.ProjectID, true) {
		return
	}
	var req crewActionRequest
	if !decode(c, &req) {
		return
	}
	_, author := h.roomActor(c)
	h.crewMu.Lock()
	msg, ok := h.repo.Message(c.Param("mid"))
	if !ok || msg.Crew == nil || msg.RoomID != room.ID {
		h.crewMu.Unlock()
		c.JSON(http.StatusNotFound, gin.H{"error": "协作任务不存在"})
		return
	}
	if req.Action == "sop" {
		h.crewMu.Unlock()
		id, err := h.crewToSOP(room, msg)
		if err != nil {
			respond(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"workflowId": id})
		return
	}
	err := h.applyCrewAction(&msg, req, author, room.ProjectID)
	if err == nil {
		h.saveCrew(msg)
	}
	h.crewMu.Unlock()
	if err != nil {
		respond(c, err)
		return
	}
	h.ensureCrewLoop()
	c.JSON(http.StatusOK, msg)
}

func (h *roomsHandlers) applyCrewAction(msg *domain.ChatMessage, req crewActionRequest, author, projectID string) error {
	m := msg.Crew
	switch req.Action {
	case "start":
		if m.Status != "proposed" {
			return HTTPError{http.StatusConflict, "这个任务已经开工了"}
		}
		m.Status = "running"
		if next := crewNextPending(m); next != nil {
			h.crewSay(*msg, crewCaptain, "", "开工。"+next.AgentName+" 先来「"+next.Title+"」。", "", "")
		}
	case "stop":
		if !domain.CrewMissionActive(m.Status) {
			return HTTPError{http.StatusConflict, "任务已经结束"}
		}
		for i := range m.Tasks {
			if m.Tasks[i].Status == "running" && m.Tasks[i].RunID != "" {
				_ = h.engine.Cancel(m.Tasks[i].RunID)
				m.Tasks[i].Status = "skipped"
			}
		}
		m.Status, m.Question = "stopped", ""
		h.crewSay(*msg, crewCaptain, "", author+" 让大家先停下。做完的部分都留在需求单里。", "", "")
	case "retry", "skip":
		if m.Status != "waiting" {
			return HTTPError{http.StatusConflict, "现在没有卡住的步骤"}
		}
		h.crewResolveStuck(msg, req.Action, author, "")
	case "plan":
		if !domain.CrewMissionActive(m.Status) {
			return HTTPError{http.StatusConflict, "任务已经结束"}
		}
		agents := map[string]domain.AgentDefinition{}
		for _, a := range h.crewRoster(projectID, m.StartedBy) {
			agents[a.ID] = a
		}
		kept := make([]domain.CrewTask, 0, len(m.Tasks))
		for _, t := range m.Tasks {
			if t.Status != "pending" {
				kept = append(kept, t)
			}
		}
		for _, in := range req.Tasks {
			a, ok := agents[strings.TrimSpace(in.AgentID)]
			if !ok {
				return HTTPError{http.StatusBadRequest, "角色不可用：" + in.AgentID}
			}
			title := roomTrim(strings.TrimSpace(in.Title), 40)
			if title == "" {
				title = "接着做"
			}
			kept = append(kept, domain.CrewTask{ID: fmt.Sprintf("t%d", len(kept)+1+int(time.Now().UnixNano()%1000)), AgentID: a.ID, AgentName: crewAgentName(a), Title: title, Instruction: roomTrim(strings.TrimSpace(in.Instruction), 600), Status: "pending", AddedBy: author})
		}
		if len(kept) > crewMaxTasks {
			return HTTPError{http.StatusBadRequest, fmt.Sprintf("一次协作最多 %d 步", crewMaxTasks)}
		}
		if crewNextPending(&domain.CrewMission{Tasks: kept}) == nil && m.Status == "proposed" {
			return HTTPError{http.StatusBadRequest, "至少留一步"}
		}
		for i := range kept {
			kept[i].ID = fmt.Sprintf("t%d", i+1)
		}
		m.Tasks = kept
	default:
		return HTTPError{http.StatusBadRequest, "unknown action"}
	}
	return nil
}

// crewResolveStuck moves a waiting mission on: retry the failed step, skip
// it, or (with feedback) hand the asking bot a follow-up step.
func (h *roomsHandlers) crewResolveStuck(msg *domain.ChatMessage, action, author, feedback string) {
	m := msg.Crew
	var stuck *domain.CrewTask
	for i := len(m.Tasks) - 1; i >= 0; i-- {
		if m.Tasks[i].AgentID == m.QuestionAgent && (m.Tasks[i].Status == "failed" || m.Tasks[i].Status == "succeeded") {
			stuck = &m.Tasks[i]
			break
		}
	}
	question := m.Question
	m.Status, m.Question, m.QuestionAgent = "running", "", ""
	if stuck == nil {
		return
	}
	switch {
	case action == "skip":
		if stuck.Status == "failed" {
			stuck.Status = "skipped"
		}
		h.crewSay(*msg, crewCaptain, "", "好，跳过「"+stuck.Title+"」，继续往下。", "", "")
	case action == "retry" && stuck.Status == "failed":
		stuck.Status, stuck.Error, stuck.RunID = "pending", "", ""
		h.crewSay(*msg, crewCaptain, "", "让 "+stuck.AgentName+" 再试一次「"+stuck.Title+"」。", "", "")
	default:
		if stuck.Status == "failed" {
			stuck.Status = "skipped"
		}
		instruction := "群里的回复：" + feedback
		if question != "" {
			instruction = "你之前问：" + question + "\n" + instruction
		}
		follow := domain.CrewTask{AgentID: stuck.AgentID, AgentName: stuck.AgentName, Title: "按反馈继续「" + roomTrim(stuck.Title, 16) + "」", Instruction: instruction, Status: "pending", AddedBy: author}
		h.crewInsertNext(m, follow)
		h.crewSay(*msg, crewCaptain, "", "收到，交回给 "+stuck.AgentName+" 继续。", "", "")
	}
}

// crewInsertNext puts a step right after the finished ones so it runs next.
func (h *roomsHandlers) crewInsertNext(m *domain.CrewMission, t domain.CrewTask) {
	at := len(m.Tasks)
	for i := range m.Tasks {
		if m.Tasks[i].Status == "pending" {
			at = i
			break
		}
	}
	m.Tasks = append(m.Tasks[:at], append([]domain.CrewTask{t}, m.Tasks[at:]...)...)
	for i := range m.Tasks {
		m.Tasks[i].ID = fmt.Sprintf("t%d", i+1)
	}
}

// crewHear speaks only when spoken to, like a group-chat bot: "@LoopForge"
// answers a question or starts a mission, "@某个 Bot" or replying to a bot's
// post adds a step for it, any message unblocks a waiting mission, and a reply
// to a mission's messages is handed to the next bot as a requirement.
// Plain chatter between people is left alone.
func (h *roomsHandlers) crewHear(room domain.ChatRoom, said domain.ChatMessage, spec *fakeModelSpec) {
	body := strings.TrimSpace(said.Body)
	quoted, hasQuote := domain.ChatMessage{}, false
	if said.ReplyTo != "" {
		quoted, hasQuote = h.repo.Message(said.ReplyTo)
	}
	if hasQuote && quoted.Kind == domain.ChatKindCrewPost && quoted.AgentID == "" && quoted.CrewID == "" && !strings.HasPrefix(body, "@") {
		body = "@LoopForge " + body
	}
	lower := strings.ToLower(body)
	if strings.HasPrefix(lower, "@loopforge") {
		goal := strings.TrimSpace(body[len("@loopforge"):])
		if goal == "" || crewWantsAnswer(goal) {
			h.answerInRoom(room, said, firstNonEmpty(goal, "你好，你能做什么？"), spec)
			return
		}
		{
			lines := strings.SplitN(goal, "\n", 2)
			detail := ""
			if len(lines) > 1 {
				detail = lines[1]
			}
			if _, err := h.startCrewMission(room, said.UserID, said.Author, crewRequest{Goal: lines[0], Detail: detail, Source: "chat"}); err != nil {
				h.crewSay(domain.ChatMessage{RoomID: room.ID}, crewCaptain, "", "没能接这个活："+err.Error(), "", "")
			}
		}
		return
	}
	h.crewMu.Lock()
	defer h.crewMu.Unlock()
	var msg domain.ChatMessage
	found := false
	missionID := quoted.CrewID
	if quoted.Kind == domain.ChatKindCrew {
		missionID = quoted.ID
	}
	missions := h.repo.CrewMessages(room.ID)
	for i := len(missions) - 1; i >= 0; i-- {
		if domain.CrewMissionActive(missions[i].Crew.Status) && missions[i].Crew.Status != "planning" && (missionID == "" || missions[i].ID == missionID) {
			msg, found = missions[i], true
			break
		}
	}
	if !found {
		return
	}
	m := msg.Crew
	if quoted.AgentID != "" && !strings.Contains(body, "@") {
		if a, ok := h.repo.Agent(quoted.AgentID); ok {
			body = "@" + crewAgentName(a) + " " + body
		}
	}
	lower = strings.ToLower(body)
	switch lower {
	case "停止", "停", "/stop", "stop":
		_ = h.applyCrewAction(&msg, crewActionRequest{Action: "stop"}, said.Author, room.ProjectID)
		h.saveCrew(msg)
		return
	}
	usable := map[string]bool{}
	for _, a := range h.crewRoster(room.ProjectID, m.StartedBy) {
		usable[a.ID] = true
	}
	for _, a := range h.repo.Agents() {
		name := crewAgentName(a)
		if !catalogVisible(a.ProjectID, a.Shared, room.ProjectID) || !strings.Contains(body, "@"+name) {
			continue
		}
		if !usable[a.ID] {
			fb, reason := h.fallbackBot(room.ProjectID, m.StartedBy, a.ID)
			if fb.ID == "" {
				h.crewSay(msg, crewCaptain, "", name+" 不可用："+reason+"。现在没有能接替的 Bot。", "", "")
				return
			}
			from := name
			a = fb
			name = crewAgentName(a)
			ask := strings.TrimSpace(strings.Replace(body, "@"+from, "", 1))
			t := domain.CrewTask{AgentID: a.ID, AgentName: name, Title: roomTrim(firstNonEmpty(strings.SplitN(ask, "\n", 2)[0], "按群里的要求补一步"), 24), Instruction: ask, Status: "pending", AddedBy: said.Author, ReassignedFrom: from, ReassignedReason: reason}
			m.Tasks = append(m.Tasks, t)
			for i := range m.Tasks {
				m.Tasks[i].ID = fmt.Sprintf("t%d", i+1)
			}
			h.crewSay(msg, crewCaptain, "", "已改由 "+name+" 负责（"+from+" "+reason+"）。给它加了一步「"+t.Title+"」。", "", "")
			h.saveCrew(msg)
			h.ensureCrewLoop()
			return
		}
		if len(m.Tasks) >= crewMaxTasks {
			h.crewSay(msg, crewCaptain, "", fmt.Sprintf("这次协作已经排满 %d 步了，先让大家做完。", crewMaxTasks), "", "")
			return
		}
		ask := strings.TrimSpace(strings.Replace(body, "@"+name, "", 1))
		t := domain.CrewTask{AgentID: a.ID, AgentName: name, Title: roomTrim(firstNonEmpty(strings.SplitN(ask, "\n", 2)[0], "按群里的要求补一步"), 24), Instruction: ask, Status: "pending", AddedBy: said.Author}
		m.Tasks = append(m.Tasks, t)
		for i := range m.Tasks {
			m.Tasks[i].ID = fmt.Sprintf("t%d", i+1)
		}
		ack := "收到，给 " + name + " 加了一步「" + t.Title + "」，排在现有分工之后。"
		if m.Status == "waiting" {
			ack += "现在还卡着，回复「重试」「跳过」或直接说怎么改。"
		}
		h.crewSay(msg, crewCaptain, "", ack, "", "")
		h.saveCrew(msg)
		h.ensureCrewLoop()
		return
	}
	if m.Status == "waiting" {
		switch lower {
		case "重试", "retry":
			h.crewResolveStuck(&msg, "retry", said.Author, "")
		case "跳过", "skip":
			h.crewResolveStuck(&msg, "skip", said.Author, "")
		default:
			m.Notes = append(m.Notes, domain.CrewNote{Author: said.Author, Body: roomTrim(body, 1000), At: time.Now().UTC()})
			h.crewResolveStuck(&msg, "answer", said.Author, body)
		}
		h.saveCrew(msg)
		h.ensureCrewLoop()
		return
	}
	if missionID == "" {
		return
	}
	m.Notes = append(m.Notes, domain.CrewNote{Author: said.Author, Body: roomTrim(body, 1000), At: time.Now().UTC()})
	if next := crewNextPending(m); next != nil {
		h.crewSay(msg, crewCaptain, "", "记下了，会交给下一位 "+next.AgentName+"。", "", "")
	}
	h.saveCrew(msg)
}

// crewToSOP turns the steps that delivered into a draft SOP in the same order.
func (h *roomsHandlers) crewToSOP(room domain.ChatRoom, msg domain.ChatMessage) (string, error) {
	m := msg.Crew
	if m.SavedWorkflow != "" {
		if _, ok := h.repo.Workflow(m.SavedWorkflow); ok {
			return m.SavedWorkflow, nil
		}
	}
	var nodes []domain.WorkflowNode
	var edges []domain.WorkflowEdge
	sandbox := ""
	for _, t := range m.Tasks {
		if t.Status != "succeeded" {
			continue
		}
		id := fmt.Sprintf("step%d", len(nodes)+1)
		if len(nodes) > 0 {
			edges = append(edges, domain.WorkflowEdge{ID: "e" + id, Source: nodes[len(nodes)-1].ID, Target: id, When: "success"})
		}
		nodes = append(nodes, domain.WorkflowNode{ID: id, Name: t.Title, Type: "agent", AgentID: t.AgentID, MaxAttempts: 2, Position: domain.Position{X: float64(120 + 280*len(nodes)), Y: 200}})
		if sandbox == "" {
			sandbox = h.crewSandbox(t.AgentID)
		}
	}
	if len(nodes) == 0 {
		return "", HTTPError{http.StatusConflict, "还没有做成的步骤，没法沉淀"}
	}
	wf := domain.WorkflowDefinition{
		ID: "sop-" + strings.TrimPrefix(m.ID, "crew-"), Name: roomTrim("群聊沉淀 · "+m.Goal, 40), Status: "draft",
		ProjectID: room.ProjectID, SandboxProfileID: sandbox, Nodes: nodes, Edges: edges,
		Description: "从协作群「" + room.Name + "」的一次自由协作沉淀：" + m.Goal,
	}
	if err := h.repo.PutWorkflow(wf); err != nil {
		return "", err
	}
	h.crewMu.Lock()
	if again, ok := h.repo.Message(msg.ID); ok && again.Crew != nil {
		again.Crew.SavedWorkflow = wf.ID
		h.saveCrew(again)
		h.crewSay(again, crewCaptain, "", "已沉淀成 SOP「"+wf.Name+"」（草稿），在流程模板里可以继续调整后发布。", "", "")
	}
	h.crewMu.Unlock()
	return wf.ID, nil
}
