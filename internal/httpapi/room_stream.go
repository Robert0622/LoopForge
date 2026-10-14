package httpapi

import (
	"context"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/gin-gonic/gin"
)

// fakeModelSpec drives the CI mock model. Ignored unless demo mode or
// LOOPFORGE_FAKE_MODEL is on, so a production room cannot be told what to say.
type fakeModelSpec struct {
	Text            string `json:"text"`
	FirstTokenMs    int    `json:"firstTokenMs"`
	TokenMs         int    `json:"tokenMs"`
	StallAfter      int    `json:"stallAfter"`
	StallMs         int    `json:"stallMs"`
	DisconnectAfter int    `json:"disconnectAfter"`
	// CrewStep asks a demo/fake model to stream a bot step instead of the captain.
	CrewStep bool `json:"crewStep"`
	// FailStep is the 1-based crew step that errors once, so retry can continue.
	FailStep int `json:"failStep"`
	failUsed bool
}

func fakeModelAllowed() bool {
	if envOn("LOOPFORGE_FAKE_MODEL") || envOn("LOOPFORGE_DEMO_MODE") {
		return true
	}
	return false
}

func envOn(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

type roomStreamEvent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Seq  int    `json:"seq"`
}

type roomStreamHub struct {
	mu   sync.Mutex
	byID map[string]*roomTokenStream
}

func newRoomStreamHub() *roomStreamHub {
	return &roomStreamHub{byID: map[string]*roomTokenStream{}}
}

func (h *roomsHandlers) streamHub() *roomStreamHub {
	h.streamsOnce.Do(func() { h.streams = newRoomStreamHub() })
	return h.streams
}

type roomTokenStream struct {
	mu       sync.Mutex
	events   []roomStreamEvent
	subs     []chan roomStreamEvent
	cancel   context.CancelFunc
	closed   bool
	spec     *fakeModelSpec
	question string
	roomID   string
	userID   int64
}

func (s *roomTokenStream) publish(ev roomStreamEvent) {
	s.mu.Lock()
	ev.Seq = len(s.events) + 1
	s.events = append(s.events, ev)
	subs := append([]chan roomStreamEvent(nil), s.subs...)
	s.mu.Unlock()
	for _, ch := range subs {
		ch <- ev
	}
}

func (s *roomTokenStream) finish(ev roomStreamEvent) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	ev.Seq = len(s.events) + 1
	s.events = append(s.events, ev)
	subs := s.subs
	s.subs = nil
	s.mu.Unlock()
	for _, ch := range subs {
		ch <- ev
		close(ch)
	}
}

func (s *roomTokenStream) unsubscribe(ch chan roomStreamEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.subs[:0]
	for _, sub := range s.subs {
		if sub != ch {
			next = append(next, sub)
		}
	}
	s.subs = next
}

func (s *roomTokenStream) subscribe() (chan roomStreamEvent, []roomStreamEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	replay := append([]roomStreamEvent(nil), s.events...)
	if s.closed {
		return nil, replay
	}
	ch := make(chan roomStreamEvent, 64)
	s.subs = append(s.subs, ch)
	return ch, replay
}

func (h *roomStreamHub) open(id, roomID, question string, userID int64, spec *fakeModelSpec, cancel context.CancelFunc) *roomTokenStream {
	h.mu.Lock()
	defer h.mu.Unlock()
	if old := h.byID[id]; old != nil && old.cancel != nil {
		old.cancel()
	}
	st := &roomTokenStream{cancel: cancel, spec: spec, question: question, roomID: roomID, userID: userID}
	h.byID[id] = st
	return st
}

func (h *roomStreamHub) get(id string) *roomTokenStream {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.byID[id]
}

