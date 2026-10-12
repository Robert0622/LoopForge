package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strings"

	"github.com/Tencent/LoopForge/internal/extsrc"
)

type webPlugin struct {
	path        string
	contentType string
	version     string
	data        []byte
}

var (
	webPlugins        []webPlugin
	webPluginUnsafeCh = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
)

// ConfigureWebPlugins loads .js and .css files that extend the web UI. Each
// source is a file, a directory or an http(s) URL; files are read once and
// served from /plugins/ so they satisfy the same-origin script policy. Scripts
// load after the core UI and use window.LoopForge. It must run before the
// server starts handling requests.
func ConfigureWebPlugins(ctx context.Context, sources []string) (int, error) {
	docs, err := extsrc.LoadAll(ctx, sources, ".js", ".css")
	if err != nil {
		return 0, fmt.Errorf("web plugins: %w", err)
	}
	loaded := make([]webPlugin, 0, len(docs))
	for i, doc := range docs {
		name := doc.Name
		if idx := strings.IndexAny(name, "?#"); idx >= 0 {
			name = name[:idx]
		}
		base := path.Base(strings.ReplaceAll(name, "\\", "/"))
		var contentType string
		switch strings.ToLower(path.Ext(base)) {
		case ".js":
			contentType = "text/javascript; charset=utf-8"
		case ".css":
			contentType = "text/css; charset=utf-8"
		default:
			return 0, fmt.Errorf("web plugin %s: only .js and .css files are supported", doc.Name)
		}
		sum := sha256.Sum256(doc.Data)
		loaded = append(loaded, webPlugin{
			path:        fmt.Sprintf("/plugins/%02d-%s", i+1, webPluginUnsafeCh.ReplaceAllString(base, "-")),
			contentType: contentType,
			version:     hex.EncodeToString(sum[:8]),
			data:        doc.Data,
		})
	}
	webPlugins = loaded
	return len(loaded), nil
}

func serveWebPlugin(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/plugins/") {
		return false
	}
	for _, p := range webPlugins {
		if p.path == r.URL.Path {
			w.Header().Set("Content-Type", p.contentType)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(p.data)
			return true
		}
	}
	http.NotFound(w, r)
	return true
}

func injectWebPlugins(html string) string {
	var styles, scripts strings.Builder
	for _, p := range webPlugins {
		ref := p.path + "?v=" + p.version
		if strings.HasPrefix(p.contentType, "text/css") {
			fmt.Fprintf(&styles, "<link rel=\"stylesheet\" href=%q>\n", ref)
		} else {
			fmt.Fprintf(&scripts, "<script src=%q></script>\n", ref)
		}
	}
	if styles.Len() > 0 {
		html = strings.Replace(html, "</head>", styles.String()+"</head>", 1)
	}
	if scripts.Len() > 0 {
		if i := strings.LastIndex(html, "</body>"); i >= 0 {
			html = html[:i] + scripts.String() + html[i:]
		}
	}
	return html
}
