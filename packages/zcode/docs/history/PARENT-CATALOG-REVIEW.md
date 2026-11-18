# Independent catalog review closure

2026-11-14. Read-only review; no parent source or test file was edited by
the ZCode lane. This closes two catalog findings, not the final shared
host release gate.

The independent probe is
`dist/parent-ledger-review-_39ywg8i/catalog_reproduction_test.go`, copied
unchanged to
`usbox:/tmp/magpie-zcode-parent-review-nAEeSyhn/catalog_reproduction_test.go`.
Its SHA256 before and after closure is:

`c94f2bfbb8b7bbf20503b28b727a3338178691a13749b119221a820ae5b4a0ba`.

On exact source tree `43b7e290d9c3370881780b23ad49222f5d27f44b`,
the original visible-name control passed, while renamed-model quota and
Sub2API balance-window assertions failed. The parent fixed the projection
and added its tests in three files. The closure overlay maps exactly
those three files plus the unchanged independent probe:

| Parent target | Closure replacement SHA256 |
| --- | --- |
| `internal/provider/extension_catalog_projection.go` | `32e11612b3b3a347a9ae69a33b4fbbf4c91a86b50190e3f17952fab50702030b` |
| `internal/provider/extension_catalog_projection_test.go` | `cd036497dd03886d5f59d06712ed96a04c6e6f2d47cb66453e95177ac30138eb` |
| `internal/provider/extension_catalog_quota_test.go` | `471b09a020cfa7061bf938ac7ac0462dd2fa1d2b2e0aae95dcfeda474aade9db` |

I independently read and parsed the owner's exact
`/tmp/magpie-sdk-final-base-xus7_td9/catalog-review-race20-result.json`
and `catalog-review-race20.log`, checked every overlay replacement hash
and rehashed the probe. The scoped race20 log contains **480 pass,
0 fail/skip/race**, including **80 independent probe pass events**:
original control, renamed-model subtest and parent, and balance-window
test, each repeated 20 times. Its SHA256 is:

`de2c10db440b99827a8b8eed08a8d563375e8886397f013d8d6e281fd0dde5cb`.

The closed causes are quota lookup by visible alias instead of original
model identity, and a plan-only quota key excluding ordinary balance-kind
windows. No additional ledger/accounting defect was confirmed by this
bounded review. Previously fixed time-import and Hold-before-stamp issues
are not new findings.

The owner's closure uses isolated HOME/offline caches and owner-visible
UID1000 with inherited descendant seccomp network filtering. Its record
explicitly says no private network namespace; this is not the ZCode
fixture's separate loopback-only namespace proof.

The copied result, complete log, overlay, network record and independently
derived report are preserved under
`dist/parent-ledger-review-_39ywg8i/closure/` and at the corresponding
remote review root. No budget/masking/settings/keepalive census extension
was undertaken for this closure.
