# Qoder (`qoder`)

Standalone native Magpie SDK (`1.12`, native ABI `1`) plugin serving Qoder, Qoder CN, QwenWork, and QwenWork Enterprise without any Node/Bun or community plugin runtime dependency.

## Providers

- `qoder` — Qoder international (`qoder.com`), browser PKCE device sign-in and PAT exchange, daily `account.checkin`.
- `qoder-cn` — Qoder CN (`qoder.cn`), browser PKCE device sign-in (with device-token fallback), PAT exchange, enterprise VPC (`<instance>.vpc.qoder.com.cn`), enterprise dedicated domain (Qoder CN 2.0 and Lingma 1.0 IDE cache), daily `account.checkin`.
- `qwenwork` — QwenWork (`gateway.qwenwork.cn`), browser PKCE device sign-in (`deviceOnly`, personal/biz identity refresh, wallets usage).
- `qwenwork-enterprise` — QwenWork Enterprise flagship (custom host or `QWENWORK_ENT_DOMAIN`, fallback catalog on 503).

## Build & Test

From the workspace root on Linux (`usbox`):

```bash
python3 -I scripts/check.py qoder --race --sdk /path/to/magpie
python3 -I scripts/build.py qoder --sdk /path/to/magpie --output /tmp/qoder-dist
```
