# Roadmap

[简体中文](roadmap.zh-CN.md)

LoopForge aims to be a fully automated loop for requirements: a requirement goes in, a mergeable pull request comes out, and people only step in where the team decides they must. This page lists what is planned. It is a direction, not a schedule; priorities shift with feedback, and issues and pull requests are welcome.

## Available today

- **The automated loop.** Scheduled issue sync, clarification scoring, scheduled dispatch, verdicts and command gates, reject-and-rework with downstream reset, integration branch and pull request delivery, optional human approval nodes.
- **Observability and evaluation.** Live telemetry per run, process metrics across runs, and an evaluation of every run whose recommendations can be applied to SOPs and roles in one click.
- **Expert experience as SOPs.** Built-in backend, frontend and product delivery SOPs, reusable roles, declared artifacts with full attempt history, knowledge base search over MCP.

## Planned

### Self-feedback loop

Evaluation already proposes concrete changes to SOP nodes, gates and role instructions, and a person applies them. Next:

- Accumulate recommendations across runs and surface the ones that recur, instead of one run at a time.
- Apply low-risk changes automatically as a new SOP or role revision, with the evidence attached and one-click rollback.
- Compare metrics before and after a change (first-pass rate, rework rate, lead time) so the loop can keep or revert it on its own.

### Chat-first entry

Describe what you need in one sentence, from the web UI or a chat tool, and get the result back as a reply. LoopForge turns the sentence into a requirement, picks the SOP, runs it, and answers with the pull request, preview or report. The full studio stays available for setting up SOPs and roles.

### Faster first run

- A short "quick fix" SOP (implement → test → open PR) alongside the full delivery templates.
- A leaner Studio navigation that shows the daily views first and moves configuration out of the way.
- An English UI.

### Isolation out of the box

Today the `docker compose` setup runs every run as a process inside the LoopForge container; one container per run needs a host or runner machine with Docker. Planned: per-run containers in the default deployment as well, without giving the platform container control of the host.

### Fewer dependencies

- AI features outside SOP runs (requirement clarification, the assistant, run evaluation) currently use CodeBuddy. Planned: let them use any configured executor or model provider.
- More executors, including ACP-compatible agents.
- More trackers: GitLab, Gitea and Jira, through the provider interface described in [extending.md](extending.md#issue-trackers).
- An object storage backend for artifacts.

## Contributing to the roadmap

Open an issue to discuss an item before starting large work. Smaller improvements can go straight to a pull request; see [CONTRIBUTING.md](../CONTRIBUTING.md).
