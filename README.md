<p align="center">
  <img src="web/images/loopforge-logo.png" alt="LoopForge logo" width="96">
</p>

<h1 align="center">LoopForge</h1>

<p align="center">SOP-driven delivery pipelines for coding agents, by Tencent open source.</p>

<p align="center">
  <a href="./LICENSE"><img alt="License" src="https://img.shields.io/badge/license-Apache--2.0-FF5A1F?labelColor=16161A&color=FF5A1F&style=flat-square"></a>
  <a href="https://github.com/Tencent/LoopForge/actions/workflows/platform.yml"><img alt="platform" src="https://img.shields.io/github/actions/workflow/status/Tencent/LoopForge/platform.yml?branch=main&label=platform&labelColor=16161A&style=flat-square"></a>
  <a href="https://github.com/Tencent/LoopForge/actions/workflows/validate.yml"><img alt="cli" src="https://img.shields.io/github/actions/workflow/status/Tencent/LoopForge/validate.yml?branch=main&label=cli&labelColor=16161A&style=flat-square"></a>
  <a href="https://www.npmjs.com/package/loopforge-cli"><img alt="npm loopforge-cli" src="https://img.shields.io/npm/v/loopforge-cli?label=npm&labelColor=16161A&color=FF5A1F&style=flat-square"></a>
  <img alt="Go 1.25+" src="https://img.shields.io/badge/go-1.25%2B-00ADD8?labelColor=16161A&style=flat-square">
</p>

<p align="center">
  <b>English</b> · <a href="./README.zh-CN.md">简体中文</a>
</p>

**SOP-driven delivery pipelines for coding agents.** Publish a workflow once. Every issue then goes through the same roles, gates and hand-offs, and ends as a pull request you can review.

## Two ways to use LoopForge

