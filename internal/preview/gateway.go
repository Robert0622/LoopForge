package preview

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/Tencent/LoopForge/internal/domain"
)

//go:embed backend_intercept.js
var previewBackendInterceptJS string

const maxRewrittenHTML = 2 << 20

func init() {
	_ = mime.AddExtensionType(".js", "text/javascript; charset=utf-8")
	_ = mime.AddExtensionType(".mjs", "text/javascript; charset=utf-8")
	_ = mime.AddExtensionType(".css", "text/css; charset=utf-8")
	_ = mime.AddExtensionType(".json", "application/json")
	_ = mime.AddExtensionType(".svg", "image/svg+xml")
	_ = mime.AddExtensionType(".wasm", "application/wasm")
	_ = mime.AddExtensionType(".map", "application/json")
	_ = mime.AddExtensionType(".woff2", "font/woff2")
}

var rootAttrRe = regexp.MustCompile(`(?i)(\s(?:src|href|poster|action)\s*=\s*(['"]))(/[^/][^'"]*)`)
var jsRootAssetRe = regexp.MustCompile("([`'\"])(/(?:assets|static|chunks|media|fonts|css|js|img|images)(?:/[^`'\"]*)?)")

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token, rel, ok := ParsePreviewPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	prefix := PreviewBasePath(token)
	if strings.TrimSuffix(r.URL.Path, "/") == prefix && !strings.HasSuffix(r.URL.Path, "/") {
		http.Redirect(w, r, prefix+"/", http.StatusFound)
		return
	}
	session, preview, ok := s.LookupSession(token)
	if !ok {
		http.Error(w, "preview session expired", http.StatusForbidden)
		return
	}
	s.RenewSession(session)
	if rel == "" || strings.HasSuffix(r.URL.Path, "/") {
		rel = preview.EntryFile
	}
	body, name, err := s.memberBytes(r, preview, rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ext := strings.ToLower(path.Ext(name))
	if ext == ".html" || ext == ".htm" {
		body = rewritePreviewHTML(body, prefix, session.BackendURL)
	} else if ext == ".css" {
		body = []byte(rewriteCSSURLs(string(body), prefix))
	} else if ext == ".js" || ext == ".mjs" || ext == ".cjs" {
		body = []byte(rewriteJSRootURLs(string(body), prefix))
	}
	s.writePreviewHeaders(w, name, session.BackendURL)
	http.ServeContent(w, r, name, preview.UpdatedAt, bytes.NewReader(body))
}

func (s *Service) memberBytes(r *http.Request, preview domain.RunPreview, rel string) ([]byte, string, error) {
	safe, err := SanitizeArchiveMember(rel)
	if err != nil {
		return nil, "", err
	}
	body, err := s.readMember(r, preview, safe)
	if err == nil {
		return body, safe, nil
	}
	if preview.SPAFallback && acceptsHTML(r) {
		body, err = s.readMember(r, preview, preview.EntryFile)
		if err == nil {
			return body, preview.EntryFile, nil
		}
	}
	return nil, "", err
}

func (s *Service) readMember(r *http.Request, preview domain.RunPreview, name string) ([]byte, error) {
	if preview.StorageKey == "" && preview.ContentSHA256 == "" {
		return nil, os.ErrNotExist
	}
	key := preview.StorageKey
	if key == "" {
		key = preview.ContentSHA256
	}
	obj, _, err := s.store.Open(r.Context(), key)
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	body, err := ReadTarZstMember(obj, name, s.limits())
	if errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return body, err
}

func (s *Service) writePreviewHeaders(w http.ResponseWriter, name, backend string) {
	ext := strings.ToLower(path.Ext(name))
	ctype := mime.TypeByExtension(ext)
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), clipboard-read=(), clipboard-write=()")
	w.Header().Set("Content-Security-Policy", s.previewCSP(backend))
	w.Header().Set("Cache-Control", "no-cache")
	if ext != ".html" && ext != ".htm" && strings.Contains(name, ".") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
}

func (s *Service) previewCSP(backend string) string {
	ancestors := strings.TrimSpace(s.cfg.FrameAncestors)
	if ancestors == "" {
		ancestors = "'self'"
	} else if !strings.Contains(ancestors, "'self'") {
		ancestors = "'self' " + ancestors
	}
	connect := "'none'"
	if origin := backendOrigin(backend); origin != "" {
		connect = "'self' " + origin
	}
	return "sandbox allow-scripts allow-forms allow-modals allow-downloads allow-same-origin; frame-ancestors " + ancestors + "; base-uri 'self'; connect-src " + connect + "; form-action 'self'"
}

func backendOrigin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func acceptsHTML(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return accept == "" || strings.Contains(accept, "text/html") || strings.Contains(accept, "*/*")
}

