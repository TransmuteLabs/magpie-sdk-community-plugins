# Headless tools and offline history extraction

The UI selection/auth-broker cut remains separate. This lane uses the
existing 1.4 host-derived tool scope and owner exposure.

1. Add declared local-owner read tools for sanitized selected-account
   information, official MCP usage, informational ordinary quota and
   controller status. Only Frame.Auth/Scope select credentials; `_magpie`
   is declared for gateway discovery but must already be removed by host.
2. Keep official MCP credential headers separate from Off-Peak:
   selected ZCode JWT plus paid business/MaaS JWT and exact Team scope.
   No token minting, free fallback or caller identity-header overrides.
3. Port the recovered `GET /api/v1/mcp/usage` aggregate envelope into a
   bounded pure projection; usage remains Aside/informational.
4. Add pure offline history readers for native JSONL and legacy snapshots,
   using the recovered schemas as evidence. Retain complete original bytes,
   unknown/native fields, large integers and external-body references.
5. Convert supported sources into a versioned source-preserving history
   bundle and restore their exact original format. Do not pretend that a
   bundle is an executable resume or a lossless cross-agent conversion.
   Do not register a SessionMigration descriptor before its adapter exists.
6. Exercise declared tools through the actual native host with selected
   account authority. A tool's usage observation must not acknowledge the
   host quota barrier; only provider.usage owns that protocol.
7. Run synthetic regressions, race/vet/format/native/platform gates on
   usbox, compare executable-source hashes and commit verified slices.

Read-only sources: recovered official MCP credentials and quota provider,
shared validation schemas, native JSONL helpers and legacy session parser.
No donor changes, live SQLite reads, SQL dependency, host secret access,
CLI authentication wrapper, live endpoint call or new dependency.
