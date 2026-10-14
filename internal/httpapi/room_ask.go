package httpapi

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
)

var (
	crewStatusAsk = regexp.MustCompile(`进度|状态|做到哪|卡在哪|卡住|在跑什么|谁在做|做完了吗|好了吗`)
	crewTaskVerb  = regexp.MustCompile(`实现|开发|修复|修一下|修掉|写一|写个|加一|加个|新增|改成|改一下|重构|调研|整理|出一份|出个|设计一|搭一|部署|上线|补一|做一|做个|帮我做|跑一下|排查`)
	crewAskWord   = regexp.MustCompile(`(?s)^(什么|怎么|为什么|如何|是否|能不能|能否|可不可以|谁|哪|多少|有没有|解释|总结|概括|翻译|介绍|说说|讲讲|你好|嗨|hi|hello)|[?？吗呢]\s*$`)
)

// crewWantsAnswer tells a question for the captain apart from work for the
// team. Status questions always get an answer; anything that reads like a
// task goes to the team even when it ends with a question mark.
func crewWantsAnswer(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return false
	}
	if crewStatusAsk.MatchString(t) {
		return true
	}
	if crewTaskVerb.MatchString(t) {
		return false
	}
	return crewAskWord.MatchString(t) || len([]rune(t)) <= 6
}

func roomQuoteAuthor(m domain.ChatMessage) string {
	if m.Kind == domain.ChatKindCrewPost && m.AgentID == "" {
		return crewCaptain
	}
	name := strings.TrimSpace(m.Author)
	if i := strings.Index(name, "("); i > 0 && strings.HasSuffix(name, ")") {
		name = name[i+1 : len(name)-1]
	}
	return strings.TrimPrefix(name, "演示 · ")
}

func roomQuoteText(m domain.ChatMessage) string {
	switch {
	case m.Kind == domain.ChatKindCrew && m.Crew != nil:
		return "@LoopForge " + m.Crew.Goal
	case m.Kind == "dispatch":
		return fmt.Sprintf("派活：%s → %s", firstNonEmpty(m.IssueTitle, m.Body), firstNonEmpty(m.WorkflowName, m.TargetName))
	}
	return m.Body
}

// answerInRoom posts a pending captain reply right away so everyone sees the
// bot is on it, then streams the answer token by token.
func (h *roomsHandlers) answerInRoom(room domain.ChatRoom, said domain.ChatMessage, question string, spec *fakeModelSpec) {
	parent := said.ParentID
	if parent == "" {
		parent = said.ReplyTo
	}
	reply := domain.ChatMessage{
		ID: newChatID("msg"), RoomID: room.ID, Author: crewCaptain, Kind: domain.ChatKindCrewPost, Pending: true,
		ReplyTo: said.ID, ReplyAuthor: roomQuoteAuthor(said), ReplyText: roomTrim(said.Body, 200),
		ParentID: parent, CreatedAt: time.Now().UTC(),
	}
	if err := h.repo.PutMessage(reply); err != nil {
		return
	}
	h.beginReplyStream(room, said, reply, question, spec)
}

