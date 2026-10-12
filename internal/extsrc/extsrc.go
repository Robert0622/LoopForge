// Package extsrc reads operator-supplied catalogs (MCP market entries, SOP
// templates, web plugins) from local files, directories or http(s) URLs.
package extsrc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MaxDocumentBytes caps a single file or HTTP response.
const MaxDocumentBytes = 8 << 20

// Document is one loaded file; Name is the path or URL it came from.
type Document struct {
	Name string
	Data []byte
}

// Split parses a comma- or newline-separated source list.
func Split(v string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '\n' }) {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func isURL(source string) bool {
	return strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://")
}

// Load reads one source. A directory yields every file with one of the given
// extensions (sorted by name, not recursive); a file or URL yields itself.
func Load(ctx context.Context, source string, exts ...string) ([]Document, error) {
	if isURL(source) {
		data, err := fetch(ctx, source)
		if err != nil {
			return nil, err
		}
		return []Document{{Name: source, Data: data}}, nil
	}
	info, err := os.Stat(source)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		data, err := readFile(source)
		if err != nil {
			return nil, err
		}
		return []Document{{Name: source, Data: data}}, nil
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var out []Document
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || !hasExt(entry.Name(), exts) {
			continue
		}
		path := filepath.Join(source, entry.Name())
		data, err := readFile(path)
		if err != nil {
			return nil, err
		}
		out = append(out, Document{Name: path, Data: data})
	}
	return out, nil
}

// LoadAll loads every source in order.
func LoadAll(ctx context.Context, sources []string, exts ...string) ([]Document, error) {
	var out []Document
	for _, source := range sources {
		docs, err := Load(ctx, source, exts...)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", source, err)
		}
		out = append(out, docs...)
	}
	return out, nil
}

// JSONList decodes a document holding either a single JSON object, an array
// of objects, or an object whose listKey field is such an array.
func JSONList(data []byte, listKey string) ([]json.RawMessage, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, fmt.Errorf("empty document")
	}
	if data[0] == '[' {
		var list []json.RawMessage
		if err := json.Unmarshal(data, &list); err != nil {
			return nil, err
		}
		return list, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	if raw, ok := obj[listKey]; ok && listKey != "" {
		var list []json.RawMessage
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, fmt.Errorf("%s: %w", listKey, err)
		}
		return list, nil
	}
	return []json.RawMessage{data}, nil
}

func hasExt(name string, exts []string) bool {
	if len(exts) == 0 {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	for _, want := range exts {
		if ext == want {
			return true
		}
	}
	return false
}

func readFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readLimited(f)
}

func readLimited(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxDocumentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxDocumentBytes {
		return nil, fmt.Errorf("larger than %d bytes", MaxDocumentBytes)
	}
	return data, nil
}

var httpClient = &http.Client{Timeout: 20 * time.Second}

func fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return readLimited(resp.Body)
}
