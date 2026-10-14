package web

import "embed"

//go:embed index.html boot.js i18n.js i18n-en.js design.css vscode.html vscode.js vscode.css app.js app assistant.js assistant.css rooms.js orchestrate.js collab.css metrics-dashboard.css business-domains.css knowledge.js knowledge.css mcp-setup.txt styles.css assets.css platform.css observability.css artifacts.css sops.css sync.css access.css onboarding.css product-v3.css product-v4.css images/credentials/*.svg images/loopforge-logo.png favicon.ico favicon.png mcp-icons monaco vscode-web extensions vendor
var Files embed.FS
