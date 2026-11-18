# Historical records

These plans, receipts and prototype records retain the standalone repository's
original paths, hashes, versions and results. Their text is not rewritten to
claim verification of the monorepo. Use [the source map](../../../../docs/migration/source-map.json)
for old-to-new locations and [the relocation report](../migration/RELOCATION.md)
for current checks.

The prototype remains historical; active browser code lives in `../../web/`.
Its compatibility re-export now points to that location. Existing prototype
regressions are still run against the active compiled control code.

`src/testdata/donor0166/collect.py` (relative to the package) is an immutable
receipt collector for its externally supplied original-layout snapshot bundle,
not a current workspace acceptance runner. Other historical replay drivers
still require their explicitly named pinned fixture inputs. Their successful
old receipts are not regenerated during relocation.
