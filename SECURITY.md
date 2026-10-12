# Security Policy

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security problems.

Report vulnerabilities privately through
[GitHub Security Advisories](https://github.com/Tencent/LoopForge/security/advisories/new).
Include the affected version or commit, a description of the impact, and steps
to reproduce. We aim to acknowledge reports within 3 business days and to ship
a fix or mitigation for confirmed high-severity issues within 30 days.

## Supported versions

Security fixes land on the default branch and in the latest tagged release.

## Deployment guidance

The LoopForge platform runs coding agents that execute commands against real repositories.
Treat it as privileged infrastructure:

- Keep `LOOPFORGE_AUTH_REQUIRED=true` (the default) and put TLS in front of the
  server. Do not expose the development HTTP listener directly to the Internet.
- The `local` sandbox driver is **not** an isolation boundary. Use `linux-bwrap`,
  `macos-seatbelt`, or Docker for unattended runs, and never mix untrusted
  tenants on one host: none of the bundled drivers is a hostile multi-tenant
  boundary.
- Only enable `LOOPFORGE_TRUST_IDENTITY_HEADERS` behind a reverse proxy that
  strips and re-injects the identity header, and restrict
  `LOOPFORGE_TRUSTED_PROXY_CIDRS` to that proxy.
- Model API keys, Git tokens and personal MCP tokens are stored per user in the
  database. Protect database backups accordingly and prefer short-lived,
  least-privilege tokens (for example fine-grained GitHub PATs scoped to the
  repositories a project uses).

## LoopForge CLI

Installation permissions and deployment responsibilities for the CLI and its
agent workflows are described in [cli/SECURITY.md](cli/SECURITY.md). Report
vulnerabilities in either part through the same private advisory channel.
