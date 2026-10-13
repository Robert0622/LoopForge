<p align="center">
  <img src="web/images/loopforge-logo.png" alt="LoopForge logo" width="96">
</p>

<h1 align="center">LoopForge</h1>

<p align="center"><b>A fully automated loop from requirement to pull request.</b><br>Coding agents run your team's SOP end to end. People stay on the loop, not in it.</p>

<p align="center">
  <a href="./LICENSE"><img alt="License" src="https://img.shields.io/badge/license-MIT-FF5A1F?labelColor=16161A&color=FF5A1F&style=flat-square"></a>
  <a href="https://github.com/Tencent/LoopForge/actions/workflows/platform.yml"><img alt="platform" src="https://img.shields.io/github/actions/workflow/status/Tencent/LoopForge/platform.yml?branch=main&label=platform&labelColor=16161A&style=flat-square"></a>
  <a href="https://github.com/Tencent/LoopForge/actions/workflows/validate.yml"><img alt="cli" src="https://img.shields.io/github/actions/workflow/status/Tencent/LoopForge/validate.yml?branch=main&label=cli&labelColor=16161A&style=flat-square"></a>
  <a href="https://www.npmjs.com/package/loopforge-cli"><img alt="npm loopforge-cli" src="https://img.shields.io/npm/v/loopforge-cli?label=npm&labelColor=16161A&color=FF5A1F&style=flat-square"></a>
  <img alt="Go 1.25+" src="https://img.shields.io/badge/go-1.25%2B-00ADD8?labelColor=16161A&style=flat-square">
</p>

<p align="center">
  <b>English</b> · <a href="./README.zh-CN.md">简体中文</a>
</p>

Hand LoopForge a requirement. Coding agents take it through your team's SOP (clarify, design, implement, review, test, open a pull request) and come back with something you can merge. Gates decide whether each step really passed, and failed work goes back to the right step with the reason attached. You decide where a person has to sign off; everywhere else the loop keeps running on its own. That is **Human on the Loop**.

LoopForge grew out of the R&D SOPs of Tencent teams and is used by multiple project teams inside Tencent, including SkillHub and teams with demanding quality requirements.

<p align="center">
  <img src="docs/images/run.png" alt="A LoopForge run: every SOP step finished, with its artifacts, telemetry and evaluation" width="100%">
</p>

## Three things LoopForge does differently

### 1. A fully automated loop for requirements

- **Requirements come in on their own.** GitHub Issues sync on a schedule. Each new item is scored against the project's clarification bar, and anything below the bar is sent back to clarification instead of reaching the coding agents.
- **Dispatch needs no one.** Scheduled triggers pick up every ready requirement and start its SOP. Integration-branch conflicts are resolved by an alignment agent, and the same requirement is never run twice.
- **"Done" is checked, not claimed.** Every step ends with an explicit verdict (pass, rework, fail, needs a human). Command gates such as tests, linters and scripts must pass. A rejection rewinds to the chosen step, resets everything downstream and carries the reason into the next attempt.
- **Every run in its own container.** Each requirement runs in a fresh container with its own workspace, removed when the run ends, out of the box with `docker compose`. Runs never see each other's files or processes.
- **Delivery closes itself.** Work lands on an integration branch and a pull request. Once the PR is merged, the requirement closes.
- **Humans where you want them.** Approval steps are optional nodes in the SOP: require sign-off on the design and nothing else, or run the whole path unattended.

### 2. Observable and evaluable, built to improve itself

- **See every run as it happens.** Model activity, tool calls, tokens and cost, gate results and artifacts stream live, with provider-neutral telemetry and OTLP export.
- **Measure the process, not just the tokens.** Closure rate, first-pass rate, rework rate, lead time per phase and the reliability of each node show which step needs fixing.
- **Every run is evaluated.** An evaluation scores the run and proposes concrete changes, such as adjusting an SOP node, adding or removing a gate, or tightening a role's instructions. Each change can be applied in one click.
- **Next: a closed self-feedback loop.** Evaluation results will feed back into roles and SOPs automatically, so the pipeline improves from its own runs. See the [roadmap](docs/roadmap.md).

