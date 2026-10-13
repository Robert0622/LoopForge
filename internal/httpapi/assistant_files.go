package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/gin-gonic/gin"
)

const (
	assistantFileMaxBytes     = 20 << 20
	assistantFilesPerUpload   = 10
	assistantFilesPerMessage  = 10
	assistantGeneratedMax     = 4
	assistantGeneratedMaxSize = 200 << 10
)

var assistantFileIDPattern = regexp.MustCompile(`^[a-f0-9]{24}$`)

type assistantFileMeta struct {
	domain.AssistantAttachment
	Source    string    `json:"source"` // upload, generated
	CreatedAt time.Time `json:"createdAt"`
}

// assistantGeneratedFile is a text file the assistant returns in
// submit_assistant_reply (diagram SVG, Markdown, CSV ...). Content is
// cleared once it has been stored.
type assistantGeneratedFile struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	MimeType string `json:"mimeType,omitempty"`
	Content  string `json:"content,omitempty"`
	Size     int64  `json:"size,omitempty"`
}

var assistantGeneratedExt = map[string]string{
	".svg": "image/svg+xml", ".md": "text/markdown", ".txt": "text/plain", ".csv": "text/csv",
	".json": "application/json", ".yaml": "text/yaml", ".yml": "text/yaml", ".html": "text/html",
	".mmd": "text/plain", ".xml": "application/xml", ".diff": "text/plain", ".patch": "text/plain",
	".sql": "text/plain", ".py": "text/plain", ".js": "text/plain", ".ts": "text/plain", ".go": "text/plain",
	".sh": "text/plain",
}

func assistantFilesRoot(userID int64) string {
	return filepath.Join(assistantSessionRoot(userID), "files")
}

func newAssistantFileID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func cleanAssistantFileName(name string) string {
	name = filepath.Base(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == '/' || r == ':' || r == '"' || r == '<' || r == '>' || r == '|' || r == '?' || r == '*' {
			return '_'
		}
		return r
	}, name)
	if name == "" || name == "." || name == ".." {
		name = "file"
	}
	if utf8.RuneCountInString(name) > 120 {
		ext := filepath.Ext(name)
		name = string([]rune(strings.TrimSuffix(name, ext))[:100]) + ext
	}
	return name
}

func storeAssistantFile(userID int64, name, mimeType, source string, r io.Reader) (assistantFileMeta, error) {
	id := newAssistantFileID()
	dir := filepath.Join(assistantFilesRoot(userID), id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return assistantFileMeta{}, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "blob"), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return assistantFileMeta{}, err
	}
	n, err := io.Copy(f, io.LimitReader(r, assistantFileMaxBytes+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > assistantFileMaxBytes {
		err = fmt.Errorf("单个文件不能超过 %d MB", assistantFileMaxBytes>>20)
	}
	if err != nil {
		_ = os.RemoveAll(dir)
		return assistantFileMeta{}, err
	}
	name = cleanAssistantFileName(name)
	if mimeType == "" || mimeType == "application/octet-stream" {
		if byExt := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); byExt != "" {
			mimeType = byExt
		}
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	meta := assistantFileMeta{
		AssistantAttachment: domain.AssistantAttachment{ID: id, Name: name, Mime: mimeType, Size: n},
		Source:              source,
		CreatedAt:           time.Now().UTC(),
	}
	raw, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), raw, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return assistantFileMeta{}, err
	}
	return meta, nil
}

func loadAssistantFile(userID int64, id string) (assistantFileMeta, string, error) {
	if !assistantFileIDPattern.MatchString(id) {
		return assistantFileMeta{}, "", os.ErrNotExist
	}
	dir := filepath.Join(assistantFilesRoot(userID), id)
	raw, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return assistantFileMeta{}, "", err
	}
	var meta assistantFileMeta
	if err := json.Unmarshal(raw, &meta); err != nil || meta.ID != id {
		return assistantFileMeta{}, "", os.ErrNotExist
	}
	return meta, filepath.Join(dir, "blob"), nil
}

