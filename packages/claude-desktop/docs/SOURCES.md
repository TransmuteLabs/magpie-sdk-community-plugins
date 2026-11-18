# Source provenance

Package `claude-desktop` version **0.1.0** provides module
`agent-claude-desktop`, agent profile `claude-desktop-sdk` and canonical target
`claude-desktop`. Its minimum contract is **API 1.5**, native ABI **1**.
These are distinct from the public SDK release, which follows the Magpie app
version, and from the vendor Desktop version.

## Reviewed references

| Reference | Exact identity | Role |
| --- | --- | --- |
| Recovered Claude Desktop | Local `Agents/claudeapp-gui`, commit `ff8d92302c3d0d26ca0629b8344687ad3f0928c5`, Desktop 5.11492.5 | Vendor configuration, mode precedence and normalization, 3p startup, credential, bundled UI and inference contract |
| Recovered Desktop follow-up | Local `Agents/claudeapp-gui`, commit `120edadaf4622bae55b2928ecd63a2fa26b0f0cb`, Desktop 6.6395.0 | Checked profile selection, gateway options, catalog and Code child inference |
| Magpie Desktop integration | `TransmuteLabs/magpie`, commit `31dcfcc3369544f0a9f7acdb3ec02cd7cbf3c771` | Existing configuration ownership, Desktop gateway and model alias implementation |

The recovered repository had no configured public remote when inspected.
Its origin is the locally supplied recovery of `/Applications/Claude.app`
5.11492.5 documented in `restored/RECOVERY.md`. No public repository URL or
vendor-authored TypeScript provenance is invented for that recovered source.
Selected exact file hashes are in [source-lock.json](source-lock.json).
The three listed Magpie implementation donor files were unchanged from the
recorded commit when read; the surrounding SDK checkout has separate work.

The community `cowork` package refers to a different product/integration and
is not a donor for this Desktop protocol. It is not a runtime dependency.
Neither the recovered source checkout nor another community package is
required to run the resulting native SDK package.

## Configuration contract coordinates

All coordinates here are in recovered
`restored/src/build/index.chunk-xV3fJhc7/index.ts`:

| Reader | Coordinate | Behavior to preserve |
| --- | --- | --- |
| `Su`, `wu`, `Eu` | 59723, 59857, 59860 | Third-party directory, configuration and library paths |
| `c1e`, `d1e`, `$lr` | 62623, 62646, 170072 | Applied UUID, hybrid-pointer precedence and flat local configuration |
| `VZe`, `HZe`, `q5n` | 61199, 61209, 161806 | Explicit profile mode precedes chooser; forced gateway mode precedes persisted root mode |
| `rWi`, `sWi`, `VD`, `writeJsonFile` | 237546, 237584, 144646, 18726 | Conditional clearing of an obsolete stored first-party selection |
| `BFe`, `$Fe` | 43223, 43472 | Static gateway credential and base URL settings |
| `kE`, `hasResolvedCredential` | 135238, 158538 | Static credential without interactive Anthropic sign-in |
| `getMainWindowUrl`, `LOi`, `JGi` | 158599, 225059, 239183 | Real bundled 3p interface and app protocol |
| `GGi` | 238849 | Local bootstrap/account/settings API handling |
| `desktopHome` schema | 47834 | Current Chat/Cowork surfaces; separate Code switch at 47202 |
| `Ro`, `zo`, `eAn` | 37567, 37641, 128899 | Model admission and catalog shape |
| `JT`, `uuo` | 132522, 312984 | Child inference credentials and base URL |

The review was read-only: no Desktop process, network, account, credential
store, hosts/CA change or live user configuration was used. Recovery metadata
describes reconstructed modules and types; this review does not turn its
syntax check into original-source or runtime equivalence proof.

The recovered bootstrap `index.pre/index.ts` repeats the mode selector at
56538–56565 and applies the third-party user-data path at 58786–58803.
The selected profile's explicit `"1p"` wins over an otherwise disabled
chooser. With a configured gateway and disabled chooser, absent/`"any"`/`"3p"`
profile modes do not depend on the remembered root mode. The removal path
above is conditional; it does not establish that every launch deletes a
stored `"3p"` value.

## Update procedure

The 6.6395.0 follow-up confirms the same gateway profile and catalog contracts
in `restored/src/build/index.chunk-yuTTraBu/index.ts`. Its gateway URL validator
allows plain HTTP on `127.0.0.1`, `localhost` and `[::1]`. The plugin's
endpoint check follows that list, so it rejects unsupported loopback aliases
before producing a configuration plan. The check also rejects a missing
hostname, ports above 65535 and IPv6 zone identifiers. The newer Desktop
was inspected as source. Installed application acceptance remains pending.

Compare a new Desktop recovery with the pinned files and follow actual
callers of each watched function. Check profile flattening, deployment
precedence, sealed credential syntax, default directory selection, bundled
UI availability, surface configuration and model admission together. An
unchanged endpoint string alone does not establish protocol compatibility.

Compare Magpie's Desktop gateway/configuration donors when updating the host.
Retain the original key and `/agent/claude-desktop` path so catalog aliases,
effort handling, routing, accounting and logging continue through the host.
Do not introduce provider registration to solve an agent configuration issue.

Run `src/agent_test.go` and `tests/provenance_test.py`, then the ordinary
host-owned agent paths. The current source passed isolated macOS arm64
native-host checks, including profile-mode normalization and refusal,
Messages/SSE, model aliases, token counting, canonical request accounting and
restoration. [Verification](VERIFICATION.md) records the frozen host source,
library and evidence hashes. Live signed-application acceptance of Chat, Code
and Cowork and execution on macOS amd64 remain pending. Linux and Windows
compatibility is not claimed. Follow [live acceptance](LIVE-ACCEPTANCE.md) for
the real application checks.
