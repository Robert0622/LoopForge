# LoopForge Worker Signal Standard

Independently orchestrated external workflows can report completed execution usage without creating an LoopForge Run. See [External workflow usage](external-workflow-usage.md) for project-scoped credentials, ingestion, and dashboard integration.

The contract is owned by the platform, not by any CLI or model provider. Its current version is `loopforge.observability/v1`. Any local CLI, remote worker, SDK, container runner, or future execution backend can integrate by producing a versioned `WorkerEnvelope` with one of two independent payload types:

- `observation`: aggregatable lifecycle, span, usage, tool, model, error, artifact, verification, delivery, and sandbox signals.
- `log`: cursor-addressable stdout, stderr, worker, and platform diagnostic records.

Both payloads carry the same source identity and correlation context across project, issue, task, run, workflow, node, attempt, Agent definition, executor, sandbox, provider session, trace, and span. Native provider bodies are normalized at the adapter boundary; orchestration and UI consumers never branch on CLI-native JSON.

The live contract and installed adapter capabilities are discoverable:

```text
GET /api/observability/spec
```

Remote or embedded workers use the vendor-neutral ingress endpoint:

```text
POST /api/runs/{runId}/signals
Content-Type: application/json

{"specVersion":"loopforge.observability/v1","type":"log","log":{...}}
```

# Portable execution observability

LoopForge converts provider-specific streams into one `TelemetryEvent` envelope before persistence or presentation. UI code never reads native Claude Code, Codex, or CodeBuddy event shapes.

```json
{
  "specVersion": "loopforge.observability/v1",
  "sequence": 42,
  "timestamp": "2026-08-11T10:00:00Z",
  "runId": "run-...",
  "nodeId": "implement",
  "category": "tool",
  "name": "tool.call",
  "phase": "start",
  "provider": "codebuddy",
  "model": "deepseek-v4-pro",
  "sessionId": "...",
  "toolCallId": "...",
  "message": "Write",
  "usage": null,
  "source": {"layer":"executor","adapter":"anthropic-stream-v1","provider":"codebuddy","protocol":"stream-json"},
  "context": {"projectId":"platform","issueId":"issue-1","taskId":"task-1","runId":"run-1","workflowId":"issue-fix-production","nodeId":"fix","attempt":1,"agentId":"issue-fix-engineer","executorId":"codebuddy-deepseek-flash","traceId":"run-1","spanId":"run-1:fix:1"},
  "metrics": [{"name":"tokens.input","value":1200,"unit":"count","kind":"counter"}],
  "tool": {"name":"Write","operation":"tool.call","callId":"tool-1"},
  "attributes": {}
}
```

Portable categories are `lifecycle`, `model`, `reasoning`, `output`, `tool`, `usage`, `process`, `verification`, `artifact`, `delivery`, and `sandbox`. `TokenUsage` consistently exposes input, output, cache-read, cache-write, total tokens, and provider-reported cost.

Delivery systems publish the same `observation` envelope with `category: "delivery"`. The portable attributes are `stage` (`mr`, `checkers`, `pipeline`, `acceptance`, or `closure`), `status`, `url`, `sha`, and `detail`. This lets GitHub, GitLab, Jenkins, or any CI adapter feed the delivery view without the UI understanding provider-native payloads.

The runtime protocol adapter owns native parsing. The engine owns run/node correlation, monotonic sequence numbers, aggregation, retention bounds, and summaries. The primary UI transport is SSE:

```text
GET /api/runs/{runId}/stream?after={sequence}
```

Each `run` event contains current control-plane state plus `telemetryDelta`. The browser merges coalesced event IDs and falls back to polling only when the stream disconnects. The incremental polling endpoint remains available:

```text
GET /api/runs/{runId}/telemetry?after={sequence}
```

The polling endpoint returns only newer events plus current run status, node summaries, usage, and tool-call totals. A future event-store or OpenTelemetry exporter can consume the same envelope without changing executor adapters or the web UI.

## Portable real-time logs

Logs are stored and streamed independently from observations so free-form diagnostics cannot corrupt metrics. They have their own monotonic cursor and retention bound:

```text
GET /api/runs/{runId}/logs?after={logSequence}
```

The SSE run payload contains both `telemetryDelta` and `logDelta`. The UI merges each channel by its own ID and sequence, and renders the selected node's live worker logs separately from its structured trace.

Reasoning, output, and tool-input deltas are coalesced in storage to avoid one database write per token while retaining near-real-time activity. Provider raw streams should be retained separately only when policy permits; they may contain prompts, source code, tool inputs, or secrets.

## SOP operational metrics

`GET /api/workflow-metrics` returns lightweight workflow aggregates without requiring the catalog to load and scan every telemetry body. Metrics include total, active, waiting, succeeded, failed, cancelled and reworked runs; closure, terminal-success and rework rates; average terminal duration; token, tool and artifact totals; recent run summaries; and per-node execution, success, failure, wait, rework and duration data.

The dashboard uses these definitions:

- **闭环率** = succeeded runs / all runs. It answers how much submitted work has reached the successful terminal state.
- **成功率** = succeeded / (succeeded + failed). Cancelled and still-active work is excluded from terminal reliability.
- **返工率** = runs with at least one controlled reject/rework / all runs.
- **进行中** and **待审批** are separate, so human-gate queues are not hidden inside generic active state.

Run telemetry is SSE-driven. Catalog aggregates refresh every three seconds while the catalog is open; a production event store can later push aggregate invalidations without changing the metric contract.