func (h *roomsHandlers) streamMessage(c *gin.Context) {
	room, msg, ok := h.roomMessage(c)
	if !ok {
		return
	}
	if !h.roomAccess(c, room.ProjectID, false) {
		return
	}
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	st := h.streamHub().get(msg.ID)
	if st == nil {
		writeSSE(c, "done", roomStreamEvent{Type: "done", Text: msg.Body})
		return
	}
	live, replay := st.subscribe()
	if live != nil {
		defer st.unsubscribe(live)
	}
	for _, ev := range replay {
		writeSSE(c, ev.Type, ev)
		if roomStreamTerminal(ev.Type) {
			return
		}
	}
	if live == nil {
		return
	}
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-ping.C:
			writeSSE(c, "ping", roomStreamEvent{Type: "ping"})
		case ev, open := <-live:
			if !open {
				return
			}
			writeSSE(c, ev.Type, ev)
			if roomStreamTerminal(ev.Type) {
				return
			}
		}
	}
}

func roomStreamTerminal(kind string) bool {
	return kind == "done" || kind == "error" || kind == "stopped"
}

func (h *roomsHandlers) stopMessage(c *gin.Context) {
	room, msg, ok := h.roomMessage(c)
	if !ok {
		return
	}
	if !h.roomAccess(c, room.ProjectID, true) {
		return
	}
	st := h.streamHub().get(msg.ID)
	if st == nil || st.cancel == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "这条回复没有正在进行的输出"})
		return
	}
	st.cancel()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *roomsHandlers) retryMessage(c *gin.Context) {
	room, msg, ok := h.roomMessage(c)
	if !ok {
		return
	}
	if !h.roomAccess(c, room.ProjectID, true) {
		return
	}
	if msg.Kind != domain.ChatKindCrewPost {
		c.JSON(http.StatusBadRequest, gin.H{"error": "只能重试 Bot 的回复"})
		return
	}
	if msg.AgentID != "" && msg.CrewID != "" {
		h.restartCrewPost(msg)
		fresh, _ := h.repo.Message(msg.ID)
		c.JSON(http.StatusOK, fresh)
		return
	}
	prev := h.streamHub().get(msg.ID)
	spec := (*fakeModelSpec)(nil)
	question := ""
	userID := msg.UserID
	if prev != nil {
		if prev.cancel != nil {
			prev.cancel()
		}
		spec, question, userID = prev.spec, prev.question, prev.userID
	}
	if question == "" {
		question = firstNonEmpty(msg.ReplyText, "再答一次")
	}
	msg.Body = ""
	msg.Pending = true
	msg.NodeStatus = ""
	if err := h.repo.PutMessage(msg); err != nil {
		respond(c, err)
		return
	}
	said := domain.ChatMessage{ID: msg.ReplyTo, RoomID: room.ID, UserID: userID, Author: msg.ReplyAuthor, Body: question, ParentID: msg.ParentID, ReplyTo: msg.ParentID}
	h.beginReplyStream(room, said, msg, question, spec)
	c.JSON(http.StatusOK, msg)
}

func (h *roomsHandlers) roomMessage(c *gin.Context) (domain.ChatRoom, domain.ChatMessage, bool) {
	room, ok := h.repo.Room(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return domain.ChatRoom{}, domain.ChatMessage{}, false
	}
	msg, ok := h.repo.Message(c.Param("mid"))
	if !ok || msg.RoomID != room.ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		return domain.ChatRoom{}, domain.ChatMessage{}, false
	}
	return room, msg, true
}

