# Actual native-module rollback verification

Verification code only. `host_test.go.fixture` is not compiled or shipped
as a provider module. It imports the public SDK and standard library;
its package-level host test accesses the existing Actions test hook only
after the driver copies it into a separate SDK source copy.

The caller supplies an explicit immutable SDK checkout and a built
headless native ZCode distribution. The driver creates another temporary
SDK copy, adds only the `_test.go` fixture and runs its bounded tests.
It hashes every production file before/after and leaves the original SDK
and MAIN untouched. Python generates a synthetic SQLite input; the actual
native Go module performs the conversion, with no Python runtime bridge.

On usbox, under temporary HOME/XDG and pinned caches:

```sh
python3 -I testdata/sqlite-rollback/run.py \
  --sdk /path/to/immutable-sdk \
  --distribution /path/to/unpacked-native-zcode \
  --output /path/to/new-proof-directory
```

The actual Manager installs and loads the real native module. The actual
SessionMigration path returns its actual binary-snapshot bundle file
plan. After the real host write, the verification-only hook cancels.
Twenty fresh race-enabled test processes keep native library lifetime
bounded instead of accumulating native images in one process.
Tests assert strong preimages, exact source/target/backup bytes, mode,
used-plan rejection and the host backup journal status.

- Normal post-write cancellation restores the original and records
  `rolled_back`.
- A foreign write before execution prevents every write/backup.
- A foreign write during rollback survives, returns an authority/change
  error, and retains `prepared` backup state for explicit recovery.

No host production overlay, vendor call, financial import, credential,
installed app or auto-snapshot capability is involved.
