# Grasp UX parity plan (LoopForge)

Goal: lower first-run cognitive cost by borrowing the parts of
[cocofhu/grasp](https://github.com/cocofhu/grasp) that help users *start*,
*configure CLIs*, and *act without babysitting the web UI*.

Keep LoopForge's identity (issue-driven versioned SOPs). Do not copy Grasp's
FSM-canvas philosophy or Live page variants in this pass.

## Status

| # | Item | Status |
|---|---|---|
| 1 | First visitor creates the admin in the browser; open self-registration (admin toggle) | done |
| 2 | Tour ends on 个人设置 at the tab that still needs a key; optional credential wizard from 接入指南 | done |
| 3 | Built-in short "quick fix" SOP as a default non-demo path | done |
| 4 | Humanized run errors with next-step buttons | done |
| 5 | Approvals inbox as cards | done |
| 6 | Outbound IM notify (group-chat webhook; plain-text adapter fallback) | done |
| 7 | One-time approval share link (no login) | done |
| 8 | README: first-run account + four-step path | done |
| 9 | `./start.sh -y`: install missing Docker/Compose/curl/git, free-port fallback, build the checkout (release tags pull), Go/npm mirror fallback | done |
| 10 | Environment doctor (`/api/doctor` + 接入指南 card with fix buttons) | done |
| 11 | `loopforge --self-test` / `./start.sh doctor`: demo run + share-link approval + artifact, end to end | done |
| 12 | Sandbox image prewarm on first start (`LOOPFORGE_PREWARM_SANDBOX`, `./start.sh warm`) | done |
| 13 | `./start.sh reset` (fresh data) and compose healthcheck | done |

Later (not this pass unless time left): Cursor/OpenCode executors, project
credential aliases, clarify `page.html`, failure/rollback edges on canvas,
inbound chat bots.

## Design notes

- No built-in password: without `LOOPFORGE_ADMIN_PASSWORD` the first visitor
  creates the admin, so a fresh instance never runs on a known login.
- The tour ends on 个人设置; the credential wizard no longer pops up by
  itself and stays reachable from 接入指南 and the environment doctor.
- Short SOP: script/mock-friendly when demo; otherwise uses the user's
  configured executor.
- IM notify: group-chat robot webhook via `LOOPFORGE_NOTIFY_WEBHOOK`.
- Share link: HMAC token signed with session secret, TTL 24h, approve only
  (reject still requires login).
