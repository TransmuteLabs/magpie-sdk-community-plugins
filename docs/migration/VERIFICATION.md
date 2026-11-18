# Monorepo verification

The relocation preserves the reviewed plugin behavior. Package-specific checks and limits are recorded in each `packages/<name>/docs/migration/RELOCATION.md`. The original SDK audit remains evidence for its own source snapshot.

## Combined acceptance — 2026-11-15

All five default packages built and installed through the ordinary SDK CLI in a private configuration. The combined check command passed, including Go race tests, vet, format checks and registered Python/TypeScript/Node/Bun checks.

| Gate | Result |
| --- | --- |
| `scripts/check.py all --sdk <source4> --race` | PASS, 78.71 seconds |
| `scripts/build.py all --sdk <source4> --output <private>/dist` | PASS, five complete native distributions |
| Frozen source4 Magpie, `go build -mod=readonly -tags nogui` | PASS |
| Ordinary `plugin sdk install`, `inspect`, `list`, `remove` | PASS for all five; installed file hashes match each distribution; catalog empty after removal |
| Shared tooling regressions | PASS, four tests including real Go SDK/modfile paths containing spaces and isolated Python entry points |

The final review found that the original OpenCode donor `.gitattributes` rules had been archived without remaining active. The root `.gitattributes` now restores all three rules at their new paths. A fifth tooling regression performs a real temporary Git index/checkout with `core.autocrlf=true`, compares three actual donor files byte-for-byte and checks both whitespace attributes. Without the rules it records four failures; with only the restored rules it passes. All five root tests pass in the final metadata delta. No target commit or index change is used by this test. Package runtime, build inputs and distributions are unchanged by this correction.

The installed versions were Command Code 0.1.3, Devin 0.8.0, OMP 0.1.0, OpenCode 0.8.0 and ZCode 0.15.0. The test configuration contained only these SDK packages and no `node_modules`. Manifest comparison accounts specifically for Go's omitted `ProviderOption.default=false`; all other manifest fields and all installed file bytes were compared.

The complete build/race/install baseline contains 683 executable/reference files, SHA256
`be05a2f6ae4b1f12d3b93f66c94482869d414d75fac2484e0763a53eb3c92b58`.
After the separately verified Git metadata correction, the final source has 684 files, SHA256
`33414d905947f6de486bd07abc002e85f10bdae0689c2243e9e18dd9d7c893ee`.
Its only differences are `.gitattributes`, the additional root regression, and README's Git prerequisite. Exact manifests are `source.json` and `source-final.json` in the evidence directory.
The SDK source4 content SHA256 is
`609a7b5a1bfd8da599639900e19a2635296b5f5d1ce85bcbd8df0df702d5b493`.
Migration receipts are outside that source identity. They were supplied as a separately hashed documentation overlay for link checks. An initial check run omitted them and failed two Devin documentation-link assertions; that harness error and its log are retained. No product assertion was weakened. The corrected full check passed without another source change.

Exact snapshots, commands, logs and network receipts:
`usbox:/tmp/magpie-monorepo-final-20261115/` (with local summaries at the same path). `result.json` retains the initial result; `check-with-docs-result.json` records the corrected full check. The independent review separately reproduces and closes the initial ZCode ZIP-only/root-directory output mismatch.

## Independent review

Verdict: **ACCEPT for relocation and preservation** on the final 684-file snapshot. Both confirmed migration findings are closed: ZCode default output now satisfies the root builder, and donor Git attributes remain active. No blocking relocation finding remains. The [review receipt](independent-review.json) records exact hashes and evidence. This is a bounded organization review, with the separate runtime limits below retained.

## Historical relocation retry anomaly

The single combined full run passed, but ZCode's unchanged
`TestCompletionPaidBusyRetriesOnlyConfirmedUnusedCaps` is not reliably green under repetition. Separate race/count20 runs against both the copied original and relocated sources observed four/five requests where the test expects three. Each recorded 172 pass and eight fail events, including parent events. The cause is not isolated. No retry code or assertion was changed as part of relocation; this finding must not be reported as fixed by moving files. Details and exact paired logs are in the ZCode relocation record.

This is the result at relocation, not a current package-readiness claim.
The later SDK1.12 policy and its repeated strict-grant regression are recorded
in [ZCode native adoption](../../packages/zcode/docs/NATIVE-ADOPTION-EVIDENCE.md#retry-regression-after-relocation).
The old cause remains unisolated; the later result does not turn the original
paired failures green or attribute their repair to moving files.
See [current status](../STATUS.md) for subsequent package and host gates.

## Preservation

The inventory maps all 667 original source files, including dirty/untracked changes: 581 remain byte-identical and 86 have relocation-related edits. Runtime Go files remain byte-identical. The originals' content, HEAD and status were checked unchanged after relocation. All five full Git bundles verified successfully. See [recovery information](RECOVERY.md) and [preservation summary](preservation-summary.json).

## Limits

Verification runs on regular usbox, Linux/amd64, with isolated user directories, existing offline tool caches and a descendant network guard. No live vendor accounts, installed Magpie, macOS Keychain or actual client sessions are used. This organization change does not provide new live or cross-platform acceptance.
