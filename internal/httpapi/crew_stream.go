package httpapi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
)

const crewStreamPrefix = "stream:"

func (h *roomsHandlers) rememberCrewSpec(id string, spec *fakeModelSpec) {
	if id == "" || spec == nil || !fakeModelAllowed() {
		return
	}
	cp := *spec
	h.crewSpecMu.Lock()
	defer h.crewSpecMu.Unlock()
	if h.crewSpecs == nil {
		h.crewSpecs = map[string]*fakeModelSpec{}
	}
	h.crewSpecs[id] = &cp
}

func (h *roomsHandlers) crewSpecFor(id string) *fakeModelSpec {
	h.crewSpecMu.Lock()
	defer h.crewSpecMu.Unlock()
	if h.crewSpecs == nil {
		return nil
	}
	return h.crewSpecs[id]
}

func (h *roomsHandlers) useCrewFakeStream(missionID string) bool {
	if !fakeModelAllowed() {
		return false
	}
	if envOn("LOOPFORGE_DEMO_MODE") {
		return true
	}
	spec := h.crewSpecFor(missionID)
	return spec != nil && spec.CrewStep
}

func (h *roomsHandlers) takeCrewFail(missionID string, step int) bool {
	h.crewSpecMu.Lock()
	defer h.crewSpecMu.Unlock()
	spec := h.crewSpecs[missionID]
	if spec == nil || spec.FailStep == 0 || spec.FailStep != step || spec.failUsed {
		return false
	}
	spec.failUsed = true
	return true
}

func crewTaskStep(tasks []domain.CrewTask, id string) int {
	for i := range tasks {
		if tasks[i].ID == id {
			return i + 1
		}
	}
	return 1
}

func crewDemoText(t domain.CrewTask) string {
	id := t.ID
	if id == "" {
		id = "step"
	}
	return "结论：" + t.AgentName + " 已完成「" + t.Title + "」\n产物：" + id + ".md\n未决问题：无"
}

func (h *roomsHandlers) crewFakeSpec(mission *domain.CrewMission, t *domain.CrewTask) (fakeModelSpec, bool) {
	step := crewTaskStep(mission.Tasks, t.ID)
	local := fakeModelSpec{Text: crewDemoText(*t), FirstTokenMs: 120, TokenMs: 12}
	spec := h.crewSpecFor(mission.ID)
	if spec != nil {
		if step == 1 && strings.TrimSpace(spec.Text) != "" {
			local.Text = spec.Text
		}
		if spec.FirstTokenMs > 0 {
			local.FirstTokenMs = spec.FirstTokenMs
		}
		if spec.TokenMs > 0 {
			local.TokenMs = spec.TokenMs
		}
		if step == 1 {
			local.StallAfter = spec.StallAfter
			local.StallMs = spec.StallMs
			local.DisconnectAfter = spec.DisconnectAfter
		}
	}
	fail := h.takeCrewFail(mission.ID, step)
	if fail && local.DisconnectAfter == 0 {
		local.DisconnectAfter = 2
	}
	return local, fail
}

func (h *roomsHandlers) launchNextCrewStep(missionID string) {
	h.crewMu.Lock()
	defer h.crewMu.Unlock()
	msg, ok := h.repo.Message(missionID)
	if !ok || msg.Crew == nil || msg.Crew.Status != "running" {
		return
	}
	for _, t := range msg.Crew.Tasks {
		if t.Status == "running" {
			return
		}
	}
	next := crewNextPending(msg.Crew)
	if next == nil {
		msg.Crew.Status = "done"
		h.saveCrew(msg)
		h.crewSay(msg, crewCaptain, "", crewWrapUp(msg.Crew), "", "")
		return
	}
	if err := h.startCrewTask(msg, next); err != nil {
		next.Status, next.Error = "failed", roomTrim(err.Error(), 400)
		msg.Crew.Status, msg.Crew.QuestionAgent = "waiting", next.AgentID
		h.saveCrew(msg)
		h.crewSay(msg, crewCaptain, "", next.AgentName+" 没能开工："+next.Error+"\n回复「重试」「跳过」或「停止」。", "", "")
		return
	}
	h.saveCrew(msg)
}

