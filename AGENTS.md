# Magpie SDK plugin workspace

One repository, independently installable packages under packages/. Read docs/REORGANIZATION-PLAN.md before changing layout. Each package separates src (its Go module), web, tests, scripts and docs. Keep current entry instructions in README; historical plans/evidence belong in docs/history. Use root scripts for routine checks/builds. Preserve public Go module import identities and native module IDs.

Build/test/format only on regular usbox, never usbox-adm or the workstation. Use isolated HOME/XDG and existing pinned offline Go caches; no live credentials, vendor requests or installed applications. Never kill target-warden. Do not add dependencies or publish/push. No commits or merges until independent review is positive. Do not modify sibling original plugin repositories or Magpie while reorganizing. No new runtime features; preserve the audited behavior. Preserve ignored/untracked source separately before any deletion. Original repos and backups remain intact.

Use public SDK only. Each package remains self-contained at runtime. Record donors and exact versions in docs/SOURCES.md and docs/source-lock.json. Historical metadata is evidence, not current readiness. Every fixture process must stop/join. Local commits, when authorized, follow the intent-first Lore protocol.
