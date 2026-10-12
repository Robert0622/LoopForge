# Architecture

LoopForge keeps four contracts independent:

1. `WorkflowDefinition` owns DAG control flow, retry, verification, controlled rework, and the single Sandbox Profile used by each Run.
2. `AgentDefinition` owns instructions, rules, skills, MCP references, lifecycle hooks, its self-correction loop, human intervention points, artifact contracts, and role-level verification.
3. `ExecutorDefinition` adapts that role to Claude Code, Codex, CodeBuddy, or another CLI.
4. `SandboxProfile` selects explicit local-unsafe, macOS Seatbelt, Linux bubblewrap, Docker, and later gVisor/Kata/Firecracker-backed remote implementations.
5. `ArtifactContract` declares immutable node outputs independently from CLI text output.

The engine discovers all dependency-ready nodes concurrently, then serializes their execution through the Run's single sandbox/workspace so parallel DAG branches cannot write the same checkout concurrently. A verification failure can reset the configured `rejectTo` node and all of its descendants without introducing a cycle into the saved DAG. Manual gates expose approve and reject actions through the API without occupying a process-concurrency slot.

## Runtime boundary

`internal/runtime.Sandbox` is currently the process/container boundary:

```go
type Sandbox interface {
    ID() string
    Workspace() string
    Exec(context.Context, CommandSpec, EventSink) (string, error)
    Close(context.Context) error
}
```

The current implementation includes an explicitly unsafe local driver, lightweight macOS Seatbelt and Linux bubblewrap process sandboxes, and persistent Docker-container sandboxes. Provider requirements explicitly declare Claude/Codex state mounts and runtime features; generic sandbox code does not infer a provider from `argv[0]`. CodeBuddy API-key runs never mount host `~/.codebuddy` and receive a Run-private HOME. Process concurrency is acquired for each `Exec`, while the sandbox/container and workspace remain attached through retry and approval waits. Docker additionally applies its configured CPU and memory limits. Before a remote provider is added, this interface must gain provider-neutral file upload/download methods; a cloud sandbox cannot expose a host-local `Workspace()` path.

`ExecutorRunner` materializes one global asset registry and an Agent-scoped binding view:

```text
.loopforge/
  skills/<skill-id>/SKILL.md       # managed entities
  rules/<rule-id>.md
  mcp/<server-id>.json
  hooks/codebuddy/hook.py
  bindings/<agent-id>/             # selected-asset link farms
  config/<agent-id>/               # compiled provider configuration
    hooks/<hook-id>.sh|.py         # role hook scripts for this invocation
  <agent-id>/                      # symlinks only
    skills -> ../bindings/<agent-id>/skills
    rules  -> ../bindings/<agent-id>/rules
    mcp    -> ../bindings/<agent-id>/mcp
    agent-system.md -> ../config/<agent-id>/agent-system.md
  runtime/<invocation-id>/         # execution state only
    run-context.json
    observability/hooks.ndjson
  environment/<id>/                # install.sh output for process-sandbox toolchains
  docker/<id>/                     # temporary Dockerfile build context
```

Seatbelt and bubblewrap load the selected environment asset from the database (one row, including `installScript`) and run that script once inside the sandbox with `PREFIX` / `LOOPFORGE_ENVIRONMENT_ROOT` pointing at `.loopforge/environment/<id>/`. Studio edits write the same payload back to the database; the next sandbox init uses the stored script. A workspace may still drop a single `loopforge-environment/` directory as a prebuilt prefix. Docker sandboxes ignore this overlay: they load the selected Dockerfile asset from the database (`content` and optional `files`) and build a temporary image during sandbox initialization. There is no repository Dockerfile directory and no registry image name on the sandbox profile.

Skill, Rule, and MCP entities have one managed copy. Role hooks live on `AgentDefinition` and are compiled only for the invocation of a node using that Agent: Claude Code and CodeBuddy receive an invocation-specific settings file, while Codex receives equivalent inline `hooks.*` configuration. Each Agent directory is a symbolic-link view of only the assets referenced by its `AgentDefinition`; an invocation owns logs and run context, never asset selection. The runner never writes `AGENTS.md`, `CLAUDE.md`, `CODEBUDDY.md`, or provider skill directories into the repository. Provider implementations own flags, materialization, protocol parsing, state requirements, and credential-binding requests. CodeBuddy receives generated settings with `--setting-sources none`; repository-local `.mcp.json` is not discovered.

Seatbelt denies reads of repository Agent CLI paths. Bubblewrap and Docker replace them with empty read-only mounts. The denylist covers root instruction files and `.agents`, `.codex`, `.claude`, `.codebuddy`, and `.mcp.json`, so repository configuration cannot override LoopForge-selected assets. Claude and Codex can explicitly request their host CLI state. CodeBuddy instead requests a private HOME and controlled project credential binding; its API key exists only in the target child environment and is excluded from process telemetry.

Each Workflow Run owns one workspace shared by its nodes, retries, and gates. A run binds one or more repositories, all treated as equals: every repository is checked out into its own first-level sub-directory (`workspace/<repo>/`) and gets the same work branch, its own integration merge, and its own baseline PR. SOP deliverables such as `fix-plan.md` stay at the run workspace root so they never enter a git checkout. Verification gates must declare `executionScope` as `run` or `repositories`; the engine does not infer a working directory from a `git` command prefix. Each repository `.git` directory is inside the allowed workspace and supports normal Git operations without exposing the main checkout or another Run. Hook telemetry remains under `.loopforge/runtime/<invocation-id>` so the host process can observe it without widening the sandbox boundary.