func rewritePreviewHTML(body []byte, prefix, backend string) []byte {
	if len(body) > maxRewrittenHTML {
		return body
	}
	html := injectBaseTag(string(body), prefix)
	html = injectPreviewViewport(html)
	html = injectPreviewBackend(html, prefix, backend)
	html = rewriteRootRelativeRefs(html, prefix)
	html = rewriteCSSURLs(html, prefix)
	return []byte(html)
}

func injectBaseTag(html, prefix string) string {
	if strings.Contains(strings.ToLower(html), "<base") {
		return html
	}
	href := prefix
	if href == "" {
		href = "/"
	}
	if !strings.HasSuffix(href, "/") {
		href += "/"
	}
	tag := `<base href="` + href + `">`
	lower := strings.ToLower(html)
	if i := strings.Index(lower, "<head>"); i >= 0 {
		at := i + len("<head>")
		return html[:at] + tag + html[at:]
	}
	if i := strings.Index(lower, "<head "); i >= 0 {
		if end := strings.Index(html[i:], ">"); end >= 0 {
			at := i + end + 1
			return html[:at] + tag + html[at:]
		}
	}
	return tag + html
}

func injectPreviewViewport(html string) string {
	const tag = `<style data-af-preview-viewport>html,body{height:100%;min-height:100%;margin:0}</style>`
	if strings.Contains(html, `data-af-preview-viewport`) {
		return html
	}
	lower := strings.ToLower(html)
	if i := strings.Index(lower, "</head>"); i >= 0 {
		return html[:i] + tag + html[i:]
	}
	return tag + html
}

func injectPreviewBackend(html, prefix, backend string) string {
	backend = strings.TrimSpace(backend)
	if backend == "" || strings.Contains(html, `data-af-preview-backend`) {
		return html
	}
	raw, err := json.Marshal(backend)
	if err != nil {
		return html
	}
	prefixJSON, err := json.Marshal(strings.TrimRight(prefix, "/"))
	if err != nil {
		return html
	}
	if strings.Contains(previewBackendInterceptJS, "</script>") {
		return html
	}
	// Classic script at the start of <head> runs before deferred module
	// bundles. Axios/jQuery/ky and the rest go through fetch or XHR;
	// sendBeacon/EventSource/WebSocket are patched the same way. Preview
	// static files under prefix stay local; every other http(s) call is
	// sent to the configured backend origin with path/query unchanged.
	intercept := strings.Replace(previewBackendInterceptJS, "__AF_PREFIX__", string(prefixJSON), 1)
	tag := `<script data-af-preview-backend>window.__LOOPFORGE_PREVIEW_BACKEND__=` + string(raw) + `;` + intercept + `</script>`
	lower := strings.ToLower(html)
	if i := strings.Index(lower, "<head>"); i >= 0 {
		at := i + len("<head>")
		return html[:at] + tag + html[at:]
	}
	if i := strings.Index(lower, "<head "); i >= 0 {
		if end := strings.Index(html[i:], ">"); end >= 0 {
			at := i + end + 1
			return html[:at] + tag + html[at:]
		}
	}
	return tag + html
}
func rewriteRootRelativeRefs(html, prefix string) string {
	if prefix == "" {
		return html
	}
	return rootAttrRe.ReplaceAllStringFunc(html, func(m string) string {
		parts := rootAttrRe.FindStringSubmatch(m)
		if len(parts) != 4 {
			return m
		}
		ref := parts[3]
		if strings.HasPrefix(ref, prefix+"/") || ref == prefix {
			return m
		}
		return parts[1] + prefix + ref
	})
}

func rewriteCSSURLs(s, prefix string) string {
	var b strings.Builder
	i := 0
	for {
		j := strings.Index(s[i:], "url(")
		if j < 0 {
			b.WriteString(s[i:])
			break
		}
		j += i
		b.WriteString(s[i : j+4])
		k := j + 4
		for k < len(s) && (s[k] == ' ' || s[k] == '\t') {
			b.WriteByte(s[k])
			k++
		}
		if k < len(s) && (s[k] == '\'' || s[k] == '"') {
			b.WriteByte(s[k])
			k++
		}
		if k < len(s) && s[k] == '/' && (k+1 >= len(s) || s[k+1] != '/') {
			if !strings.HasPrefix(s[k:], prefix) {
				b.WriteString(prefix)
			}
		}
		i = k
	}
	return b.String()
}

// rewriteJSRootURLs prefixes quoted root-relative asset paths inside JS
// bundles. Dynamic concatenation and non-asset routes such as /api stay
// untouched; builds should still use a relative base for those cases.
func rewriteJSRootURLs(s, prefix string) string {
	if prefix == "" || len(s) > maxRewrittenHTML {
		return s
	}
	return jsRootAssetRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := jsRootAssetRe.FindStringSubmatch(m)
		if len(parts) != 3 {
			return m
		}
		ref := parts[2]
		if strings.HasPrefix(ref, prefix+"/") || ref == prefix {
			return m
		}
		return parts[1] + prefix + ref
	})
}
