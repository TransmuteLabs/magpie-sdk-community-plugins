# Owned ZCode agent continuation

2026-11-14. Start from CN checkpoint `3d949ff`. Only this repository is
writable. Keep the SDK 1.4 headless distribution; extend the existing
SDK 1.5 native/TypeScript bundle with an explicit `zcode-native` profile
of the built-in `zcode` target. No second plugin or credential owner.
Final verification host is the shared devzeus commit
`d07078c7926daf6f33af09c40297c941bd39da92`, tree
`ed28dfd65eca68d15e9400b56660bbbf5e7bc319`.

1. Read the exact vendor CLI 0.17.1 / desktop 3.18.2 provider-file codec,
   runtime path and launch sources at `fec975905a2abd4c07e35c40d51fa097a2258ec9`.
   Reuse public SDK AgentWiring, host file snapshots, binding revisions,
   backups, launch and inherited ZCode Library placement.
2. Add regression fixtures before implementation. Assert native manifest
   registration; connect/set/sync/disconnect of source-shaped provider
   rules; explicit model selection; preservation of foreign/manual rules,
   format and user edits; invalid input refusal and exact binding echo.
   Run the unchanged new assertions against old code on usbox.
3. Add the vendor config adapter in `agent.go` and register it only in
   `NewControlModule`/`ControlManifest`. Plans return checked file changes
   and binding changes, never perform filesystem writes. Keep runtime/MCP
   and credentials with their existing owners.
4. Exercise the packaged native bundle through ordinary host
   discovery/connect/set/sync/disconnect/run and Library in a clean HOME.
   Use a clearly identified synthetic CLI when the real client is absent.
   Preserve root ownership checks and fail closed on stale/off ownership.
5. Run scoped module tests/race, format/vet, strict TypeScript and relevant
   native integration fixtures on usbox. The user lifted the temporary
   GOMAXPROCS=2 / Go -p=2 cap; reuse the existing usbox caches.
   Use isolated HOME/XDG, offline pinned caches and loopback namespace.
   No broad host suite; the leader owns union verification.
6. Record exact host/source/artifact identities, observed behavior and
   remaining gaps in source-lock, provenance and evidence. Commit locally
   using Lore; no push.

This is a bounded extraction, not a wholesale copy of the built-in
controller. Do not change existing financial, auth, SQLite or resource
guards to make agent fixtures pass. Automatic live DB/WAL capture, actual
client startup, community gift/claim/dual-trial parity and rendered GUI
acceptance require separate evidence.
