# MiniMax

Standalone native MiniMax Code provider for China (`minimax-code`) and Global
(`minimax-code-global`). Includes device authorization, chat, live model
discovery, account usage, daily check-in, search, tools and image/video operations.
Uses the public Magpie SDK; no community plugin, Bun or Node runtime is needed.

From the workspace root on regular usbox:

```sh
python3 -I scripts/check.py minimax --sdk /path/to/magpie
python3 -I scripts/build.py minimax --output /tmp/plugins --sdk /path/to/magpie
```

The root builder creates `/tmp/plugins/minimax`. The direct package builder uses
the exact output directory: `python3 -I packages/minimax/scripts/build.py
--output /tmp/minimax --sdk /path/to/magpie`. Output must be empty or absent.
Install that directory through Magpie's SDK plugin installer. In
**Providers → Add → Subscriptions**, choose **MiniMax Code** for China or
**MiniMax Code (Global)**, then approve the displayed device code. Providers
also offers reauthorization and sign-out for stored accounts. Credentials
and refresh transactions remain host-owned. See the [account contract](docs/CONTRACT.md).

Version `0.1.1` requires SDK API 1.12 with `provider-signin` and native ABI 1.
Complete supported chat refusals project original
facts through shared request policy before acceptance/output. The host owns
bounded waiting and eligible-account fallback; see the
[supported projection and remaining gaps](docs/REQUEST-POLICY.md).
Exact web-search and connector-catalog reads use named host recovery permits,
including catalog reads after paid acceptance. Generic connector calls retain
their existing single-dispatch fence. See the
[named-read contract and verification boundaries](docs/READ-POLICY.md).
Accepted v4 verification is historical; the API 1.12 source delta has separate
evidence and no new native acceptance.
The bounded ordinary-host Chat and Messages checks use the actual handlers
through an exec harness. The earlier Chat 409 was traced to the fixture
ignoring upstream streaming; see the [causal completion regression](docs/CHAT-COMPLETION.md)
and [exact facts projection limits](docs/FACTS-PROJECTION.md).

See [runtime boundaries](docs/CONTRACT.md), [source provenance](docs/SOURCES.md)
and [verification](docs/VERIFICATION.md) for supported behavior and test limits.