MCP secret placeholders use the exact form `${secret:ENV_NAME}` and are resolved from the control-plane process environment only when a run is materialized. They are never stored as plaintext in the platform state file.

## Execution path

1. The saved visual graph is validated as a DAG. Script nodes run either shell or Python and can use the same failure/reject-to-node path as Agent nodes.
2. The complete SOP definition is frozen into the Run. Later collaborative edits only affect new runs.
3. The first executable node lazily acquires the Run workspace and the Workflow's selected `SandboxProfile`.
4. Dependency-ready nodes queue on that shared Run environment; retries and approval resumes reuse it.
5. Agent references are resolved into the stable `.loopforge/<agent-id>` symbolic-link view. `.loopforge/runtime/<invocation-id>` stores execution state only; the sandbox hides repository Agent CLI configuration and the executor adapter explicitly injects the Agent view.
6. The selected executor adapter invokes Claude Code, Codex CLI, CodeBuddy CLI, or the deterministic mock.
7. Every Agent node automatically receives the invocation-scoped `submit_gate_result` MCP tool and a terminal instruction; this is not user-configurable. The Agent must submit `pass`, `rework`, `fail`, or `human` with a concrete `reason`; `human` additionally requires `actionRequired` describing exactly what a person must confirm. Missing results cause one resume of the same CLI session. `rework` routes to the node's configured upstream target, `fail` terminates the node, and `human` pauses for approval. Objective `command` gates still run before a manual gate can pause the node.
8. Declared artifacts are validated, hashed, snapshotted, and registered before human approval. Direct downstream nodes receive the newest successful upstream attempt under `.loopforge/inputs/<node>/<artifact>.<ext>`.
9. A failed verification either terminates the run or resets the configured target and its descendants. Manual gates pause until approve/reject arrives through the API.

Successful runs still delete the workspace. If `WorkflowSnapshot.preview` declares a static directory, a Preview Finalizer archives that directory into the database (`preview_blobs`) and holds cleanup until the bundle is ready or finalization times out. Workspaces without a frontend project skip preview (`status=skipped`) instead of failing the run or inventing a static page. Studio then opens a ready bundle through a short-lived session at `/preview/{token}/` on the Studio host.

## Agent-internal orchestration

SOP and Agent control flow deliberately operate at different levels. An SOP
connects independently owned work units and describes cross-role dependencies.
An Agent definition describes how one reusable role reaches its own definition
of done:

- `loop` retries failed Agent gates in the same sandbox so the next iteration
  can inspect and repair the previous iteration's work;
- `verification` defines reusable objective command and manual gates. The
  engine separately injects the required structured terminal protocol into
  every Agent invocation;
- `artifactOutputs` defines the role's normal product contract, which an SOP
  node may refine by artifact name for a particular scenario;
- `interventions` declares human checkpoints. The local engine currently
  enforces `after_execution` and `before_publish`; the other declared stages
  are preserved for remote-worker checkpoint scheduling.

This keeps a "code review Agent" portable across SOPs without copying its
review report, quality gate, loop policy, and required human decision into
every workflow node.

Agent prompts can consume `{{input}}`, direct dependency output through `{{upstream}}`, a specific result through `{{node.<id>.output}}`, upstream artifact manifests through `{{artifacts}}`, a specific artifact through `{{artifact.<node>.<name>}}`, and the latest reject reason through `{{feedback}}`. Rejecting a gate resets the target and descendants, increments the target rework count, and injects the reason into the next target attempt. Historical artifact attempts remain available for audit.

## Worker and cloud sandbox boundary

A worker is a data-plane process that leases executable node jobs from the control plane. “Single worker” means the API process itself currently schedules and executes every node, and its concurrency semaphore only protects that one process. It cannot provide cross-host capacity, leases, heartbeats, crash recovery, or capability-aware routing.

The production split is:

1. The control plane transactionally writes node state plus an outbox record to PostgreSQL.
2. A dispatcher publishes an idempotent job to a durable queue.
3. A registered worker leases the job, heartbeats it, and selects a compatible sandbox provider and runtime image.
4. Artifact inputs are fetched from S3-compatible object storage using short-lived URLs and uploaded into the remote sandbox.
5. The worker streams normalized telemetry and renews its lease while Claude Code, Codex, or CodeBuddy runs.
6. Verification attestations and artifact outputs are uploaded before the worker atomically completes the lease.
7. Lease expiry makes a job recoverable; idempotency keys prevent two completions from corrupting run state.

For an initial production deployment, Kubernetes Jobs with gVisor and per-sandbox NetworkPolicy are the practical default. Kata Containers or Firecracker should be used when tenants or workloads are mutually untrusted. Plain local processes and ordinary Docker containers remain development modes.

## Production replacement points

- Replace the single-schema SQLite/MySQL store with per-aggregate Postgres repositories.
- Replace the in-process scheduler with Temporal while retaining the domain state machine.
- Implement OpenSandbox and Kubernetes Agent Sandbox drivers behind `Sandbox`.
- Send run events to NATS or Redis Streams and archive them in object storage.
- Resolve secrets through a short-lived credential broker; never place credentials in runtime images.
