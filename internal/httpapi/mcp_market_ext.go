package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/extsrc"
)

// ConfigureMCPMarket merges operator catalogs into the built-in MCP market and
// sets which market entries every project gets by default. Each source is a
// JSON file, a directory of JSON files or an http(s) URL holding either
// {"items": [...]}, a bare array, or one item. Entries whose id matches a
// built-in entry replace it. An empty defaults list keeps the built-in
// default. It must run before the server starts handling requests.
func ConfigureMCPMarket(ctx context.Context, sources, defaults []string) (int, error) {
	docs, err := extsrc.LoadAll(ctx, sources, ".json")
	if err != nil {
		return 0, fmt.Errorf("MCP market: %w", err)
	}
	var extra []mcpMarketItem
	for _, doc := range docs {
		items, err := parseMarketDocument(doc.Data)
		if err != nil {
			return 0, fmt.Errorf("MCP market %s: %w", doc.Name, err)
		}
		extra = append(extra, items...)
	}
	merged := mergeMarketItems(mcpMarketCatalog.Items, extra)
	previous := builtinMCPMarket
	builtinMCPMarket = merged
	if len(defaults) > 0 {
		for _, id := range defaults {
			if _, ok := marketItem(id); !ok {
				builtinMCPMarket = previous
				return 0, fmt.Errorf("default project MCP %q is not in the MCP market", id)
			}
		}
		defaultProjectMarketMCPAliases = append([]string(nil), defaults...)
	}
	return len(extra), nil
}

func parseMarketDocument(data []byte) ([]mcpMarketItem, error) {
	raws, err := extsrc.JSONList(data, "items")
	if err != nil {
		return nil, err
	}
	out := make([]mcpMarketItem, 0, len(raws))
	for i, raw := range raws {
		var item mcpMarketItem
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("item %d: %w", i, err)
		}
		item.ID = strings.TrimSpace(item.ID)
		item.Name = strings.TrimSpace(item.Name)
		item.URL = strings.TrimSpace(item.URL)
		item.AuthType = strings.TrimSpace(item.AuthType)
		item.Transport = strings.TrimSpace(item.Transport)
		if item.Transport == "" {
			item.Transport = "http"
		}
		if item.Transport != "http" {
			return nil, fmt.Errorf("item %q: market entries must use the http transport", item.ID)
		}
		if item.Category == "" {
			item.Category = "custom"
		}
		check := domain.MCPServerDefinition{
			ID: item.ID, Name: item.Name, Transport: item.Transport, URL: item.URL,
			AuthType: item.AuthType, Scope: "project", ProjectID: "check",
		}
		if err := domain.ValidateMCPServer(check); err != nil {
			return nil, fmt.Errorf("item %q: %w", item.ID, err)
		}
		out = append(out, item)
	}
	return out, nil
}

func mergeMarketItems(base, extra []mcpMarketItem) []mcpMarketItem {
	out := append([]mcpMarketItem(nil), base...)
	index := map[string]int{}
	for i, item := range out {
		index[item.ID] = i
	}
	for _, item := range extra {
		if i, ok := index[item.ID]; ok {
			out[i] = item
			continue
		}
		index[item.ID] = len(out)
		out = append(out, item)
	}
	return out
}
