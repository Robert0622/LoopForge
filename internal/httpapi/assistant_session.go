package httpapi

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/LoopForge/internal/repo"
	"github.com/gin-gonic/gin"
)

func (h *assistantHandlers) listSessions(c *gin.Context) {
	current, ok := h.requestUser(c.Request, true)
	if !ok {
		c.JSON(401, gin.H{"error": "请先登录后使用 AI 助手"})
		return
	}
	items, err := listAssistantConversations(h.repo, current.ID)
	if err != nil {
		c.JSON(500, gin.H{"error": "读取对话失败"})
		return
	}
	c.JSON(200, items)
}

func (h *assistantHandlers) session(c *gin.Context) {
	current, ok := h.requestUser(c.Request, true)
	if !ok {
		c.JSON(401, gin.H{"error": "请先登录后使用 AI 助手"})
		return
	}
	id := strings.TrimSpace(c.Param("id"))
	switch c.Request.Method {
	case "GET":
		item, err := loadAssistantConversation(h.repo, current.ID, id)
		if err != nil {
			c.JSON(404, gin.H{"error": "对话不存在"})
			return
		}
		c.JSON(200, item)
	case "PUT":
		existing, _ := loadAssistantConversation(h.repo, current.ID, id)
		var conv assistantConversation
		if !decode(c, &conv) {
			return
		}
		conv.ID = assistantRecordID(id)
		if conv.Title == "" {
			conv.Title = existing.Title
		}
		if conv.ProjectID == "" {
			conv.ProjectID = existing.ProjectID
		}
		if conv.Model == "" {
			conv.Model = existing.Model
		}
		if err := saveAssistantConversation(h.repo, current.ID, conv); err != nil {
			c.JSON(500, gin.H{"error": "保存对话失败"})
			return
		}
		stored, err := loadAssistantConversation(h.repo, current.ID, conv.ID)
		if err != nil {
			c.JSON(500, gin.H{"error": "保存对话失败"})
			return
		}
		c.JSON(200, stored)
	case "DELETE":
		if err := deleteAssistantConversation(h.repo, current.ID, id); err != nil {
			c.JSON(404, gin.H{"error": "对话不存在"})
			return
		}
		c.JSON(200, gin.H{"ok": true})
	default:
		c.JSON(405, gin.H{"error": "method not allowed"})
	}
}

func assistantSessionBase() string {
	if override := strings.TrimSpace(os.Getenv("LOOPFORGE_ASSISTANT_SESSION_DIR")); override != "" {
		return override
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		home = os.TempDir()
	}
	return filepath.Join(home, ".loopforge", "assistant")
}

func assistantSessionRoot(userID int64) string {
	return filepath.Join(assistantSessionBase(), strconv.FormatInt(userID, 10))
}

func assistantCLIHome(userID int64) string {
	return filepath.Join(assistantSessionRoot(userID), "home")
}

func assistantCLIWorkspaceForUser(userID int64) string {
	return filepath.Join(assistantSessionRoot(userID), "ws")
}

func ensureAssistantSessionDirs(userID int64) error {
	for _, dir := range []string{assistantCLIHome(userID), filepath.Join(assistantCLIHome(userID), "tmp"), assistantCLIWorkspaceForUser(userID)} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func newAssistantSessionID() string {
	return "af" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

func assistantRecordID(id string) string {
	id = filepath.Base(strings.TrimSpace(id))
	if id == "" || id == "." || id == ".." {
		return ""
	}
	return id
}

func saveAssistantConversation(st *repo.Store, userID int64, conv assistantConversation) error {
	if st == nil {
		return os.ErrInvalid
	}
	if conv.ID == "" {
		conv.ID = newAssistantSessionID()
	}
	if strings.TrimSpace(conv.Title) == "" {
		for _, m := range conv.Messages {
			if m.Role == "user" && strings.TrimSpace(m.Content) != "" {
				conv.Title = strings.TrimSpace(m.Content)
				break
			}
		}
		if conv.Title == "" {
			conv.Title = "新对话"
		}
	}
	if runes := []rune(conv.Title); len(runes) > 36 {
		conv.Title = string(runes[:36]) + "…"
	}
	return st.PutAssistantSession(userID, conv)
}

func loadAssistantConversation(st *repo.Store, userID int64, id string) (assistantConversation, error) {
	if st == nil {
		return assistantConversation{}, os.ErrNotExist
	}
	item, ok := st.AssistantSession(userID, id)
	if !ok {
		return assistantConversation{}, os.ErrNotExist
	}
	return item, nil
}

func listAssistantConversations(st *repo.Store, userID int64) ([]assistantConversation, error) {
	if st == nil {
		return nil, os.ErrInvalid
	}
	return st.ListAssistantSessions(userID), nil
}

func deleteAssistantConversation(st *repo.Store, userID int64, id string) error {
	id = assistantRecordID(id)
	if st == nil || id == "" {
		return os.ErrNotExist
	}
	err := st.DeleteAssistantSession(userID, id)
	deleteAssistantCLISession(userID, id)
	if err != nil {
		return os.ErrNotExist
	}
	return nil
}

func deleteAssistantCLISession(userID int64, id string) {
	if id == "" || id == "." || id == ".." {
		return
	}
	for _, root := range []string{
		filepath.Join(assistantCLIHome(userID), ".codebuddy", "projects"),
		filepath.Join(assistantCLIHome(userID), ".codebuddy", "sessions"),
	} {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if path == root {
					return err
				}
				return nil
			}
			if path == root {
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if !assistantCLISessionName(d.Name(), id) {
				return nil
			}
			_ = os.RemoveAll(path)
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		})
	}
}

func assistantCLISessionName(name, id string) bool {
	return name == id || strings.HasPrefix(name, id+".")
}