// uploadFiles handles POST /api/assistant/files: multipart parts named "files".
func (h *assistantHandlers) uploadFiles(c *gin.Context) {
	current, ok := h.requestUser(c.Request, true)
	if !ok {
		c.JSON(401, gin.H{"error": "请先登录"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, assistantFilesPerUpload*assistantFileMaxBytes+(1<<20))
	if err := c.Request.ParseMultipartForm(8 << 20); err != nil {
		c.JSON(400, gin.H{"error": fmt.Sprintf("上传内容无效，或超过单个文件 %d MB 的上限", assistantFileMaxBytes>>20)})
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	headers := c.Request.MultipartForm.File["files"]
	if len(headers) == 0 {
		c.JSON(400, gin.H{"error": "缺少名为 files 的上传文件"})
		return
	}
	if len(headers) > assistantFilesPerUpload {
		c.JSON(400, gin.H{"error": fmt.Sprintf("一次最多上传 %d 个文件", assistantFilesPerUpload)})
		return
	}
	out := make([]domain.AssistantAttachment, 0, len(headers))
	for _, fh := range headers {
		if fh.Size > assistantFileMaxBytes {
			c.JSON(400, gin.H{"error": fmt.Sprintf("%s 超过 %d MB", cleanAssistantFileName(fh.Filename), assistantFileMaxBytes>>20)})
			return
		}
		src, err := fh.Open()
		if err != nil {
			c.JSON(400, gin.H{"error": "读取上传文件失败"})
			return
		}
		meta, err := storeAssistantFile(current.ID, fh.Filename, fh.Header.Get("Content-Type"), "upload", src)
		src.Close()
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		out = append(out, meta.AssistantAttachment)
	}
	c.JSON(200, gin.H{"files": out})
}

var assistantInlineMimes = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true, "image/svg+xml": true,
	"application/pdf": true, "text/plain": true, "text/markdown": true, "text/csv": true, "application/json": true,
}

// file handles GET /api/assistant/files/:id. Files are private to their
// owner; everything is served sandboxed so an uploaded SVG or HTML page
// cannot run script on the platform origin.
func (h *assistantHandlers) file(c *gin.Context) {
	current, ok := h.requestUser(c.Request, true)
	if !ok {
		c.JSON(401, gin.H{"error": "请先登录"})
		return
	}
	meta, path, err := loadAssistantFile(current.ID, c.Param("id"))
	if err != nil {
		c.JSON(404, gin.H{"error": "文件不存在"})
		return
	}
	f, err := os.Open(path)
	if err != nil {
		c.JSON(404, gin.H{"error": "文件不存在"})
		return
	}
	defer f.Close()
	ctype := meta.Mime
	if strings.HasPrefix(ctype, "text/") && !strings.Contains(ctype, "charset") {
		ctype += "; charset=utf-8"
	}
	disposition := "attachment"
	if assistantInlineMimes[strings.ToLower(strings.TrimSpace(strings.SplitN(meta.Mime, ";", 2)[0]))] && c.Query("download") == "" {
		disposition = "inline"
	}
	c.Header("Content-Type", ctype)
	c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": meta.Name}))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "sandbox; default-src 'none'; img-src 'self' data:; style-src 'unsafe-inline'")
	c.Header("Cache-Control", "private, max-age=86400")
	http.ServeContent(c.Writer, c.Request, "", meta.CreatedAt, f)
}

// stageAssistantAttachments copies the files of the current user turn into
// the CLI workspace so the model can open them with Read, and returns the
// prompt block that lists their paths.
func stageAssistantAttachments(userID int64, items []domain.AssistantAttachment) (string, []domain.AssistantAttachment, error) {
	if len(items) == 0 {
		return "", nil, nil
	}
	if len(items) > assistantFilesPerMessage {
		return "", nil, fmt.Errorf("每条消息最多 %d 个附件", assistantFilesPerMessage)
	}
	workspace := assistantCLIWorkspaceForUser(userID)
	var lines []string
	clean := make([]domain.AssistantAttachment, 0, len(items))
	for _, item := range items {
		meta, src, err := loadAssistantFile(userID, strings.TrimSpace(item.ID))
		if err != nil {
			return "", nil, errors.New("附件不存在或已过期，请重新上传")
		}
		dstDir := filepath.Join(workspace, "attachments", meta.ID)
		dst := filepath.Join(dstDir, meta.Name)
		if _, err := os.Stat(dst); err != nil {
			if err := os.MkdirAll(dstDir, 0o700); err != nil {
				return "", nil, err
			}
			if err := copyAssistantFile(src, dst); err != nil {
				return "", nil, err
			}
		}
		lines = append(lines, fmt.Sprintf("- %s（%s，%s）：%s", meta.Name, meta.Mime, humanAssistantSize(meta.Size), dst))
		clean = append(clean, meta.AssistantAttachment)
	}
	block := "用户本轮附带了文件，已放在本地，需要时用 Read 工具打开（图片也能直接 Read 查看）：\n" + strings.Join(lines, "\n")
	return block, clean, nil
}

func copyAssistantFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func humanAssistantSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func validateAssistantGeneratedFiles(files []assistantGeneratedFile) ([]assistantGeneratedFile, error) {
	if len(files) > assistantGeneratedMax {
		return nil, fmt.Errorf("files 每轮最多 %d 个", assistantGeneratedMax)
	}
	out := make([]assistantGeneratedFile, 0, len(files))
	for _, f := range files {
		f.Name = cleanAssistantFileName(f.Name)
		ext := strings.ToLower(filepath.Ext(f.Name))
		mimeType, ok := assistantGeneratedExt[ext]
		if !ok {
			return nil, fmt.Errorf("files[%s]：只支持文本类文件（.svg .md .csv .json .html .mmd .txt 等），扩展名决定类型", f.Name)
		}
		if strings.TrimSpace(f.Content) == "" {
			return nil, fmt.Errorf("files[%s]：content 为空", f.Name)
		}
		if len(f.Content) > assistantGeneratedMaxSize {
			return nil, fmt.Errorf("files[%s]：内容超过 %d KB", f.Name, assistantGeneratedMaxSize>>10)
		}
		if ext == ".svg" && !strings.Contains(f.Content, "<svg") {
			return nil, fmt.Errorf("files[%s]：不是有效的 SVG", f.Name)
		}
		f.MimeType = mimeType
		out = append(out, f)
	}
	return out, nil
}

func storeAssistantGeneratedFiles(userID int64, files []assistantGeneratedFile) []assistantGeneratedFile {
	out := make([]assistantGeneratedFile, 0, len(files))
	for _, f := range files {
		meta, err := storeAssistantFile(userID, f.Name, f.MimeType, "generated", strings.NewReader(f.Content))
		if err != nil {
			continue
		}
		out = append(out, assistantGeneratedFile{ID: meta.ID, Name: meta.Name, MimeType: meta.Mime, Size: meta.Size})
	}
	return out
}