func (h *roomsHandlers) openCrewFakeStep(msg domain.ChatMessage, t *domain.CrewTask) error {
	post := h.newCrewPost(msg, t)
	spec, fail := h.crewFakeSpec(msg.Crew, t)
	ctx, cancel := context.WithCancel(context.Background())
	st := h.streamHub().open(post.ID, post.RoomID, "", post.UserID, &spec, cancel)
	if err := h.repo.PutMessage(post); err != nil {
		cancel()
		return err
	}
	t.StreamGen++
	t.Status, t.RunID, t.StreamPost = "running", crewStreamPrefix+post.ID, post.ID
	t.StartedAt, t.Error = time.Now().UTC(), ""
	go h.runCrewFake(ctx, cancel, st, msg.ID, t.ID, t.StreamGen, post, spec, fail)
	return nil
}

func (h *roomsHandlers) newCrewPost(msg domain.ChatMessage, t *domain.CrewTask) domain.ChatMessage {
	return domain.ChatMessage{
		ID: newChatID("msg"), RoomID: msg.RoomID, Author: t.AgentName, AgentID: t.AgentID,
		Kind: domain.ChatKindCrewPost, CrewID: msg.ID, Pending: true,
		IssueID: msg.IssueID, IssueNumber: msg.IssueNumber, IssueTitle: msg.IssueTitle,
		CreatedAt: time.Now().UTC(),
	}
}

func (h *roomsHandlers) attachCrewRunStream(msg domain.ChatMessage, t *domain.CrewTask) {
	post := h.newCrewPost(msg, t)
	if err := h.repo.PutMessage(post); err != nil {
		return
	}
	t.StreamGen++
	t.StreamPost = post.ID
	go h.watchCrewRun(msg.ID, t.ID, t.StreamGen, post, t.RunID)
}

func (h *roomsHandlers) restartCrewPost(post domain.ChatMessage) {
	h.crewMu.Lock()
	msg, ok := h.repo.Message(post.CrewID)
	taskID := ""
	if ok && msg.Crew != nil {
		for i := range msg.Crew.Tasks {
			t := &msg.Crew.Tasks[i]
			if t.StreamPost != post.ID && strings.TrimPrefix(t.RunID, crewStreamPrefix) != post.ID {
				continue
			}
			t.StreamGen++
			t.Status, t.Error = "running", ""
			taskID = t.ID
			if msg.Crew.Status == "waiting" || msg.Crew.Status == "stopped" {
				msg.Crew.Status, msg.Crew.Question, msg.Crew.QuestionAgent = "running", "", ""
			}
		}
		if taskID != "" {
			h.saveCrew(msg)
		}
	}
	h.crewMu.Unlock()
	if taskID == "" || msg.Crew == nil {
		return
	}
	post.Body, post.Pending, post.NodeStatus, post.Error = "", true, "", ""
	_ = h.repo.PutMessage(post)
	var task domain.CrewTask
	for _, t := range msg.Crew.Tasks {
		if t.ID == taskID {
			task = t
			break
		}
	}
	if strings.HasPrefix(task.RunID, crewStreamPrefix) || h.useCrewFakeStream(msg.ID) {
		spec, fail := h.crewFakeSpec(msg.Crew, &task)
		ctx, cancel := context.WithCancel(context.Background())
		st := h.streamHub().open(post.ID, post.RoomID, "", post.UserID, &spec, cancel)
		go h.runCrewFake(ctx, cancel, st, msg.ID, task.ID, task.StreamGen, post, spec, fail)
		return
	}
	go h.watchCrewRun(msg.ID, task.ID, task.StreamGen, post, task.RunID)
}

func (h *roomsHandlers) crewStreamLive(missionID, taskID string, gen int) bool {
	msg, ok := h.repo.Message(missionID)
	if !ok || msg.Crew == nil {
		return false
	}
	for i := range msg.Crew.Tasks {
		if msg.Crew.Tasks[i].ID == taskID {
			return msg.Crew.Tasks[i].StreamGen == gen
		}
	}
	return false
}

