# API9 required shared hunks: parent owns application

2026-11-14. The user released concurrency defaults and rich quota owner
adapters after the union merge. ZCode 0.12 is already frozen as 5f36e8a.
This document coordinates shared files; the ZCode lane does not edit
contract.go, types.go, manifest.go, composite or JS manifest/index/types/
package. Current checkout ae957e5 includes designated common base
52a8d85d7956fd027d76b0baea96ea5cf002c34a.

The parent has integrated the public fields, pointer cloning, Go and JS
manifest hooks, package files and TS declarations. This packet is retained
as the historical additive request, not a patch to reapply. The scoped
Go/Node/Bun and TS verification is recorded in API9-EVIDENCE.md.
The new cold configuration/account-stamp finding has its own minimal
parent-owned hunk and unchanged independent probe under
`testdata/api9/ACCOUNT-STAMP-PARENT-HANDOFF.md`; no shared stamp edit is
made by this lane.

## Exact shared declarations requested

In `pkg/extension/contract.go`, ProviderDescriptor:

```go
MaxConcurrency *int `json:"maxConcurrency,omitempty"`
MaxAccounts    *int `json:"maxAccounts,omitempty"`
```

FullContract version minor: 9. No new capability bit.

In `pkg/extension/types.go`, UsageWindow:

```go
Amount  *float64 `json:"amount,omitempty"`
Balance *float64 `json:"balance,omitempty"`
Display string   `json:"display,omitempty"`
Unit    string   `json:"unit,omitempty"`
Span    int64    `json:"span,omitempty"`
```

Existing `Limit` remains a float64. It is metadata in SDK projection:
neither amount/limit nor balance creates routing percentage. Existing
`Used`, `Unknown`, `Aside` and model/time guards remain authoritative.
Amount is nonnegative; Balance is signed wallet data. Missing/null pointer
values stay absent. Finite numeric magnitude is at most 2^53-1. Span is
integer seconds in 0..9223372036 (native time.Duration range in seconds).
Display/unit are bounded UTF-8 (512/64 bytes), no control characters.
Reported unit never becomes execution currency or measured cost.

In `pkg/extension/manifest.go` cloneManifest provider loop, deep-copy both
new pointer defaults. In its provider validation loop call:

```go
if err := ValidateProviderDefaults(manifest, provider); err != nil {
    return err
}
```

The new `pkg/extension/provider_defaults.go` validator is ZCode lane-owned:
non-nil fields require API9, integer values 0..1000. Omission preserves
host behavior. User account/provider overrides, including explicit zero,
win. Private-service manifests must not gain defaults belonging to another
component; retain the existing descriptor ownership rules.

Update shared JS API version/compatibility minor unions through 9.
Provider declarations add `maxConcurrency?: number | null` and
`maxAccounts?: number | null`. UsageWindow adds `amount?: number | null`,
`balance?: number | null`, `display?: string`, `unit?: string`,
`span?: number`. Export and invoke `validateProviderDefaults` from the
new lane-owned `provider-defaults.js`; it accepts `(manifest, provider)`
and throws SDKError on invalid values. Clone pointer semantics in Go;
JS immutable clones retain values independently.

Export `validateRichQuotaWindow` from the lane-owned `rich-quota.js`
if the public SDK convention calls for it. Signature:
`(manifest, window) => void`. Go equivalent
`ValidateRichQuotaWindow(manifest Manifest, window UsageWindow,
fields map[string]json.RawMessage) error`.
The lane wires it into the existing usage validators, not shared manifest.

Parent owns API9 compatibility checks in Go composite and JS composite,
version strings and public TS declarations/package exports. This lane
does not raise those files in parallel. Please preserve the new JS files
in the ordinary package files list if it is explicit.

Application packet: `testdata/api9/parent-public-fields.patch`.
It is a review packet only, not applied by this lane. Preserve the existing
`ProviderDescriptor.Options` additions. The inline JS provider declaration
also needs `maxConcurrency?: number | null; maxAccounts?: number | null;`
before its closing brace; its long shared line is deliberately not
replaced by the patch. Run gofmt after inserting the additive Go fields.
Both helper implementation files and both usage-validator hooks now exist
in the shared checkout. The Go/JS validators and provider projection tests
compiled and failed on immutable ae957 before production edits.

## Lane scope and regression plan

Before production edits, add owner regression fixtures for unmodified
base behavior. The lane owns `internal/provider/concurrency.go`,
`extension_provider.go`, new scoped provider/gateway tests and fixture
worker, quota GUI presentation tests and any required narrow card seam.
Parent retains catalog projection files.

The public helpers get Go/JS parity and negative fixtures. Ordinary Front
must prove held requests, account fanout, cancellation/body release,
explicit user zero, off/config/stale/replaced bindings. Current native
queues are reused. Rich amount/unit/balance/span/reset presentation must
retain Unknown/Aside and exact zero without fabricating execution cost.

Only usbox builds/tests, offline isolated HOME/network, no live vendors.
No host commit by this lane. After owner acceptance, adapt one ordinary
full ZCode distribution, source-lock/provenance and evidence. Preserve
0.12 artifacts and all prior failures.

## Bounded implementation handoff

The shared tree contains the lane's Go/JS validators and usage hooks,
`internal/provider/extension_defaults.go`, `concurrency.go`,
`extension_models_cache.go`, `extension_refresh.go`,
`extension_provider.go`, `quota_amount.go`, `subscription_usage.go`,
`quotas.go` and `routing.go`. The ordinary GUI quota-count guard is in
`internal/gui/assets/app.js`. That file and
`docs/subsystems/extension-providers.md` also contain parent-owned edits;
their whole-file diff is not attributed to this lane.

The ordinary Front fixture is
`internal/gateway/extension_defaults_test.go` with
`testdata/defaultsworker/main.go`. Provider and presentation fixtures are
`internal/provider/extension_defaults_quota_test.go` and
`internal/gui/tests/extension-rich-quota-functions.test.cjs`. The latter
executes the actual formatter/pace source through Node VM; it is not
rendered-browser evidence. `extension-rich-quota.test.cjs` retains the
browser fixture, but browser binaries are unavailable and were not installed.

On the immutable API9 union over ae957e5, scoped SDK/provider race20 is
**100 + 180 passes**, actual Front race20 **140 passes**, Node/Bun
validator checks **4/4**, strict TS positive fields compile, and the
presentation fixture has **2 passes**. Six compiled causal removals fail
at the expected owner assertions. API9-EVIDENCE.md names exact packet and
log hashes and the conditional AccountStamp closure.

All host files remain uncommitted for the parent to integrate. Do not
apply the historical public-field patch again or include the proposed
AccountStamp overlay as if it were already in the tested source tree.
The module's 0.13 ordinary package and source lock are separate from the
parent's eventual committed shared-host acceptance.
