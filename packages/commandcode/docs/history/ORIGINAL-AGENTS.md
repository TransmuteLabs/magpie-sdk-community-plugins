# Command Code SDK module

Keep vendor protocol, authentication, plans, models, quotas, history and UI
inside this module. Use public SDK packages and declared host contracts.

- Build and test only on `usbox`; `usbox-adm` is administration only.
- Use temporary HOME/XDG and pinned existing offline Go caches.
- Do not read or mutate live credentials, accounts, sessions or installed apps.
- No additional runtime plugin dependency. A clean installation contains
  this package alone, including its native controller and optional TS UI.
- No new third-party dependency or version without an explicit request.
- Do not push or publish. Local commits use the intent-first Lore protocol.
- Keep donor checkouts read-only and record their exact source revision.
- Write a bounded extraction plan and behavioral regressions before moving
  donor code. Preserve supported semantics or return a precise refusal.
- Separate actual host/client evidence from fixtures and live vendor tests.
- Unknown quota/plan/auth state is unknown, not an empty allowance or a
  fabricated successful operation. Preserve original upstream statuses.
- Never kill target-warden. Every fixture process must be canceled and joined.

Use `cmd/`, protocol/domain packages, `web/` and `testdata/` as needed.
Do not add an author-specific transport shim already provided by the SDK.
