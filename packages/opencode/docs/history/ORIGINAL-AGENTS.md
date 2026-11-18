# Our Magpie extensions

These modules are developed alongside the SDK in `../magpie`. Keep vendor
behavior here and generic host contracts in Magpie.

- Build and test only through `usbox`. `usbox-adm` is administration only.
- Use the SDK's declared Go toolchain, isolated HOME/XDG and pinned caches.
- Never read or change live credentials, accounts, sessions or app settings.
- Never kill target-warden. End only processes started by your own fixture.
- No new third-party dependencies, remote publication, pushes or live vendor
  calls without an explicit instruction.
- Use public SDK imports only. Do not import Magpie's internal packages.
- Keep module directories independent. Shared helpers require demonstrated
  duplication and a documented contract.
- Document minimum SDK contracts and each missing host adapter. Unknown
  protocol semantics fail explicitly; do not silently drop or invent them.
- Before refactoring donor code, write a short file-level extraction plan and
  preserve its behavior with relevant fixtures.
- Keep recovered artifacts and cassettes read-only. They may contain private
  data; commit only reviewed synthetic or sanitized test fixtures.
- Do not begin ZCode migration until the user explicitly releases its hold.
- Local commits use intent-first Lore messages with relevant Constraint,
  Rejected, Confidence, Scope-risk, Tested and Not-tested trailers.

Each module separates domain code, `cmd/` entry points, optional `web/`
assets, package metadata and fixtures. Authors must not implement transport
plumbing already owned by the public SDK.
