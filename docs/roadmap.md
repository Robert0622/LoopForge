# Roadmap

[简体中文](roadmap.zh-CN.md)

LoopForge aims to be a fully automated loop for requirements: a requirement goes in, a mergeable pull request comes out, and people only step in where the team decides they must. This page lists what is planned. It is a direction, not a schedule; priorities shift with feedback, and issues and pull requests are welcome.

## Available today

- **The automated loop.** Scheduled issue sync, clarification scoring, scheduled dispatch, verdicts and command gates, reject-and-rework with downstream reset, integration branch and pull request delivery, optional human approval nodes.
- **Per-run isolation.** One container per run, out of the box with `docker compose`, on bound runner machines, or on any host with Docker.
- **Observability and evaluation.** Live telemetry per run, process metrics across runs, and an evaluation of every run whose recommendations can be applied to SOPs and roles in one click.
- **Expert experience as SOPs.** Built-in backend, frontend and product delivery SOPs, reusable roles, declared artifacts with full attempt history, knowledge base search over MCP.

## Planned

### Self-feedback loop

Evaluation already proposes concrete changes to SOP nodes, gates and role instructions, and a person applies them. Next:

- Accumulate recommendations across runs and surface the ones that recur, instead of one run at a time.
- Apply low-risk changes automatically as a new SOP or role revision, with the evidence attached and one-click rollback.
- Compare metrics before and after a change (first-pass rate, rework rate, lead time) so the loop can keep or revert it on its own.

### Chat-first entry

The dashboard already has a chat entry: clarify in one sentence, create and dispatch, and watch running or waiting workflows in the same view. Next: more chat tools, and results returned as replies (pull request, preview or report). The full studio stays available for setting up SOPs and roles.

### Faster first run

- A short "quick fix" SOP (implement → test → open PR) alongside the full delivery templates.
- A leaner Studio navigation that shows the daily views first and moves configuration out of the way.
- An English UI.

### Isolation without the Docker socket

The default deployment already starts one container per run, through the host's Docker socket, which is root-equivalent on that host. Planned: a rootless option (for example rootless Docker or a dedicated sandbox runtime) so teams can keep per-run containers without handing the platform the host.

### Fewer dependencies

- AI features outside SOP runs (requirement clarification, the assistant, run evaluation) currently use CodeBuddy. Planned: let them use any configured executor or model provider.
- More executors, including ACP-compatible agents.
- More trackers: GitLab, Gitea and Jira, through the provider interface described in [extending.md](extending.md#issue-trackers).
- An object storage backend for artifacts.

## Contributing to the roadmap

Open an issue to discuss an item before starting large work. Smaller improvements can go straight to a pull request; see [CONTRIBUTING.md](../CONTRIBUTING.md).