func (h *roomsHandlers) runCrewFake(ctx context.Context, cancel context.CancelFunc, st *roomTokenStream, missionID, taskID string, gen int, post domain.ChatMessage, spec fakeModelSpec, fail bool) {
	defer cancel()
	body, err := h.writeStream(ctx, st, &post, func(emit func(string) bool) error {
		return runFakeModel(ctx, spec, emit)
	})
	if !h.crewStreamLive(missionID, taskID, gen) {
		st.finish(roomStreamEvent{Type: "stopped", Text: strings.TrimSpace(body)})
		return
	}
	stopped := ctx.Err() != nil
	if stopped {
		err = nil
	} else if fail && err != nil {
		err = fmt.Errorf("这一步没有做成，点「重试」可以接着跑")
	}
	h.finishCrewPost(&post, body, stopped, err)
	h.settleCrewStream(missionID, taskID, gen, post.ID, body, stopped, err)
	h.finishStreamEvent(st, &post, stopped, err)
}

func (h *roomsHandlers) watchCrewRun(missionID, taskID string, gen int, post domain.ChatMessage, runID string) {
	ctx, cancel := context.WithCancel(context.Background())
	st := h.streamHub().open(post.ID, post.RoomID, "", post.UserID, nil, cancel)
	defer cancel()
	var emitted string
	body, err := h.writeStream(ctx, st, &post, func(emit func(string) bool) error {
		return h.followCrewRun(ctx, runID, emit, &emitted)
	})
	stopped := ctx.Err() != nil
	if !h.crewStreamLive(missionID, taskID, gen) {
		st.finish(roomStreamEvent{Type: "stopped", Text: strings.TrimSpace(body)})
		return
	}
	if stopped && h.engine != nil && !strings.HasPrefix(runID, crewStreamPrefix) {
		_ = h.engine.Cancel(runID)
		err = nil
	}
	if body == "" {
		body = emitted
	}
	h.finishCrewPost(&post, body, stopped, err)
	h.settleCrewStream(missionID, taskID, gen, post.ID, body, stopped, err)
	h.finishStreamEvent(st, &post, stopped, err)
}

func (h *roomsHandlers) followCrewRun(ctx context.Context, runID string, emit func(string) bool, emitted *string) error {
	deadline := time.Now().Add(8 * time.Minute)
	var sent string
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("这一步超过 8 分钟还没有结果")
		}
		run, ok := h.repo.Run(runID)
		if ok {
			text := crewLiveText(run)
			if strings.HasPrefix(text, sent) && len(text) > len(sent) {
				if !emit(text[len(sent):]) {
					return ctx.Err()
				}
				sent = text
				*emitted = sent
			}
			if roomRunTerminal(run.Status) {
				if sent == "" {
					if !emitRunesDone(ctx, text, emit) {
						return ctx.Err()
					}
					*emitted = text
				}
				if run.Status == "succeeded" {
					return nil
				}
				reason := run.Status
				if node := run.Nodes["work"]; node != nil && strings.TrimSpace(node.Error) != "" {
					reason = node.Error
				}
				return fmt.Errorf("%s", roomTrim(reason, 400))
			}
		}
		if !waitCtx(ctx, 200*time.Millisecond) {
			return ctx.Err()
		}
	}
}

func crewLiveText(run domain.Run) string {
	var b strings.Builder
	for _, ev := range run.Telemetry {
		if ev.Name == "agent.output" && ev.Message != "" {
			b.WriteString(ev.Message)
		}
	}
	if b.Len() > 0 {
		return b.String()
	}
	if node := run.Nodes["work"]; node != nil {
		return node.Output
	}
	return ""
}

func emitRunesDone(ctx context.Context, text string, emit func(string) bool) bool {
	return emitRunes(ctx, text, emit) == nil
}

func (h *roomsHandlers) writeStream(ctx context.Context, st *roomTokenStream, post *domain.ChatMessage, run func(func(string) bool) error) (string, error) {
	var b strings.Builder
	emit := func(tok string) bool {
		if tok == "" {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		default:
		}
		b.WriteString(tok)
		st.publish(roomStreamEvent{Type: "token", Text: tok})
		if b.Len()%24 == 0 {
			post.Body = b.String()
			_ = h.repo.PutMessage(*post)
		}
		return true
	}
	err := run(emit)
	return b.String(), err
}

