# Third-Party Notices

LoopForge is licensed under the Apache License 2.0 (see [LICENSE](LICENSE)); the
CLI under `cli/` is licensed under the MIT License (see [cli/LICENSE](cli/LICENSE)).
The components below keep their original licenses.

## Bundled in this repository

| Component | Path | License | Notes |
| --- | --- | --- | --- |
| Monaco Editor | `web/monaco/` | MIT, Copyright (c) Microsoft Corporation | See `web/monaco/LICENSE` and `web/monaco/ThirdPartyNotices.txt`. |
| Superpowers | `brainstorming` and `writing-plans` skills under `cli/.codebuddy/`, `cli/.codex/`, `cli/.cursor/`, `cli/.claude/` | MIT, Copyright (c) 2025 Jesse Vincent | Vendored with compatibility adaptations. Exact paths in [cli/THIRD_PARTY_NOTICES.md](cli/THIRD_PARTY_NOTICES.md). |

## Fetched at build time (not distributed in this repository)

| Component | Path | License |
| --- | --- | --- |
| vscode-web, a web build of Code - OSS | `web/vscode-web/` via `scripts/fetch-vscode-web.sh` | MIT, Copyright (c) Microsoft Corporation |

## Dependencies

The platform's Go module dependencies are listed in [go.mod](go.mod) and are
used unmodified under their own licenses (MIT, BSD, Apache-2.0, ISC and, for
`github.com/go-sql-driver/mysql`, MPL-2.0). The CLI's dependencies are listed
in [cli/package.json](cli/package.json) and [cli/pyproject.toml](cli/pyproject.toml).
The Docker image installs the Claude Code, Codex and CodeBuddy CLIs from their
public package registries under their vendors' terms.
