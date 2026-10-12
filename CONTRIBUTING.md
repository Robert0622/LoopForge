# Contributing to LoopForge

Thanks for your interest in LoopForge! Bug reports, feature ideas, docs fixes
and pull requests are all welcome.

This guide covers the platform at the repository root. For the CLI and its
agent workflows under `cli/`, follow [cli/CONTRIBUTING.md](cli/CONTRIBUTING.md)
and run the checks listed in [cli/AGENTS.md](cli/AGENTS.md) from inside `cli/`.

## Before you start

- For bugs, open an issue with the version/commit, what you did, what you
  expected, and what happened (logs and screenshots help).
- For larger changes (new executors, tracker providers, sandbox drivers, schema
  changes), open an issue first so we can agree on the approach.
- Security issues go through [SECURITY.md](SECURITY.md), not public issues.

## Development setup

Requirements: Go 1.25+, Git, and optionally Node.js 20+ (browser tests, and
the web tests use `node --check` when it is installed) and Docker (for
`make test-mysql`).

```bash
git clone https://github.com/Tencent/LoopForge.git
cd LoopForge
make run            # http://localhost:8080, SQLite under ~/.loopforge
make demo           # like make run, plus a scripted executor and demo SOP (no API key needed)
make test           # unit tests (SQLite, no external services)
make e2e            # browser tests (Playwright) against a throwaway instance
make test-mysql     # optional: gorm integration tests against a throwaway MySQL container
make web-deps       # optional: fetch vscode-web for the in-browser workspace editor
```

The frontend is plain JavaScript embedded into the Go binary (`web/`); there is
no bundler or build step. Files under `web/` are embedded at compile time,
so restart `make run` after editing them. `web/app.js` holds shared state and
helpers, and `web/app/<view>.js` holds one view each; they are classic scripts
sharing one global scope, loaded in the order listed in `web/index.html`, so
top-level code may only call functions from files loaded before it.

## Code guidelines

- Read [AGENTS.md](AGENTS.md) first. It defines where repository (`internal/repo`)
  and HTTP handler (`internal/httpapi`) code belongs; please follow it.
- Run `gofmt` and `go vet ./...`; CI rejects unformatted code.
- Add or update tests next to the code you change. Tests must not depend on
  network access or on any externally configured database.
- Every user-facing capability is exposed through `/api/*`, the OpenAPI
  contract (`docs/api/openapi.yaml`), and the MCP operation list
  (`internal/application/operations.go`). Keep the three in sync; the tests in
  `docs/` check this.
- Keep comments for constraints the code cannot express. Match the density and
  style of the surrounding code.

## Pull requests

1. Fork the repository and create a topic branch.
2. Keep each PR focused on one change; describe the motivation and how you
   verified it.
3. Make sure `make test` passes locally, and `make e2e` when you touch the UI
   or a user-facing flow. Add a browser test for new flows under `e2e/tests/`.
4. CI also runs `scripts/check-internal-refs.sh` and a secret scan; keep
   hostnames, IPs and credentials of your own environment out of the tree.
5. By submitting a contribution you agree that it is licensed under the
   [Apache License 2.0](LICENSE), or the [MIT License](cli/LICENSE) for
   changes under `cli/`.

Maintainers review PRs as time allows. Thank you for helping make LoopForge
better!

## Extending instead of forking

Custom MCP market entries, SOP templates and UI pages can be loaded at
startup without code changes; see [docs/extending.md](docs/extending.md).
If an extension point you need is missing, open an issue describing the use
case.

## Releases

Releases are tagged `v0.MINOR.PATCH` on `master`. Until 1.0, a minor release
may change configuration or APIs; the release notes list every breaking
change and how to migrate. Before tagging, `make test`, `make e2e`, the
internal-references check and the secret scan must pass on the release commit.
