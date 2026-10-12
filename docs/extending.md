# Extending LoopForge

LoopForge can be customized without forking: point it at your own MCP catalog,
SOP templates and web plugins. Every setting takes a comma-separated list of
sources; a source is a local file, a directory (its files are read in name
order, not recursively) or an `http(s)://` URL. Sources are read once at
startup. If a configured source is missing or invalid, the server refuses to
start and prints the file and the reason.

A working example of all three lives in [`examples/extensions`](../examples/extensions)
and is loaded by the browser test suite:

```bash
LOOPFORGE_MCP_MARKET=examples/extensions/mcp-market.json \
LOOPFORGE_TEMPLATES=examples/extensions/templates \
LOOPFORGE_WEB_PLUGINS=examples/extensions/plugins \
./bin/loopforge
```

With Docker, mount the directory and use container paths:

```bash
docker run -p 8080:8080 -v loopforge-data:/data \
  -v "$PWD/examples/extensions:/extensions:ro" \
  -e LOOPFORGE_MCP_MARKET=/extensions/mcp-market.json \
  -e LOOPFORGE_TEMPLATES=/extensions/templates \
  -e LOOPFORGE_WEB_PLUGINS=/extensions/plugins \
  ghcr.io/tencent/loopforge:latest
```

## MCP market (`LOOPFORGE_MCP_MARKET`)

Adds entries to **能力资产 → MCP 管理 → MCP 市场** (MCP market). A document holds
`{"items": [...]}`, a bare array, or a single item — the same shape as the
built-in [`mcp_market_catalog.json`](../internal/httpapi/mcp_market_catalog.json):

```json
{
  "items": [
    {
      "id": "team-knowledge",
      "name": "Team knowledge base",
      "description": "Search our design docs",
      "category": "knowledge",
      "tags": ["docs"],
      "url": "https://mcp.example.com/knowledge",
      "authType": "personal-token",
      "detailUrl": "https://wiki.example.com/mcp",
      "aliases": ["kb"]
    }
  ]
}
```

- `id`, `name` and an `http(s)` `url` are required. Market entries use the
  HTTP transport; add stdio servers as custom MCPs in the UI instead.
- `authType` is empty (no auth) or `personal-token`: each run owner's personal
  MCP token (set under **个人设置**) is sent as a bearer token.
- An entry whose `id` matches a built-in entry replaces it — for example,
  point `github` at GitHub Enterprise.

`LOOPFORGE_DEFAULT_PROJECT_MCPS` lists the market ids (or aliases) installed
on every project, replacing the default `github`. Every id must exist in the
market.

## SOP templates (`LOOPFORGE_TEMPLATES`)

Adds platform-maintained templates next to the built-in ones in **流程模板 →
通用模板** (SOP templates → generic). Projects copy them or run them directly. A
document holds one template, an array of templates or `{"templates": [...]}`.
The format is the same as the built-in templates in
[`internal/repo/templates`](../internal/repo/templates); a minimal one is
[`examples/extensions/templates/docs-update.json`](../examples/extensions/templates/docs-update.json).

- `Workflow.id` and at least one node are required. Every agent node's
  `agentId` must be defined in the template's `Agents`, and every edge must
  connect existing nodes. Agents may reference the shared rules and skills
  (`engineering`, `security`, `review`, `repository-navigation`, ...) or bring
  their own in `Rules` and `Skills`.
- When `Workflow.sandboxProfileId` is empty, the template uses the same
  sandbox as the built-in templates (`LOOPFORGE_TEMPLATE_SANDBOX`).
- **Updates:** a template is inserted when missing. A stored template is
  replaced only when the file declares a higher `Workflow.version`, so
  platform admins' edits in the UI survive restarts. Bump `version` to roll
  out a new revision; it overwrites those edits.

## Web plugins (`LOOPFORGE_WEB_PLUGINS`)

Loads `.js` and `.css` files into the Studio UI. Files are read at startup
and served from `/plugins/` (URLs included), which keeps the strict
same-origin content security policy intact. Stylesheets load in `<head>`;
scripts run after the core UI in list order.

Scripts use the `window.LoopForge` API (`apiVersion: 1`):

| Member | Description |
| --- | --- |
| `registerView({id, title, icon, render})` | Adds a page under the **扩展** (Extensions) sidebar group. `id` is lowercase letters, digits and `-`; the page is reachable at `#/<id>`. `render(container, ctx)` runs every time the page opens and may be async. |
| `api(path, options)` | Calls the REST API with the signed-in user's session; resolves to parsed JSON and throws an `Error` with the server's message on failure. Other options go to `fetch`; `{silent: true}` hides the top progress bar. |
| `toast(message, type)` | Shows a notification; `type` is `info`, `ok` or `err`. |
| `escapeHTML(value)` | Escapes text for `innerHTML`. Always escape API data. |
| `currentProject()` / `currentUser()` | The selected project and the signed-in user (`null` before sign-in). |
| `navigate(view)` | Switches to a built-in or plugin page. |

`ctx` passed to `render` has `project`, `user`, `isPlatformAdmin` and the same
`api`, `toast` and `escapeHTML` helpers. Plugins run with the user's
permissions, so the server still enforces access control; only load plugins
you trust, the same way you would review any code deployed on the server.

```js
(function () {
  const { registerView, api, escapeHTML } = window.LoopForge;
  registerView({
    id: 'my-report',
    title: 'My report',
    async render(el, ctx) {
      const runs = await api(`/api/runs?projectId=${encodeURIComponent(ctx.project.id)}`);
      el.innerHTML = `<div style="padding:24px">${escapeHTML(ctx.project.name)}: ${runs.length} runs</div>`;
    },
  });
})();
```

The REST API is described in [docs/api](api).

## Issue trackers

GitHub is the built-in tracker. The forge abstraction lives in
[`internal/tracker/provider.go`](../internal/tracker/provider.go); adding
GitLab, Gitea or Jira means implementing that interface inside this
repository. Contributions are welcome — please open an issue first so the
source model (issue references, branch naming, delivery links) can be agreed
on together.