func (h *roomsHandlers) crewAnswer(room domain.ChatRoom, said domain.ChatMessage, question string) string {
	status := h.roomStatusDigest(room)
	user, ok := h.repo.User(said.UserID)
	if !ok || strings.TrimSpace(user.CodeBuddyAPIKey) == "" {
		if crewStatusAsk.MatchString(question) {
			return status
		}
		return "直接回答问题要用模型，你还没配 Key：去「设置 · 个人设置」配好就行。\n现在我能做的：\n- 「@LoopForge + 要做的事」我来拉 Bot 组队干活\n- 问「进度」我汇报群里的任务\n\n" + status
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	raw, err := generateAssistantReply(ctx, h.captainPrompt(room, said, question, status), user)
	if text := strings.TrimSpace(raw); err == nil && text != "" {
		return roomTrim(text, 2000)
	}
	return "这次没答上来（模型没有返回）。可以稍后再问，或者「@LoopForge + 要做的事」让 Bot 团队去做。"
}

// roomStatusDigest is the captain's report on work started in this room.
func (h *roomsHandlers) roomStatusDigest(room domain.ChatRoom) string {
	label := map[string]string{"planning": "在排分工", "proposed": "等你点开工", "running": "在做", "waiting": "卡住了，等人回复", "done": "做完了", "failed": "没做成", "stopped": "已停止"}
	var lines []string
	missions := h.repo.CrewMessages(room.ID)
	for i := len(missions) - 1; i >= 0 && len(lines) < 5; i-- {
		m := missions[i].Crew
		done := 0
		current := ""
		for _, t := range m.Tasks {
			switch t.Status {
			case "succeeded", "skipped":
				done++
			case "running":
				current = "，" + t.AgentName + " 正在「" + t.Title + "」"
			}
		}
		line := fmt.Sprintf("- #%d %s：%s（%d/%d 步%s）", m.IssueNumber, roomTrim(m.Goal, 40), firstNonEmpty(label[m.Status], m.Status), done, len(m.Tasks), current)
		if m.Status == "waiting" && m.Question != "" {
			line += "\n  问题：" + roomTrim(m.Question, 80)
		}
		lines = append(lines, line)
	}
	for _, m := range h.repo.Messages(room.ID, 200) {
		if m.Kind == "dispatch" && m.RunID != "" && m.RunFinal == "" && !roomRunTerminal(m.RunStatus) {
			lines = append(lines, fmt.Sprintf("- SOP「%s」· #%d %s：%s", firstNonEmpty(m.WorkflowName, m.TargetName), m.IssueNumber, roomTrim(m.IssueTitle, 40), firstNonEmpty(m.RunStatus, "排队中")))
		}
	}
	if len(lines) == 0 {
		return "群里现在没有进行中的任务。"
	}
	return "群里的任务：\n" + strings.Join(lines, "\n")
}

const crewAnswerPrompt = `你是群聊里的 Bot「LoopForge」，这个项目的 PM 兼 Bot 团队队长。像同事在群里回话一样回答：中文、口语、简短（一般不超过 8 行），可以用 Markdown 列表。
只根据下面的群任务、上下文和引用的消息回答；不知道就直说，不要编。
如果对方其实是想让人动手干活，告诉他用「@LoopForge + 要做的事」交给 Bot 团队，或 @ 一条 SOP 按流程跑。
线程里的提问只带 parent_id 这条链，不要假设你看过群里的其它消息。

## 群任务
%s

## 上下文（parent_id=%s）
%s

## 引用的消息
%s

## 提问（来自 %s）
%s`

func (h *roomsHandlers) captainPrompt(room domain.ChatRoom, said domain.ChatMessage, question, status string) string {
	history := h.captainHistory(room, said)
	quote := "无"
	if said.ReplyTo != "" {
		quote = said.ReplyAuthor + "：" + said.ReplyText
	}
	parent := said.ParentID
	if parent == "" {
		parent = "主时间线"
	}
	return fmt.Sprintf(crewAnswerPrompt, status, parent, strings.Join(history, "\n"), quote, roomQuoteAuthor(said), question)
}

// captainHistory is the thread chain when the message has a parent, and a
// short slice of the room otherwise. A thread never receives the whole timeline.
func (h *roomsHandlers) captainHistory(room domain.ChatRoom, said domain.ChatMessage) []string {
	all := h.repo.Messages(room.ID, 40)
	if said.ParentID != "" || said.ReplyTo != "" {
		return roomThreadLines(all, said)
	}
	var history []string
	for _, m := range all {
		if m.ID == said.ID || m.Pending {
			continue
		}
		if text := strings.TrimSpace(roomQuoteText(m)); text != "" {
			history = append(history, roomQuoteAuthor(m)+"："+roomTrim(text, 240))
		}
	}
	return history
}

func roomThreadLines(all []domain.ChatMessage, leaf domain.ChatMessage) []string {
	byID := map[string]domain.ChatMessage{}
	for _, m := range all {
		byID[m.ID] = m
	}
	var chain []domain.ChatMessage
	seen := map[string]bool{}
	id := firstNonEmpty(leaf.ParentID, leaf.ReplyTo)
	for id != "" && len(chain) < 8 && !seen[id] {
		seen[id] = true
		m, ok := byID[id]
		if !ok {
			break
		}
		chain = append(chain, m)
		id = firstNonEmpty(m.ParentID, m.ReplyTo)
	}
	lines := make([]string, 0, len(chain))
	for i := len(chain) - 1; i >= 0; i-- {
		lines = append(lines, "parent_id="+chain[i].ID+" "+roomQuoteAuthor(chain[i])+"："+roomTrim(roomQuoteText(chain[i]), 240))
	}
	return lines
}
