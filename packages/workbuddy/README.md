# WorkBuddy Native SDK Plugin

Standalone native Magpie SDK plugin providing Tencent **WorkBuddy** (`workbuddy`, China `copilot.tencent.com`) and **WorkBuddy AI** (`workbuddy-ai`, International `www.workbuddy.ai`).

## Capabilities

- **Providers**:
  - `workbuddy` (China): Chat Completions (`/v2/chat/completions`), native Anthropic Messages for Claude models (`/v1/messages`), daily check-in (`account.checkin`), and switches `free_only` / `hide_dsh`.
  - `workbuddy-ai` (International): Chat Completions (`/v2/chat/completions`), growth/activity telemetry (`/v2/report`), and switches `ai_free_only` / `ai_hide_dsh`.
- **Authentication**: Browser OAuth polling (including enterprise/education account resolution and WeCom-compatible app version), WorkBuddy desktop `.info` session reading, API keys, saved sign-in import, and singleflight rotating token refresh.
- **Usage & Quotas**: Credit summary split (`Credits`, `Extra credits`, `Total credits`), prepaid balance windows, enterprise usage fallback, free-model credit exclusion (`notModels`), and C33-compliant 24h free-token window tracking across `usage.jsonl` and `routing/` traces for `deepseek-v4.1-flash` and `deepseek-v4.1-flash-sg`.

## Build & Test

Run via the workspace scripts on the build host (`usbox`) with isolated HOME/XDG and offline Go caches:

```bash
python3 -I scripts/check.py workbuddy --race --sdk /path/to/magpie
python3 -I packages/workbuddy/scripts/build.py --output /tmp/workbuddy-dist --sdk /path/to/magpie
```

## Provenance & Notes

- See `docs/SOURCES.md` and `docs/source-lock.json` for exact donor versions and file hashes.
- See `docs/REFUSED.md` for details on WorkBuddy `11128` channel prompt refusals.