### 3. Expert experience, captured as SOPs, with shared context

- **SOPs carry how experts work.** The built-in backend, frontend and product delivery SOPs encode how designers, engineers, reviewers, testers and product owners hand work to each other. Publish your own and every requirement follows it.
- **Roles are written once and reused.** A role bundles instructions, rules, skills, MCP tools, a self-check loop and its own definition of done. Change the review standard in one place and every SOP that uses it follows. Steps can use different CLIs and models.
- **Context is shared, not re-explained.** Each step hands over declared artifacts (design doc, patch, test report), stored content-addressed with full attempt history, so the next role starts from real deliverables instead of chat logs.
- **Knowledge accumulates.** Roles can search a team knowledge base over MCP and cite it. New findings are queued for review before they are written back.

### Also included

- **Runs on your machines, with your logins.** Bind a laptop or cloud VM with one command and use the CLI you are already signed into. Injected keys and Git tokens are scrubbed when a task ends. Sandboxes: Docker, Linux bubblewrap, macOS Seatbelt.
- **Drive it from your own tools.** Every capability is exposed over REST (OpenAPI 3.1) and as an MCP server for Cursor, Claude Code, Codex and CodeBuddy. A built-in assistant drafts roles and SOPs from a conversation.
- **Previews.** Frontend runs publish a live preview of the built site, with API calls redirectable to any backend.
- **Pinned SOP versions.** Every run is frozen to the SOP version it started with, so editing the canvas never re-routes work in flight.

```
GitHub issue ──► Clarify ──► Design ──► Implement ──► Review ──► Test ──► Open PR ──► Summary
                                 ▲            │           │         │
                                 └──── reject with reason ┴─────────┘   (downstream reset, retry)
```

## Two ways to use LoopForge

