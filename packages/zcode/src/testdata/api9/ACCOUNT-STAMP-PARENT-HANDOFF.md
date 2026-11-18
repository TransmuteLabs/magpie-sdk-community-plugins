# Cold configuration reads must not invent account changes

The API9 concurrency lane reads the module configuration revision for
static providers too, so queued selection cannot outlive configuration
replacement. Do not remove that fence to make a catalog test pass.

On the immutable ae957e5 + API9 union packet, the scoped adjacent race
suite reached 238 passes, one failure, one Bun-unavailable skip:
`TestExtensionProviderCatalogChangesInvalidateHeldSnapshots` fails at
`extension_hold_test.go:116`: `install: an unchanged catalog moved its stamp`.
Full log: `usbox:/tmp/magpie-api9-zcode-OkPR9GHA/green-adjacent-provider.log`,
SHA256 `b84f712e2c34113c78a14826b18f81cbd7b2f3eec159a26076f3f879b32cf9db`.

Root path: `extensionModelRevision` → `Manager.ModuleConfig` →
`Store.StateGet` → private namespace creation. `accountStampAt` currently
hashes the namespace name even when `accounts.json` does not exist.
The next ordinary Hold sees a synthetic account change.

The parent owns `internal/extension/account_stamp.go`. The minimal
proposed contract change is to skip a namespace whose account-file stat
returns `os.ErrNotExist`, before hashing its name. Keep every directory,
symlink, file identity, permission and unreadable-state check.

```diff
 accountInfo, statErr := accountFileInfo(namespace)
 namespace.Close()
-hash.Write([]byte(entry.Name()))
-hash.Write([]byte{0})
 if errors.Is(statErr, os.ErrNotExist) {
-    hash.Write([]byte{0})
     continue
 }
 if statErr != nil || !accountInfo.Mode().IsRegular() {
     return AccountFileStamp{}, fmt.Errorf("extension account stamp file is unavailable")
 }
+hash.Write([]byte(entry.Name()))
+hash.Write([]byte{0})
 files = append(files, accountInfo)
 hash.Write([]byte{1})
```

Independent behavior fixture:
`testdata/api9/account_stamp_probe_test.go.fixture`. Overlay it as
`internal/extension/api9_account_stamp_probe_test.go`, then run
`^TestAccountStamp` and the unchanged provider catalog Hold test, isolated
on usbox. The module lane does not edit or commit the shared stamp file.

The unchanged probe SHA256 is
`7b5379a8bd4f0f5e11d53f32976156eb2218c5175470bbe9ae67f50dac7464cf`.
`account-stamp-red-result.json` records one compiled assertion failure,
with log SHA256
`bec453f15ca1626c1b67e5070bfb032b75072af3c0cb018ce7fa0129c772ba35`.
The proposed hunk was applied only through a Go overlay:
`account-stamp-proposed-result.json` records **160 AccountStamp and 20
unchanged catalog Hold passes** under race20. Its receipt SHA256 is
`a8cabc0a8800338c41b9e4d54e511a62d3389e563364c80b934c64d08a58dea7`.
The same proposed overlay passes the adjacent scoped provider/gateway
suite, **240 + 12 passes**, zero failures/skips/races; receipt SHA256
`e09afd2edce2397a0605f73a7355d7d5e4a8a6ec0ac257f2d0afc360af34eb0d`.

These are conditional closure proofs for the proposed parent fix. The
frozen source inventory still contains the original stamp implementation.
Integrated parent closure and the final committed shared-host freeze
remain pending; no empty-namespace exception was added to provider tests.
