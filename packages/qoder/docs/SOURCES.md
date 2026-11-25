# Qoder Package Sources

This standalone native Magpie SDK package is ported from the community OpenCode
plugin donor without any runtime dependency on `@magpie-community/opencode-qoder-auth`:

- **Donor repository:** `https://github.com/magpie-community/plugins`
- **Pinned commit:** `e967ba4d946071c6c27a51cd5168b12ce832da5c` (version `0.2.73`, tree `45397b48d89ea7f1972158897dc9732a7adab536`)
- **Upstream protocol attribution:** CLIProxyAPI Qoder protocol (`https://github.com/ufec/CLIProxyAPI`, MIT); see `DONOR-LICENSE.txt`.
- **Checked date:** `2026-11-24`

## Covered Donor Behavior (up to 0.2.73)

- Four providers: `qoder` (`qoder.com`), `qoder-cn` (`qoder.cn`, enterprise VPC, enterprise dedicated domains including Lingma 1.0 IDE cache), `qwenwork` (`gateway.qwenwork.cn`), and `qwenwork-enterprise` (enterprise flagship domain or `QWENWORK_ENT_DOMAIN`).
- COSY v1 envelope signing (RSA-1024 PKCS#1 v1.5 wrapped per-request AES-128-CBC key + MD5 signature) and shuffled base64 + thirds-swap body codec.
- Single-use rotating refresh serialization (`spent` cache, `relogin` 5-minute guard, PAT re-exchange fallback, QwenWork `biz` team vs `c` personal refresh target, Lingma 1.0 cache reload).
- Model catalog discovery with `freeView` one-shot renewal, `price_factor` / promotion rate extraction, context window sizing (`windowFor`), and `outputCap` / `reclamp` (`lmodel` 131072 and learned `[1, N]` ceilings).
- Request adaptation: Claude Code identity rewrite to Claude Agent SDK (`0.2.72`), DeepSeek Harness opener comma rewrite (`0.2.73`), Kimi call-only `" "` content (`0.2.69`), deterministic tool-call ID synthesis and result pairing, tool-returned images moved to user turns, `cache_control: ephemeral` + `contents` mirroring, `typedUnions` untyped `oneOf` object typing, `business.model_key` billing attribution (`0.2.61`), and per-prompt task tracking (`TASKS` LRU).
- Streaming response parser: `<tool_call>` JSON/XML, bare Anthropic/Qwen XML calls for declared tools, bare JSON `{"tool_calls": [...]}` lines (`0.2.68`), code-fence quoting guard, dual-ID native tool calls at one index (`0.2.65`), nameless native tool call retry/drop (`#579`), and post-finish usage frames.
- Queue handling (`10605`): status polling, streaming SSE keepalives up to 60 minutes, 5-minute stuck empty-queue termination (`0.2.71`), 120-second silent-stream 504 guard (`0.2.70`), immediate 503 on `X-Magpie-Fallback: 1`, and `off` model + exhausted credits 429 (`0.2.64`).
- Usage reporting (`qoder`, `enterprise`, `Total credits` split with plan `resetsAt`, `spareFree` `NotModels`, QwenWork `account-context` + `wallets`) and daily `account.checkin` (`runtime-info` discovery incl. Windows single-file cache, `CLAIM_BENEFIT` listing/claim, `nextOpens` `until` boundary).