| | **LoopForge Platform** (repository root) | **LoopForge CLI** ([`cli/`](cli/)) |
| --- | --- | --- |
| What it is | A self-hosted service: web studio, SOP engine, runners, REST and MCP APIs | A workflow installed into a single coding agent |
| Where work runs | Your servers and bound machines, in sandboxes, shared by a team | Inside the agent session on your machine |
| Good for | Teams that want every issue to follow the same published pipeline, with approvals, metrics and PR delivery | One developer who wants clarify → design → build → review → test inside their own agent |
| Start | `docker compose up -d` ([Quick start](#quick-start)) | `npx -y loopforge-cli@latest install claude` ([cli/README.md](cli/README.md)) |

Both follow the same idea: confirm the requirement first, separate the roles, and leave reviewable evidence at every stage. They are independent, so either one can be used alone. The rest of this page covers the platform.

Coding agents (Claude Code, Codex, CodeBuddy, ...) are good at single tasks. Teams still end up acting as the **router** between them: open a session to analyze, another to implement, paste the result into a third to review, then check by hand whether the tests really passed and whether a PR was opened.

LoopForge Platform turns that routine into a **published SOP**: a DAG of roles where each step declares what it must hand over, which checks must pass, and where to send the work back when they fail. You dispatch an issue; the pipeline runs on your machines with your CLIs; you approve at the gates you chose and review the PR at the end.

```
GitHub issue ──► Clarify ──► Design ──► Implement ──► Review ──► Test ──► Open PR ──► Summary
                                 ▲            │           │         │
                                 └──── reject with reason ┴─────────┘   (downstream reset, retry)
```

## Highlights

**1. Dispatch once, the pipeline carries it through.** Dependencies, parallel branches, retries and rework targets are part of the SOP, not of someone's memory. Every run is pinned to a frozen snapshot of the SOP, so editing the canvas never re-routes work already in flight.

**2. Hand-offs are artifacts, not chat logs.** Each role declares the files it must deliver (design doc, patch, test report). Accepted outputs are stored content-addressed (SHA-256) with full attempt history, and the next role reads them from its input directory. A rejected attempt never overwrites the evidence of the previous one.

**3. "Done" means verified.** Agents must end with an explicit verdict (pass / rework / fail / needs human) and a reason. Command gates (tests, linters, scripts) run before a human can approve. A rejection jumps back to the chosen step, resets everything downstream, and feeds the reason into the next attempt.

**4. Work stays where your team already is.** GitHub Issues sync both ways (pull, push, periodic auto-sync), delivery goes through an integration branch and a pull request, merged PRs close the loop automatically. Frontend runs publish a live **preview** of the built site, with API calls redirectable to any backend while you review.

**5. Roles are written once and reused.** A role bundles instructions, rules, skills, MCP tools, a self-check loop and its own definition of done. SOPs only pick which role runs a step. Change the review standard in one place and every SOP that uses it follows. Different steps can use different CLIs and models.

**6. Runs on your machines, with your logins.** Bind a laptop or cloud VM with one command; the runs you own execute there, using the CLI you are already logged into or your own API key. Injected keys and Git tokens are scrubbed when a task ends. Sandboxes: Docker, Linux bubblewrap, macOS Seatbelt.

**7. Drive it from the tools you already use.** Every capability is exposed as REST (OpenAPI 3.1) **and** as an MCP server, so Cursor, Claude Code, Codex or CodeBuddy can create projects, write roles, publish SOPs, dispatch issues and approve runs. The built-in assistant drafts roles and SOPs from a conversation and asks before saving.

**8. See whether the process is healthy, not just token counts.** Per run: live model activity, tool calls, tokens and cost, gate results and artifacts (provider-neutral telemetry over SSE, OTLP export). Across runs: closure rate, first-pass rate, rework rate, lead time per phase and per-node reliability, so you fix the step that is flaky instead of writing a longer prompt.

## How it compares

The platform is **not** another coding agent and not a general automation tool. It sits one layer above the agents and makes a team's delivery process repeatable.

| | LoopForge Platform | Agent "coworker" boards (e.g. [Multica](https://github.com/multica-ai/multica)) | Coding agent consoles (e.g. [OpenHands](https://github.com/OpenHands/OpenHands)) | LLM app / automation builders (e.g. [Dify](https://github.com/langgenius/dify), [Langflow](https://github.com/langflow-ai/langflow), [n8n](https://github.com/n8n-io/n8n)) | Multi-agent frameworks (e.g. [CrewAI](https://github.com/crewAIInc/crewAI), [AutoGen](https://github.com/microsoft/autogen), [MetaGPT](https://github.com/FoundationAgents/MetaGPT)) |
| --- | --- | --- | --- | --- | --- |
| Unit of work | A published SOP that every issue follows | An issue assigned to an agent or a squad | A task given to one agent | A chat app or integration flow | A Python program |
| Who routes between roles | The DAG (deterministic, versioned) | A lead agent decides each time via comments | You | Generic nodes | Your code |
| Hand-off between steps | Declared, hashed artifacts | Comments on the issue | n/a | Node outputs | In-memory messages |
| What "done" means | Command gates + explicit verdicts + human approval at chosen gates; reject resets downstream | Run ends; a human closes the issue | Agent says it's done | Flow finished | Program returned |
| Output you review | PR, artifacts, live preview, delivery stages | Issue timeline and diff | Diff / PR | Chat answer / API call | Generated files |
| Where it runs | Your machines and CLIs, sandboxed | Your machines | Hosted or self-hosted runtime | Server | Wherever you run Python |
| Best fit | Repeatable paths: bug fix, feature delivery, review, release | Ad-hoc tasks where you don't know who should take it | One-off coding sessions | Business automation, RAG apps | Research and custom agent programs |

These tools compose well: use an agent board for one-off tasks, and LoopForge once a path is clear enough to write down. The platform deliberately does **not** build its own coding agent, knowledge base, or connector marketplace; it drives the CLIs you already use and talks to other systems over MCP. A role can opt into an external knowledge base that speaks the `kb_search` / `kb_fetch` MCP tools: the run searches and cites, and new notes land in `knowledge.md` for a person to review before they are written back.

## Quick start

### Docker

```bash
git clone https://github.com/Tencent/LoopForge.git && cd LoopForge
docker compose pull    # prebuilt image (linux/amd64, linux/arm64)
docker compose up -d
docker compose logs loopforge | grep "Created admin account"   # one-time admin password
```

Open <http://localhost:8080> and sign in (port taken? `LOOPFORGE_PORT=9090 docker compose up -d`). The image bundles the Claude Code, Codex and CodeBuddy CLIs; add your API key under **Settings** (个人设置).

Without `docker compose pull`, or when the image cannot be pulled, `up` builds it from source instead, which takes a few minutes. `docker compose up -d --build` always builds from source; `LOOPFORGE_VERSION=0.1.0` pins a release.

### Try it without an API key

```bash
LOOPFORGE_DEMO_MODE=true docker compose up -d    # or: make demo
```

Demo mode seeds a scripted executor and a "演示交付流程" SOP (analyze → human approval → implement → report) and makes it the default project SOP. Create a requirement on the dashboard, trigger it, approve the gate, and open the produced `delivery-report.md` under **产物** (artifacts). No model calls are made. The built-in templates stay available and need a real API key.

### From source

Requires Go 1.25+ and Git.

```bash
make run        # SQLite under ~/.loopforge, listens on :8080
```

The first start creates an `admin` account (password printed once in the log, or set `LOOPFORGE_ADMIN_PASSWORD`), a default project, the Claude Code / Codex / CodeBuddy executors, local / bubblewrap / Seatbelt / Docker sandboxes, and three built-in SOP templates. The templates run in the strongest sandbox available on the host (Docker, then bubblewrap, then Seatbelt, then a plain local process; the choice is logged at startup, override with `LOOPFORGE_TEMPLATE_SANDBOX`):

| Template | Steps |
| --- | --- |
| Backend delivery | requirement → design → implement & self-test → independent review → test → open PR → summary |
| Frontend delivery | requirement → design → implement → review → verify → open PR → archive |
| Product delivery | product definition → interactive demo → engineering hand-off → acceptance → issue write-back → summary |

### Connect GitHub

1. In **Settings**, add a GitHub token (classic `repo` scope or a fine-grained token with *Contents*, *Issues* and *Pull requests* read/write) and, for the GitHub MCP tools, a personal MCP token.
2. Bind a repository to your project (paste its URL).
3. Import an issue by URL, or enable auto-sync, then dispatch it to an SOP.

GitHub Enterprise Server: set `LOOPFORGE_GITHUB_API_URL=https://<host>/api/v3` and `LOOPFORGE_GITHUB_HOST=<host>`.

## Concepts

| Concept | What it is |
| --- | --- |
| **Project** | Boundary for issues, repositories, members and the default SOP. |
| **Issue** | A requirement or bug, local or synced from GitHub; new and imported items are scored against the project's clarification policy before dispatch. |
| **SOP (workflow)** | A versioned DAG of agent, script and human-approval nodes with gates, retries and reject targets. |
| **Role (agent)** | Executor-independent definition: instructions, rules, skills, MCP servers, hooks, self-correction loop, artifacts, gates. |
| **Run** | One execution of a frozen SOP snapshot in a shared sandbox/workspace, with telemetry and artifacts. |
| **Runner** | A bound machine that executes the runs its owner is responsible for. |

## Architecture

A single Go binary embeds the web UI (plain JavaScript, no build step). Storage is SQLite by default or MySQL for multi-instance deployments; both go through the same gorm code paths.

```
cmd/loopforge          server (API + UI + scheduler + worker; --role api|worker|all)
cmd/loopforge-runner   runner daemon for bound machines
cmd/loopforge-mcp      stdio MCP bridge to the REST API
cmd/loopforge-gate-mcp gate-result helper injected into agent sessions
internal/engine        DAG scheduling, gates, rework, delivery, integration branch
internal/runtime       executors, sandboxes, MCP materialization, telemetry adapters
internal/tracker       GitHub Issues / Pull Requests provider
internal/httpapi       REST handlers, auth, MCP gateway
internal/repo          storage (SQLite / MySQL), built-in templates
web/                   embedded Studio UI
```

More: [architecture](docs/architecture.md) · [REST API](docs/api/README.md) · [OpenAPI](docs/api/openapi.yaml) · [MCP server](docs/mcp.md) · [observability](docs/observability.md) · [assistant](docs/assistant.md).

## Configuration

Everything is optional; see [.env.example](.env.example) for the full list. The most common settings:

| Variable | Purpose |
| --- | --- |
| `LOOPFORGE_DATA_DIR` | Data directory (SQLite, session secret, workspaces). Default `~/.loopforge`. |
| `LOOPFORGE_DATABASE_URL` | `sqlite:///path/loopforge.db` or `mysql://user:pass@tcp(host:3306)/loopforge`. |
| `LOOPFORGE_ADMIN_USER` / `LOOPFORGE_ADMIN_PASSWORD` | First-boot admin account. |
| `LOOPFORGE_GITHUB_TOKEN` | Server-side fallback token for issue sync and PR delivery. |
| `LOOPFORGE_TRUST_IDENTITY_HEADERS` | Accept `X-Forwarded-User` from a trusted proxy (oauth2-proxy, Authelia, ...). |
| `LOOPFORGE_ROLE` | `all` (default), `api`, or `worker` for split deployments. |
| `LOOPFORGE_DEMO_MODE` | Seed the scripted demo executor and SOP (see *Try it without an API key*). |
| `LOOPFORGE_TEMPLATE_SANDBOX` | Sandbox for the built-in templates: `generic-docker`, `linux-bwrap`, `macos-seatbelt` or `local`. Auto-detected when unset. |
| `LOOPFORGE_MCP_MARKET` / `LOOPFORGE_TEMPLATES` / `LOOPFORGE_WEB_PLUGINS` | Add your own MCP market entries, SOP templates and UI pages from files, directories or URLs. See [docs/extending.md](docs/extending.md). |

Sign-in uses local accounts (bcrypt) by default; put the platform behind an identity-aware proxy for SSO. REST and MCP clients use personal tokens (`Authorization: Bearer afp_...`) created under **接入指南 → 我的 Token** (Connect → My tokens).

## Runners: execute on your own machine

From **Settings → Execution machines**, copy the one-line bind command and run it on the target machine:

```bash
curl -fsSL https://loopforge.example.com/runners/install.sh | bash -s -- \
  --server loopforge.example.com --token bind-xxxx --service
```

The installer downloads the runner from your server (no Go needed), discovers installed CLIs and registers the machine. Set it as your default machine and every node of the runs you own executes there: repository checkout, sandbox, agent CLI and gates. Task state lives in the database, so sleep, crashes and network drops don't lose work; leases expire and tasks are re-queued by policy. Runners update themselves when idle.

## Security

The platform executes agent commands against real repositories. The `local` sandbox is a development convenience, not a boundary. Seatbelt, bubblewrap and Docker are suitable for unattended runs on a trusted host but are not hostile multi-tenant isolation. See [SECURITY.md](SECURITY.md) for deployment guidance and how to report vulnerabilities.

## Status and roadmap

Known limitations:

- The Studio UI is currently in Chinese; English localization is planned.
- AI features outside SOP runs (requirement clarification, the built-in assistant, run evaluation) use CodeBuddy and need a CodeBuddy API key in **Settings**. Without one, create requirements with "跳过分析并建单" (skip analysis).
- Inside the Docker image, templates fall back to the `local` sandbox (no nested Docker or user namespaces). For isolated runs, bind a runner machine that has Docker or bubblewrap and switch the SOP's sandbox in **流程设置** (SOP settings), or run the binary directly on such a host.
- Only one instance per database should execute runs (`LOOPFORGE_EXECUTE_RUNS=false` on read-only replicas).
- Executors: Claude Code, Codex and CodeBuddy. More CLIs (including ACP-compatible agents) are planned.
- Artifacts are stored in the database; an object-storage backend is planned.
- Tracker providers: GitHub. GitLab, Gitea and Jira need a provider implementation in this repository (see [docs/extending.md](docs/extending.md#issue-trackers)).

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md) (code layout rules) and follow our [Code of Conduct](CODE_OF_CONDUCT.md). Changes to the CLI and its workflows follow [cli/CONTRIBUTING.md](cli/CONTRIBUTING.md) and [cli/AGENTS.md](cli/AGENTS.md).

## License

Apache License 2.0. Copyright (C) 2026 Tencent. The CLI under `cli/` stays under the [MIT License](cli/LICENSE). See [LICENSE](LICENSE) and [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for bundled third-party software.