func (h *roomsHandlers) finishCrewPost(post *domain.ChatMessage, body string, stopped bool, err error) {
	post.Body = strings.TrimSpace(body)
	post.Pending = false
	switch {
	case stopped:
		post.NodeStatus = "stopped"
	case err != nil:
		post.NodeStatus = "error"
		post.Error = err.Error()
		if post.Body == "" {
			post.Body = err.Error()
		}
	default:
		post.NodeStatus = ""
		post.Error = ""
	}
	_ = h.repo.PutMessage(*post)
}

func (h *roomsHandlers) finishStreamEvent(st *roomTokenStream, post *domain.ChatMessage, stopped bool, err error) {
	switch {
	case stopped:
		st.finish(roomStreamEvent{Type: "stopped", Text: post.Body})
	case err != nil:
		st.finish(roomStreamEvent{Type: "error", Text: err.Error()})
	default:
		st.finish(roomStreamEvent{Type: "done", Text: post.Body})
	}
}

func (h *roomsHandlers) settleCrewStream(missionID, taskID string, gen int, postID, body string, stopped bool, runErr error) {
	h.crewMu.Lock()
	defer h.crewMu.Unlock()
	msg, ok := h.repo.Message(missionID)
	if !ok || msg.Crew == nil {
		return
	}
	var t *domain.CrewTask
	for i := range msg.Crew.Tasks {
		if msg.Crew.Tasks[i].ID == taskID {
			t = &msg.Crew.Tasks[i]
			break
		}
	}
	if t == nil || t.Status != "running" || t.StreamGen != gen {
		return
	}
	t.FinishedAt = time.Now().UTC()
	if stopped {
		t.Status, t.Error = "failed", "已停止"
		if msg.Crew.Status == "running" {
			msg.Crew.Status, msg.Crew.Question, msg.Crew.QuestionAgent = "waiting", "", t.AgentID
		}
		h.saveCrew(msg)
		h.crewSay(msg, crewCaptain, "", t.AgentName+" 停在了「"+t.Title+"」。上面留下了已经写出的内容，回复「重试」可以续跑。", "", "")
		return
	}
	if runErr != nil {
		t.Status, t.Error = "failed", roomTrim(runErr.Error(), 400)
		h.markBotDegraded(t.AgentID, t.Error)
		msg.Crew.Status, msg.Crew.Question, msg.Crew.QuestionAgent = "waiting", "", t.AgentID
		h.saveCrew(msg)
		h.crewSay(msg, crewCaptain, "", "@"+crewMention(msg.Crew.StartedByName)+" "+t.AgentName+" 在「"+t.Title+"」卡住了："+t.Error+"\n回复「重试」「跳过」或「停止」，也可以直接说怎么改。", "", "")
		return
	}
	t.Status = "succeeded"
	t.Output, t.Summary = roomTrim(body, 6000), crewSummary(body)
	t.Handoff = parseCrewHandoff(body, t.Summary)
	t.Error = ""
	h.clearBotDegraded(t.AgentID)
	if q := crewQuestion(body); q != "" {
		msg.Crew.Status, msg.Crew.Question, msg.Crew.QuestionAgent = "waiting", q, t.AgentID
		h.saveCrew(msg)
		h.crewSay(msg, crewCaptain, "", "@"+crewMention(msg.Crew.StartedByName)+" "+t.AgentName+" 需要你确认："+q+"\n直接在群里回复，我会交回给它继续。", "", "")
		return
	}
	next := crewNextPending(msg.Crew)
	if stored, ok := h.repo.Message(postID); ok && next != nil {
		stored.TargetName, stored.NodeName = next.AgentName, next.Title
		_ = h.repo.PutMessage(stored)
	}
	if next == nil {
		msg.Crew.Status = "done"
		h.saveCrew(msg)
		h.crewSay(msg, crewCaptain, "", crewWrapUp(msg.Crew), "", "")
		return
	}
	if err := h.startCrewTask(msg, next); err != nil {
		next.Status, next.Error = "failed", roomTrim(err.Error(), 400)
		msg.Crew.Status, msg.Crew.QuestionAgent = "waiting", next.AgentID
		h.saveCrew(msg)
		h.crewSay(msg, crewCaptain, "", next.AgentName+" 没能开工："+next.Error+"\n回复「重试」「跳过」或「停止」。", "", "")
		return
	}
	h.saveCrew(msg)
}
