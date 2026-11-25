# WorkBuddy Native SDK Plugin Sources

- **Package**: `packages/workbuddy`
- **Module ID**: `workbuddy`
- **Providers**: `workbuddy` (China, `copilot.tencent.com`), `workbuddy-ai` (International, `www.workbuddy.ai`)
- **Public SDK Contract**: `1.12` (native ABI `1`), `github.com/yetone/magpie/pkg/extension` only

## Donor

- **Repository**: `https://github.com/magpie-community/plugins` (local sibling checkout, read-only donor)
- **Donor Package**: `packages/workbuddy` (`@magpie-community/opencode-workbuddy-auth` `0.2.37`)
- **Donor Commit**: `964674642899f4827ed3d38e4a91fdc344e6448c` (C33 Flash quota refusal alias parity on top of `origin/main` `e967ba4d946071c6c27a51cd5168b12ce832da5c`)
- **Donor Package Tree**: `4683e868a8a408506658cc94ff2664abbf0db0fa`
- **Donor License**: MIT (`docs/DONOR-LICENSE.txt`)
- **Checked**: `2026-11-24`

## Ported Behavior Summary

- Dual site configuration: WorkBuddy China (`copilot.tencent.com`, Chat + Anthropic Messages for `^claude-*`, daily check-in) and WorkBuddy AI (`www.workbuddy.ai`, Chat, `noImages: /^glm-/`, growthEvent + `/v2/report` activity telemetry).
- Cloud-hosted enterprise endpoint routing (`*.sso.copilot.tencent.com`, `*.copilot.qq.com`, `*.sso.codebuddy.cn`, excluding Tencent iOA hosts).
- Four sign-in methods per provider: browser OAuth state/token/account polling with enterprise resolution and WeCom version `5.9.3`, WorkBuddy desktop `.info` reading (with encrypted 5.6+ detection), API key (`ck_`/`pt_` remote config gating), and saved sign-in file import (`.info`, auth.json, OAuth tokens) with cross-site domain guard.
- Singleflight rotating token refresh (`KEEP_RENEWED_MS = 60s`), proactive refresh (`EARLY_MS = 60s`, host keepalive `LEAD_MS = 5m`), and one-shot 401 re-issuance with `account.replace` persistence and `X-Magpie-Sign-In` signaling.
- Full Chat Completions adaptation: `FLAGGED` rewording, `hide_dsh` path-aware stripping, `developer` -> `system`, `max_completion_tokens` -> `max_tokens`, `reasoning_content`/`reasoning_details` -> `reasoning`, `kimi-k3.1-pro*` call-only single-space fill, default system prompt, `normalizeToolChoice`, full `fitTools` schema/shape/enum/oneOf/objectRoot normalization + streaming/JSON tool-call name restoration, `withDefaultEffort` (respecting `SMALL_BUDGET = 1024`), gzipped chat bodies, `EDGE_READS = 2 MiB` ECONNRESET billing 401 probe, 400 retry cascade (`commaPaused` -> `systemAsUser` -> `replyAfterTools`) with bounded caches, SSE `content_filter` `FILTER_SAYS` suppression, and `explained` refusal/restriction/exhaustion hints.
- Native Anthropic Messages route (`/v1/messages`, `/v1/messages/count_tokens`) for China Claude models with unsigned thinking strip and 11159 signed-thinking retry.
- Billing meter usage (`Credits` / `Extra credits` / `Total credits` split, prepaid balance bags, enterprise meter fallback, paginated paid+free packages, cycle reset/span, `runsOut`, `listedExtra`, `spareFree` `notModels`).
- C33 free-model 24h token window (`100M` on WorkBuddy AI, `~200M` on China) reading both `usage.jsonl` reverse-tail and `routing/` numbered/gzipped traces (even when the ledger is absent/empty), surfacing active 429 refusals across shared aliases (`deepseek-v4.1-flash`, `deepseek-v4.1-flash-sg`) as a 100%-used blocking window with vendor reset and no invented token count.

## Verification Limits

- No live vendor network calls, real accounts, or installed desktop WorkBuddy app are touched during testing.
- All HTTP endpoints, filesystem ledgers, desktop `.info` fixtures, and routing traces are tested against isolated temporary directories and synthetic loopback stubs.
