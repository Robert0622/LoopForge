# Grasp UX parity plan (LoopForge)

Goal: lower first-run cognitive cost by borrowing the parts of
[cocofhu/grasp](https://github.com/cocofhu/grasp) that help users *start*,
*configure CLIs*, and *act without babysitting the web UI*.

Keep LoopForge's identity (issue-driven versioned SOPs). Do not copy Grasp's
FSM-canvas philosophy or Live page variants in this pass.

## Status

| # | Item | Status |
|---|---|---|
| 1 | Fixed demo login `admin` / `demo1234` when demo mode and no password set | done |
| 2 | First-run credential wizard (pick CLI → apply links → save → verify) | done |
| 3 | Built-in short "quick fix" SOP as a default non-demo path | done |
| 4 | Humanized run errors with next-step buttons | done |
| 5 | Approvals inbox as cards | done |
| 6 | Outbound IM notify (group-chat webhook; plain-text adapter fallback) | done |
| 7 | One-time approval share link (no login) | done |
| 8 | README: fixed demo creds + four-step path | done |
| 9 | `./start.sh -y`: install missing Docker/Compose/curl/git, free-port fallback, build the checkout (release tags pull), Go/npm mirror fallback | done |
| 10 | Environment doctor (`/api/doctor` + 接入指南 card with fix buttons) | done |
| 11 | `loopforge --self-test` / `./start.sh doctor`: demo run + share-link approval + artifact, end to end | done |
| 12 | Sandbox image prewarm on first start (`LOOPFORGE_PREWARM_SANDBOX`, `./start.sh warm`) | done |
| 13 | `./start.sh reset` (fresh data) and compose healthcheck | done |

Later (not this pass unless time left): Cursor/OpenCode executors, project
credential aliases, clarify `page.html`, failure/rollback edges on canvas,
inbound chat bots.

## Design notes

- Demo password only when `LOOPFORGE_DEMO_MODE=true` and
  `LOOPFORGE_ADMIN_PASSWORD` is empty. Production stays random-once.
- Wizard is a modal/overlay on first login when the user has no model key;
  skippable; replay from 接入指南.
- Short SOP: script/mock-friendly when demo; otherwise uses the user's
  configured executor.
- IM notify: group-chat robot webhook via `LOOPFORGE_NOTIFY_WEBHOOK`.
- Share link: HMAC token signed with session secret, TTL 24h, approve only
  (reject still requires login).
