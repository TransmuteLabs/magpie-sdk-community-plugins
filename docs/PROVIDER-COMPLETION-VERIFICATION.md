# Devin and MiniMax provider verification

Date: 2026-11-15. This record covers the completed provider packages against
the separately frozen SDK API 1.11 host. It does not certify the in-progress
API 1.12 shared policy or the whole current Magpie working tree.

## Reviewed artifacts

| Package | Source identity | Native Linux/amd64 artifact |
| --- | --- | --- |
| Devin 0.8.0, minimum API 1.9 | v2 archive SHA256 `c83b9ad897242b7ea93cb40f50c12c2e2b6542890757a947a2f2325819f7a165`; source manifest including workspace helper `53c1f0fbf41d436e5aee3396a6f0a6b79ed69d418dad4b936335af85fb808b27` | `eaca5723277f57e1d8df30b43e4ce1e76d8968ff6daef0145d29d845f2312978` |
| MiniMax 0.1.0, minimum API 1.11 | 35 author paths, content `18362762aa982cf1e0a186e03b9338d5bd9ce7f91275cfb40d8ba8615860b5fd` | `4ae1687285445c4570e1c08db7c6be229ad775969e57e3344f66cf0e2aedcc38` |

Both use host content
`609a7b5a1bfd8da599639900e19a2635296b5f5d1ce85bcbd8df0df702d5b493`,
binary SHA256
`dfc5c0ddcaae39339ef5d2008056d0b63ad1b7821c6f87bf6af472913e2f1e79`.
Content identities describe captured files, not invented Git commits.
Each package includes its exact donor repositories, versions, paths and update
watch points in its packaged source lock and source guide.

## Verification and independent review

All execution used regular usbox, private HOME/XDG, fixture endpoints and a
descendant network guard. Go gates used pinned offline caches and Go 1.26.8.
No live accounts, vendor calls, installed applications or admin server were
used. The authors and reviewers retain earlier failures separately.

Devin passes full package tests, race tests, vet, formatting, 20 Python checks
and the default native build. Its ordinary installed-host scenario covers
sign-in/reauth/logout, PKCE mismatch and replay controls, refusal and deferred
usage, live catalog discovery, gzip, Chat, Messages and native execution.
Three inference requests create exactly three measured ledger rows. Field 28
dimensions remain sparse; absent usage does not authorize another paid task.

The independent v2 closure replays the unchanged paid-task regression and
neighbor controls under race, and reruns the installed-host fixture against
the exact reviewed library. Its verdict accepts the bounded completion delta.
Evidence: `/tmp/magpie-devin-independent-20261115/CLOSURE-v2.md`, coverage
inventory and independent packet SHA256
`a9e9f4173ef348e96bc364806176adf4192afecf65f38b32d4eeccda8cf5a0b9`.

MiniMax passes full package race tests, vet, formatting and the native build.
Independent production-module tests exercise both regional sign-ins, chat,
streaming and tools, search, images, video, cancellation, redaction and usage.
Twenty ordinary-host cases and the original four malformed-response
regressions pass on v4; valid authentication controls also pass. Its own
HTTP transport verifies both the HTTPS proxy and origin identities.

MiniMax's independent verdict and explicit read coverage are retained in
`packages/minimax/docs/history/host-e2e-20261115`. Core author evidence:
`/tmp/magpie-minimax-core-20261115`, packet SHA256
`b3d7f92b82dc2259fafe181fb60fa4b49aaac8934f845ce177e70d1413dbb6c1`.
This is a bounded review, not a claim that every historical donor feature or
every test file was reviewed.

## Parent standalone installation check

The parent additionally ran the same ordinary SDK CLI lifecycle independently
for the two exact reviewed distributions:

```text
python3 tests/install_packages.py --magpie <source4-host> \
  --dist <fresh-distribution-parent> devin minimax
```

Both pass installation, inspection, account listing, Off, On, reload and
removal in a fresh home. The catalog contains one native package at a time,
with 14 installed files for Devin and 7 for MiniMax. Every installed file
matches its distribution hash and provenance. No account, companion plugin,
Bun dependency tree or legacy plugin is created.

Receipt: `usbox:/tmp/magpie-provider-standalone-final-20261115/result.json`;
command output, network guard and temporary-distribution cleanup are retained
beside it. Read-only preservation inspection also confirms that the 540
previously captured files of CommandCode, OMP, OpenCode and ZCode remain
byte-identical.

## Remaining host work and validation limits

The shared selected-tool preflight and terminal publication fix are verified
separately from these package artifacts. API 1.12 request policy is still a
separate implementation/review lane. Neither package introduces its own
credential-recovery or business-retry owner.

The gateway's terminal 409 fence correctly prevents replay, but the independent
MiniMax test exposed loss of the underlying vendor refusal in ordinary trace
and ledger metadata. That shared host finding remains open until its own
regression and independent closure; changing a provider's client status is not
the remedy.

No live vendor or real-agent acceptance, non-Linux execution, whole-current-host
suite, publication, commit, merge or installed-app migration is claimed by this
record.
