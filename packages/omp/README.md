# OMP sessions

One native `omp-sessions` plugin supplies OMP history reading and six session
conversion routes: OMP ↔ Claude, OMP ↔ Codex, and lossless OMP ↔ interchange.
The default distribution includes the Source reader and checked Codex lineage
support. Minimum SDK API **1.10**, native ABI **1**, Go **1.26.8** for builds.
The installed library needs no Go, Python, Bun or source checkout.

Build only on regular usbox with isolated HOME/XDG and pinned offline caches.
From this package directory:

```sh
python3 scripts/build.py --output /absolute/empty/omp-dist --sdk /absolute/magpie
python3 -m unittest discover -s tests -p 'test_*.py' -v
```

`--sdk` is optional: `MAGPIE_SDK_SOURCE`, then the workspace's sibling `magpie`,
provide its defaults. SDK selection uses a temporary modfile and never edits
`src/go.mod` or `src/go.sum`. The builder derives the full manifest from the
typed native factory, checks input hashes before/after compilation, and writes
the native library, manifest and provenance into a new or empty directory.
`scripts/build_source_preview.py` is a compatibility name for that same full
builder and accepts the same options.

The Go module, embedded manifest and fixtures live in `src/`; Python unit and
explicit integration checks in `tests/`; author tools in `scripts/`.
Use the workspace's check dispatcher for Go tests/vet/format checks. No browser
tests are needed. `tests/verify_host.py` accepts an explicitly supplied fixture
Magpie binary and distribution for isolated native install and checked
round-trip verification; the Codex/Source-owner scripts need their separately
supplied reviewed fixtures and are not ordinary unit checks.

Reading consent, store ownership, file grants, publication and resume stay
host-controlled. Installation alone does not authorize history access.

- [Detailed behavior and contracts](docs/REFERENCE.md)
- [Donor provenance and limits](docs/SOURCES.md), [source lock](docs/source-lock.json)
- [2026-11-16 source watch and native round-trip correction](docs/SOURCE-WATCH-20261116.md)
- [Relocation record and checks](docs/migration/RELOCATION.md)
- [Historical verification](docs/history/VERIFICATION.md)

Historical receipts describe their original snapshots; they do not certify
this layout or current live clients. No live vendor, account or session is
required for package unit checks.