| | **LoopForge Platform** (repository root) | **LoopForge CLI** ([`cli/`](cli/)) |
| --- | --- | --- |
| What it is | A self-hosted service: web studio, SOP engine, runners, REST and MCP APIs | A workflow installed into a single coding agent |
| Where work runs | Your servers and bound machines, in sandboxes, shared by a team | Inside the agent session on your machine |
| Good for | Teams that want every requirement to follow the same published pipeline, with approvals, metrics and PR delivery | One developer who wants clarify → design → build → review → test inside their own agent |
| Start | `docker compose up -d` ([Quick start](#quick-start)) | `npx -y loopforge-cli@latest install claude` ([cli/README.md](cli/README.md)) |

The platform is the deployed edition of the CLI workflow. Both follow the same idea: confirm the requirement first, separate the roles, and leave reviewable evidence at every stage. The CLI runs it inside one developer's agent with nothing to deploy; the platform runs it for a whole team, unattended. Either can be used alone. The rest of this page covers the platform.

## How it compares

LoopForge is **not** another coding agent and not a general automation tool. It sits one layer above the agents and makes a team's delivery process repeatable.

| | LoopForge Platform | Agent "coworker" boards (e.g. [Multica](https://github.com/multica-ai/multica)) | Coding agent consoles (e.g. [OpenHands](https://github.com/OpenHands/OpenHands)) | LLM app / automation builders (e.g. [Dify](https://github.com/langgenius/dify), [Langflow](https://github.com/langflow-ai/langflow), [n8n](https://github.com/n8n-io/n8n)) | Multi-agent frameworks (e.g. [CrewAI](https://github.com/crewAIInc/crewAI), [AutoGen](https://github.com/microsoft/autogen), [MetaGPT](https://github.com/FoundationAgents/MetaGPT)) |
| --- | --- | --- | --- | --- | --- |
| Unit of work | A published SOP that every requirement follows | An issue assigned to an agent or a squad | A task given to one agent | A chat app or integration flow | A Python program |
| Who routes between roles | The DAG (deterministic, versioned) | A lead agent decides each time via comments | You | Generic nodes | Your code |
| Hand-off between steps | Declared, hashed artifacts | Comments on the issue | n/a | Node outputs | In-memory messages |
| What "done" means | Command gates + explicit verdicts + human approval at chosen gates; reject resets downstream | Run ends; a human closes the issue | Agent says it's done | Flow finished | Program returned |
| Output you review | PR, artifacts, live preview, delivery stages | Issue timeline and diff | Diff / PR | Chat answer / API call | Generated files |
| Where it runs | Your machines and CLIs, sandboxed | Your machines | Hosted or self-hosted runtime | Server | Wherever you run Python |
| Best fit | Repeatable paths: bug fix, feature delivery, review, release | Ad-hoc tasks where you don't know who should take it | One-off coding sessions | Business automation, RAG apps | Research and custom agent programs |

These tools compose well: use an agent board for one-off tasks, and LoopForge once a path is clear enough to write down. LoopForge deliberately does **not** build its own coding agent or connector marketplace; it drives the CLIs you already use and talks to other systems over MCP.

## Quick start

### One command (recommended)

On a networked Linux or macOS host, `./start.sh` detects what is missing (curl / git / Docker / Compose), installs it after confirmation, builds the checkout you cloned (release tags pull the published image), moves to the next free port if 8080 is taken, waits for health, and prints the login. The first build takes a few minutes; Go/npm mirrors are used automatically when the default registries are unreachable:

```bash
git clone https://github.com/Tencent/LoopForge.git && cd LoopForge
./start.sh -y          # -y: install missing deps without prompts
```

Or without cloning first:

```bash
curl -fsSL https://raw.githubusercontent.com/Tencent/LoopForge/main/install.sh | bash -s -- -y
```

Then open the URL it prints (normally <http://localhost:8080>). The first visitor creates the admin account; after that, teammates register from the login page (an admin can turn sign-up off under **平台管理**). A short tour ends on **个人设置**, where you paste a model key and a GitHub token.

To try it without any key, open **工作总览**, click **一键演示**, approve the gate, and open `delivery-report.md` under **产物**. No model is called.

Helpers:

- `./start.sh doctor` checks the host, then runs an end-to-end self-test inside the container: login, environment check, one demo requirement through dispatch, approval via the public share link, and the delivery report.
- `./start.sh warm` builds the default sandbox image now (it also builds in the background on first start).
- `./start.sh logs` · `./start.sh down` (keeps data) · `./start.sh reset` (deletes data) · `./start.sh pull` (published image)

The **接入指南 → 最小接入配置** page shows the same environment check in the UI, with a fix button for each missing item.

<p align="center">
  <img src="docs/images/dashboard.png" alt="The LoopForge dashboard after a few demo requirements" width="100%">
</p>

### Run real requirements (four steps)

Start without `LOOPFORGE_DEMO_MODE`, then:

1. **Keys**: follow the first-run wizard to paste a Claude / Codex / CodeBuddy API key (replay from 接入指南); optionally add a GitHub token.
2. **Repo**: bind a repository URL on the project.
3. **Dispatch**: describe a change in the dashboard chat, or import an issue; the default short SOP is **快速修复** (implement → approve → summary).
4. **Approve**: open **待我确认**, or copy an external approval link for a colleague (no login, 24h TTL).

Longer built-in SOPs:

| SOP | Steps |
| --- | --- |
| Quick fix | implement → human approve → summary |
| Backend delivery | requirement → design → implement & self-test → independent review → test → open PR → summary |
| Frontend delivery | requirement → design → implement → review → verify → open PR → archive |
| Product delivery | product definition → interactive demo → engineering hand-off → acceptance → issue write-back → summary |

Optional: set `LOOPFORGE_NOTIFY_WEBHOOK` to a group-chat robot URL for waiting/failed alerts. Set `LOOPFORGE_PUBLIC_URL` for absolute links in notifications and share URLs.

The first real run builds the default sandbox image (Node, the agent CLIs, Go, Git), which takes a few minutes once. A full backend requirement typically takes 30 to 75 minutes; small fixes via **快速修复** are much shorter.

GitHub Enterprise Server: set `LOOPFORGE_GITHUB_API_URL=https://<host>/api/v3` and `LOOPFORGE_GITHUB_HOST=<host>`.

### Other ways to start

- `docker compose up -d --build` always builds the image from source. Without `docker compose pull`, `up` does the same automatically, which takes a few minutes. `LOOPFORGE_VERSION=0.1.0` pins a release.
- From source (Go 1.25+ and Git): `make run` keeps SQLite under `~/.loopforge` and listens on `:8080`; `make demo` starts demo mode. To run a binary, use `make build` and keep the `bin/loopforge-gate-mcp-*` helpers next to `bin/loopforge`; Agent runs need them, and the server logs a warning at startup when they are missing.

## Where runs execute

Every run gets its own environment. In the `docker compose` setup, LoopForge uses the host's Docker through the mounted socket and starts **one container per run**: the steps of a run share that container and its workspace, other runs cannot see it, and it is removed when the run ends. Containers drop all Linux capabilities and join the LoopForge network only to report back to the platform.

| Setup | Isolation |
| --- | --- |
| `docker compose up -d` (default) | One container per run on the host's Docker |
| Binary on a host with Docker, or a bound runner machine with Docker | One container per run |
| Linux host with bubblewrap, or macOS, without Docker | One bubblewrap or Seatbelt sandbox per run |
| `docker compose` without the Docker socket | Runs are processes inside the LoopForge container, each in its own workspace directory |

Mounting the Docker socket gives LoopForge root-equivalent access to the host, the same as installing it directly on the host. If that is not acceptable, delete `user: root` and the `docker.sock` line in `docker-compose.yml` and bind a separate runner machine with Docker for isolated runs.

The sandbox is picked automatically at startup (Docker, then bubblewrap or Seatbelt, then local) and logged. Change it per SOP in **流程设置** (SOP settings) or globally with `LOOPFORGE_TEMPLATE_SANDBOX`. See [Runners](#runners-execute-on-your-own-machine) for running on other machines.

## Concepts

| Concept | What it is |
| --- | --- |
| **Project** | Boundary for requirements, repositories, members and the default SOP. |
| **Requirement / issue** | Local or synced from GitHub; scored against the project's clarification bar before dispatch. |
| **SOP (workflow)** | A versioned DAG of agent, script and human-approval nodes with gates, retries and reject targets. |
| **Role (agent)** | Executor-independent definition: instructions, rules, skills, MCP servers, hooks, self-check loop, artifacts, gates. |
| **Run** | One execution of a frozen SOP snapshot in one sandbox/workspace, with telemetry, artifacts and an evaluation. |
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
internal/runeval       run evaluation and applying its recommendations
internal/tracker       GitHub Issues / Pull Requests provider
internal/httpapi       REST handlers, auth, MCP gateway
internal/repo          storage (SQLite / MySQL), built-in templates
web/                   embedded Studio UI
```

More: [architecture](docs/architecture.md) · [REST API](docs/api/README.md) · [OpenAPI](docs/api/openapi.yaml) · [MCP server](docs/mcp.md) · [observability](docs/observability.md) · [assistant](docs/assistant.md) · [extending](docs/extending.md) · [roadmap](docs/roadmap.md).

## Configuration

Everything is optional; see [.env.example](.env.example) for the full list. The most common settings:

| Variable | Purpose |
| --- | --- |
| `LOOPFORGE_DATA_DIR` | Data directory (SQLite, session secret, workspaces). Default `~/.loopforge`. |
| `LOOPFORGE_DATABASE_URL` | `sqlite:///path/loopforge.db` or `mysql://user:pass@tcp(host:3306)/loopforge`. |
| `LOOPFORGE_ADMIN_USER` / `LOOPFORGE_ADMIN_PASSWORD` | Create the admin at boot instead of in the browser (useful for scripted installs). |
| `LOOPFORGE_SIGNUP` | `closed` turns off self-registration by default; admins can still toggle it under 平台管理. |
| `LOOPFORGE_GITHUB_TOKEN` | Server-side fallback token for issue sync and PR delivery. |
| `LOOPFORGE_TRUST_IDENTITY_HEADERS` | Accept `X-Forwarded-User` from a trusted proxy (oauth2-proxy, Authelia, ...). |
| `LOOPFORGE_ROLE` | `all` (default), `api`, or `worker` for split deployments. |
| `LOOPFORGE_DEMO_MODE` | Seed the scripted demo executor and SOP. |
| `LOOPFORGE_NOTIFY_WEBHOOK` | Group-chat robot webhook for waiting/failed alerts. |
| `LOOPFORGE_PUBLIC_URL` | Public base URL for notify links and approval share links. |
| `LOOPFORGE_TEMPLATE_SANDBOX` | Sandbox for the built-in SOPs: `generic-docker`, `linux-bwrap`, `macos-seatbelt` or `local`. Auto-detected when unset. |
| `LOOPFORGE_MCP_MARKET` / `LOOPFORGE_TEMPLATES` / `LOOPFORGE_WEB_PLUGINS` | Add your own MCP market entries, SOP templates and UI pages from files, directories or URLs. See [docs/extending.md](docs/extending.md). |

**MySQL:** tables are created and extended automatically on start. Also apply [`migrations/001_run_obs_generated_columns.sql`](migrations/001_run_obs_generated_columns.sql) once; telemetry filters and analytics on MySQL rely on the generated columns it adds. `002` is optional. SQLite needs nothing.

Sign-in uses local accounts (bcrypt) by default; put the platform behind an identity-aware proxy for SSO. REST and MCP clients use personal tokens (`Authorization: Bearer afp_...`) created under **接入指南 → 我的 Token** (Connect → My tokens).

## Runners: execute on your own machine

From **个人 → 执行机器** (Settings → Execution machines), copy the one-line bind command and run it on the target machine:

```bash
curl -fsSL https://loopforge.example.com/runners/install.sh | bash -s -- \
  --server loopforge.example.com --token bind-xxxx --service
```

The installer downloads the runner from your server (no Go needed), discovers installed CLIs and registers the machine. Set it as your default machine and every step of the runs you own executes there: repository checkout, sandbox, agent CLI and gates. Task state lives in the database, so sleep, crashes and network drops don't lose work; leases expire and tasks are re-queued by policy. Runners update themselves when idle.

## Security

LoopForge executes agent commands against real repositories. The `local` sandbox is a convenience, not a boundary. Seatbelt, bubblewrap and Docker are suitable for unattended runs on a trusted host but are not hostile multi-tenant isolation. The default `docker compose` setup mounts the Docker socket, which is root-equivalent on that host; run it on a machine dedicated to LoopForge. See [SECURITY.md](SECURITY.md) for deployment guidance and how to report vulnerabilities.

## Roadmap and known limitations

Planned next: the closed self-feedback loop, a chat-first entry point (state what you need in a sentence and get the result back as a reply), a leaner Studio navigation and an English UI. The full list is in [docs/roadmap.md](docs/roadmap.md).

Known limitations today:

- The Studio UI is in Chinese.
- AI features outside SOP runs (requirement clarification, the built-in assistant, run evaluation) use CodeBuddy and need a CodeBuddy API key in **个人设置**. Without one, create requirements with "直接创建" (skip analysis).
- Only one instance per database should execute runs (`LOOPFORGE_EXECUTE_RUNS=false` on read-only replicas).
- Executors: Claude Code, Codex and CodeBuddy. Trackers: GitHub. Artifacts are stored in the database.

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md) (code layout rules) and follow our [Code of Conduct](CODE_OF_CONDUCT.md). Changes to the CLI and its workflows follow [cli/CONTRIBUTING.md](cli/CONTRIBUTING.md) and [cli/AGENTS.md](cli/AGENTS.md). Questions and ideas are welcome in [GitHub Issues](https://github.com/Tencent/LoopForge/issues).

## License

MIT. Copyright (C) 2026 Tencent. See [LICENSE](LICENSE) and [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for bundled third-party software.