// beginReplyStream writes tokens onto reply until the model finishes, the
// caller stops it, or the fake model disconnects.
func (h *roomsHandlers) beginReplyStream(room domain.ChatRoom, said domain.ChatMessage, reply domain.ChatMessage, question string, spec *fakeModelSpec) {
	ctx, cancel := context.WithCancel(context.Background())
	st := h.streamHub().open(reply.ID, room.ID, question, said.UserID, spec, cancel)
	go func() {
		defer cancel()
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
				reply.Body = b.String()
				_ = h.repo.PutMessage(reply)
			}
			return true
		}
		var err error
		if spec != nil {
			err = runFakeModel(ctx, *spec, emit)
		} else {
			err = h.streamCaptain(ctx, room, said, question, emit, &b)
		}
		reply.Body = strings.TrimSpace(b.String())
		reply.Pending = false
		switch {
		case ctx.Err() != nil:
			reply.NodeStatus = "stopped"
			_ = h.repo.PutMessage(reply)
			st.finish(roomStreamEvent{Type: "stopped", Text: reply.Body})
		case err != nil:
			reply.NodeStatus = "error"
			if reply.Body == "" {
				reply.Body = err.Error()
			}
			_ = h.repo.PutMessage(reply)
			st.finish(roomStreamEvent{Type: "error", Text: err.Error()})
		default:
			_ = h.repo.PutMessage(reply)
			st.finish(roomStreamEvent{Type: "done", Text: reply.Body})
		}
	}()
}

func runFakeModel(ctx context.Context, spec fakeModelSpec, emit func(string) bool) error {
	text := spec.Text
	if strings.TrimSpace(text) == "" {
		text = "这是一条模拟流式回复。"
	}
	tokens := make([]string, 0, utf8.RuneCountInString(text))
	for _, r := range text {
		tokens = append(tokens, string(r))
	}
	first := time.Duration(spec.FirstTokenMs) * time.Millisecond
	if first <= 0 {
		first = 200 * time.Millisecond
	}
	gap := time.Duration(spec.TokenMs) * time.Millisecond
	if gap <= 0 {
		gap = 40 * time.Millisecond
	}
	if !waitCtx(ctx, first) {
		return ctx.Err()
	}
	for i, tok := range tokens {
		n := i + 1
		if spec.DisconnectAfter > 0 && n > spec.DisconnectAfter {
			return errFakeDisconnect
		}
		if spec.StallAfter > 0 && n == spec.StallAfter+1 && spec.StallMs > 0 {
			if !waitCtx(ctx, time.Duration(spec.StallMs)*time.Millisecond) {
				return ctx.Err()
			}
		}
		if !emit(tok) {
			return ctx.Err()
		}
		if n == len(tokens) {
			break
		}
		if !waitCtx(ctx, gap) {
			return ctx.Err()
		}
	}
	return nil
}

var errFakeDisconnect = fakeDisconnectError{}

type fakeDisconnectError struct{}

func (fakeDisconnectError) Error() string { return "模拟模型中途断开" }

func waitCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (h *roomsHandlers) streamCaptain(ctx context.Context, room domain.ChatRoom, said domain.ChatMessage, question string, emit func(string) bool, buf *strings.Builder) error {
	user, ok := h.repo.User(said.UserID)
	status := h.roomStatusDigest(room)
	if !ok || strings.TrimSpace(user.CodeBuddyAPIKey) == "" || crewStatusAsk.MatchString(question) {
		text := h.crewAnswer(room, said, question)
		return emitRunes(ctx, text, emit)
	}
	var filter assistantStreamFilter
	emitted := false
	_, _, err := runAssistantCLI(ctx, assistantCLIRequest{Prompt: h.captainPrompt(room, said, question, status), Model: assistantDefaultModel, UserID: user.ID}, user, func(ev assistantStreamEvent) {
		if !filter.accept(ev) || ev.Kind != "answer" || ev.Text == "" || ev.Full {
			return
		}
		emitted = true
		emit(ev.Text)
	})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !emitted {
		text := h.crewAnswer(room, said, question)
		if buf.Len() == 0 {
			return emitRunes(ctx, text, emit)
		}
	}
	if err != nil && buf.Len() == 0 {
		return err
	}
	return nil
}

func emitRunes(ctx context.Context, text string, emit func(string) bool) error {
	for _, r := range text {
		if !emit(string(r)) {
			return ctx.Err()
		}
		if !waitCtx(ctx, 12*time.Millisecond) {
			return ctx.Err()
		}
	}
	return nil
}
