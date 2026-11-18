# CommandCode

One complete native package: Provider API Chat/Responses/Messages, Go-plan
conversion, account sign-in, models, usage, SystemOne decisions, checked agent
configuration and the browser page. Module `protocol-commandcode`, version
`0.1.3`, negotiates SDK API1.12 and native ABI1, including shared request policy.
The [policy boundaries](docs/REQUEST-POLICY.md) describe proved final refusals,
the named read-only preflight and flows that remain fenced. The audited auth, transport and
safe approval-link behavior is preserved; no optional variant is needed.
Zero Data Retention is an ordinary Providers → Command Code switch with the
donor's cost/no-upstream confirmation. The custom page displays its saved status.
The [current closure](docs/LOCAL-CLOSURE-20261116.md) records the strict-SDK
common-host and browser/API evidence separately from historical checkpoints.

Use the workspace root build/check commands for routine development. The package
builder API, run from the workspace root, is:

```sh
python3 packages/commandcode/scripts/build.py --output /tmp/commandcode-package --sdk /path/to/magpie
```

`--output` must be empty. `--sdk` is optional: `MAGPIE_SDK_SOURCE` or the sibling
Magpie checkout supplies the default. Builds use a temporary modfile and do not
rewrite `src/go.mod`. The default is an installable directory containing
`magpie-plugin.json`, native `lib/`, `package.json` and bundled `web/`.
Pass `--archive` explicitly to also produce a ZIP beside that directory;
installation needs no source checkout or npm runtime dependency. Verify a ZIP with
`python3 packages/commandcode/tests/check_package.py /path/to/archive.zip`.

The normal verification target is regular usbox with isolated HOME/XDG,
pinned offline caches and the network guard. The November16 closure also has
explicitly authorized local macOS verification with equivalent isolation and
the parent-proven sandbox. Go checks run in
`src/`: `go test ./...`, `go test -race ./...`, `go vet ./...`, `gofmt -l .`.
The root checker supplies the selected SDK and isolation.
The browser type check is `python3 tests/typecheck.py --sdk /path/to/magpie`
from this package directory; it uses the same temporary SDK mapping as the builder.

Repository references: [details](docs/REFERENCE.md),
[donors and provenance](docs/SOURCES.md), [source lock](docs/source-lock.json),
[relocation evidence and limits](docs/migration/RELOCATION.md).
Old plans and receipts in `docs/history/` keep their original identities; they
are not new monorepo test results. Native Linux fixtures and synthetic browser
tests do not establish live vendor approval or native macOS/Windows behavior.
