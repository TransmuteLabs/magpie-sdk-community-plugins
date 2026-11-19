# ZCode

One native package with the ZCode provider, controller, agent profile, static
control page and rooted CLI history. The default artifact is **0.15.0**,
public SDK minimum **1.12**, native ABI **1**. SQLite and runtime behavior are
preserved from the independently reviewed standalone source.

From the monorepo root, in the isolated offline environment on regular usbox:

```sh
python3 -I scripts/build.py zcode --output /tmp/plugin-builds --sdk /path/to/magpie
python3 -I scripts/check.py zcode --race --sdk /path/to/magpie
```

The package builder can also run directly:

```sh
python3 -I packages/zcode/scripts/build.py --output /tmp/zcode-package --sdk /path/to/magpie
```

`--sdk` is optional when `MAGPIE_SDK_SOURCE` is set or the host checkout is
adjacent to this workspace. Go 1.26.8, a C compiler, TypeScript and Bun are
existing build tools; no dependency download is needed. Builds use a temporary
modfile. The root dispatcher appends `zcode` to its output parent, producing
`/tmp/plugin-builds/zcode/magpie-plugin.json`. The direct package builder
writes installable members directly inside its `--output` directory and
refuses a nonempty destination. Runtime
uses the native library and static assets, without an SDK checkout or Bun.

For an optional release archive, the direct builder also accepts
`--archive /tmp/zcode-archives`; this writes the platform ZIP and checksums
outside the installable directory.

The provider-only **0.7.3** composition remains explicit: add `--headless`
and use a separate output directory. Both current compositions require
SDK **1.12** with `request-handoff` for local queue exhaustion. Historical
API 1.4/1.9 receipts describe their original snapshots. Installation is a
separate operation.

The host owns ordinary Messages transport retry admission, bounded waits and
sibling selection. The adapter supplies actual bounded response facts and
separate evidence for whole-execution refusals. Queue admission, Off-Peak
polling and durable financial journals retain their existing owners.
[Policy plan](docs/REQUEST-POLICY-PLAN.md) records scope and verification.

The full package also adopts existing API1.10 source resume and foreground
AgentProcess control. Its page exposes one confirmed CLI input and a matching
stop through selected-account bindings. Workspace configuration, durable
uncertainty and synthetic verification limits are in
[Native foreground](docs/NATIVE-FOREGROUND.md). This starts an owned CLI child;
desktop attachment is not claimed.

- `src/`: unchanged Go module identity, runtime, commands and Go testdata.
- `web/`: active browser sources; `tests/`: verification entry points.
- [Detailed reference](docs/REFERENCE.md), [sources](docs/SOURCES.md) and
  [source lock](docs/source-lock.json).
- [Relocation and verification](docs/migration/RELOCATION.md); historical
  plans/receipts in [docs/history](docs/history/README.md).

Historical evidence is retained as evidence of its named snapshots, not a new
workspace verification claim. No live account, vendor or installed app is
needed for the unit/browser checks.
