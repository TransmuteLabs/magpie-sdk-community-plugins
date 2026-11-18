# Claude Desktop

Configuration-only native agent integration for Claude Desktop's supported
third-party gateway mode. It selects Magpie in Desktop's own profile library,
using the application's bundled `app://localhost` interface and a local gateway
key. An Anthropic account or a community authentication plugin is not required
by this configuration path.

| Identity | Value |
| --- | --- |
| Package | `claude-desktop` 0.1.0 |
| Module | `agent-claude-desktop` |
| Agent profile | `claude-desktop-sdk`, target `claude-desktop` |
| Public Go module | `github.com/TransmuteLabs/magpie-sdk-plugin-agent-claude-desktop` |
| Contract minimum | API 1.5, native ABI 1 |
| Declared platforms | macOS arm64 and amd64 |
| Vendor source reviewed | Claude Desktop 5.11492.5 |

The API revision and native ABI describe contracts. The public SDK release
version follows the Magpie application version; these identifiers are separate.

## Use

Build and check the package through the shared workspace commands with an
existing SDK checkout:

```sh
python3 -I scripts/check.py claude-desktop --sdk /path/to/magpie
python3 -I scripts/build.py claude-desktop --sdk /path/to/magpie --output /tmp/plugins
```

Install the resulting `/tmp/plugins/claude-desktop` directory through Magpie's
SDK plugin installer. Select the Claude Desktop SDK agent and connect it to the
gateway. Quit and reopen Claude Desktop yourself after applying configuration.
This package does not automatically launch or restart the application.

The agent uses the host's existing `/agent/claude-desktop` gateway prefix with
the original host-owned key. Desktop model aliases, effort choices, routing,
usage and request logging retain their existing host path. The ordinary
Anthropic provider remains independent.

Four Desktop files under `~/Library/Application Support` are in scope:

- `Claude/claude_desktop_config.json`;
- `Claude-3p/claude_desktop_config.json`;
- `Claude-3p/configLibrary/_meta.json`;
- `Claude-3p/configLibrary/a363652b-0f53-4cd6-aede-cd0000000001.json`.

The fifth declared file, `Magpie SDK/agents/claude-desktop.json` under the same
directory, records hashes of the applied profile fields. It contains no copy
of the gateway URL or key. The host applies and backs it up in the same checked
configuration transaction.

The owned profile uses static bearer credentials, disables the deployment
chooser, sets `desktopHome` to `standard`, and enables Code. Other profiles and
unrelated JSON fields are preserved. Disconnect restores only owned values
that still match the applied configuration; subsequent user edits are kept.
Opaque sealed credentials remain opaque during synchronization.
The selected profile's disabled chooser forces third-party mode when its
optional `deploymentMode` is absent, `"any"` or `"3p"`. An explicit `"1p"` or
invalid profile mode is refused, including on connect. The package initially
writes remembered root mode on explicit connect; synchronization preserves
Desktop's later normalization or removal of those root values.
Synchronization still refuses a changed selected profile, chooser, gateway or
key. An existing hybrid bootstrap pointer must be disabled in Desktop before
connecting; the package does not overwrite that selection. Reconnect explicitly
after changing the host's gateway address or key. Disconnect uses the applied
field hashes, so a host address or key change does not prevent restoration of
unchanged owned fields. Missing or damaged ownership evidence causes an error.

## Verification boundary

The source contract was checked against the recovered 5.11492.5 application
and Magpie's existing Desktop integration. The current source passed isolated
macOS arm64 native-host checks, including profile-mode normalization,
installation, explicit ownership, synchronization, restoration, model aliases,
Messages/SSE and canonical request accounting. These checks used a synthetic
local upstream, without launching Desktop or using an Anthropic account.
Live acceptance of the installed signed application, Chat, Code and Cowork
remains pending. The amd64 target is declared but has not been executed.
Linux and Windows are not declared until their paths and runtime behavior
have been verified.

A controlling or unreadable managed device policy can prevent Desktop from
using a local profile. Cowork also retains the application's VM/runtime and
operating-system permission requirements. Installing this package does not
remove those vendor prerequisites or promise that every feature is offline.

See [design and ownership](docs/DESIGN.md) and
[source provenance](docs/SOURCES.md) for the exact contract and watch points.
[Verification](docs/VERIFICATION.md) records the isolated native-host result
and the remaining live Desktop acceptance. Follow
[live acceptance](docs/LIVE-ACCEPTANCE.md) when testing the real application.
