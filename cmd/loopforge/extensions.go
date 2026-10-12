package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/Tencent/LoopForge/internal/extsrc"
	"github.com/Tencent/LoopForge/internal/httpapi"
	"github.com/Tencent/LoopForge/internal/repo"
)

// configureExtensions loads operator catalogs and web plugins. A broken
// source stops startup: silently dropping a configured catalog would leave
// users with a half-configured instance.
func configureExtensions() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	market := extsrc.Split(os.Getenv("LOOPFORGE_MCP_MARKET"))
	defaults := extsrc.Split(os.Getenv("LOOPFORGE_DEFAULT_PROJECT_MCPS"))
	if len(market) > 0 || len(defaults) > 0 {
		n, err := httpapi.ConfigureMCPMarket(ctx, market, defaults)
		if err != nil {
			log.Fatalf("LOOPFORGE_MCP_MARKET: %v", err)
		}
		log.Printf("MCP market: %d custom entries from %d sources", n, len(market))
	}
	if templates := extsrc.Split(os.Getenv("LOOPFORGE_TEMPLATES")); len(templates) > 0 {
		repo.SetTemplateSources(templates)
		loaded, err := repo.ExternalGenericTemplates(ctx)
		if err != nil {
			log.Fatalf("LOOPFORGE_TEMPLATES: %v", err)
		}
		log.Printf("SOP templates: %d custom templates from %d sources", len(loaded), len(templates))
	}
	if plugins := extsrc.Split(os.Getenv("LOOPFORGE_WEB_PLUGINS")); len(plugins) > 0 {
		n, err := httpapi.ConfigureWebPlugins(ctx, plugins)
		if err != nil {
			log.Fatalf("LOOPFORGE_WEB_PLUGINS: %v", err)
		}
		log.Printf("web plugins: %d files from %d sources", n, len(plugins))
	}
}
