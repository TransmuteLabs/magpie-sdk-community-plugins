# Current Devin provider comparison

The default native `devin-inbound` 0.8.2 package is self-contained. It already
owns account auth, catalog, quota, Chat/Messages execution, paired native
ingress and the Devin agent/bootstrap surface. No community plugin, Bun,
Node module or separately installed Devin CLI supplies its provider hooks.
The provider ID inside this package is `devin`; the host assigns its own
namespaced provider identity. Installing this package does not adopt retired
built-in accounts or install a legacy community migration target. Sign in
explicitly to this package.

The exact source map is [source-lock.json](source-lock.json), explained in
[SOURCES.md](SOURCES.md). The community donor comparison includes
`b35d4b35b2446bd929896ddeb58f3cb299e9e2d1`, package 0.1.44. Its scoped
snapshot-marker, adjacent signed-thought and long-session changes are recorded
there alongside the recovered protocol baseline. Donor functions named below
identify behavior, not a claim of whole-donor or live-vendor parity.

The [original 0.1.41 comparison](history/provider-comparison-0.1.41.md)
preserves its exact source identities, failures and verification limits.
The [donor matrix](DONOR-MATRIX.md) retains the detailed codec boundaries.

## Provider behavior

| Behavior and donor evidence | Native package and current result |
| --- | --- |
| Provider registration: `DevinAuthPlugin` auth loader, config and provider.models hooks; OpenCode2 `v2Setup`/`devinLanguage` | [manifest](../src/magpie-agent.json), [provider module](../src/provider_module.go) and [agent module](../src/agent_module.go) register the equivalent native SDK surfaces. OpenCode v1/v2 adapters are not another runtime requirement. |
| API key/session token: auth methods, `success`, `apikey.test.mjs` | [auth owner](../src/provider_auth.go) accepts explicit credentials and verifies selected-server status before saving. No recovered refresh/revoke RPC: refresh requests sign-in again, logout removes the host account. |
| Browser PKCE: `browserSignIn`, `exchange` | [PKCE codec](../src/outbound/auth.go) implements the same authorization/exchange protocol with one-use, bounded host-private state. Explicit SDK redirects and full callback submission remain supported. The package page uses a module-owned loopback receiver and existing host poll completion; see [onboarding](ONBOARDING.md) for lifecycle, revocation and verification limits. The module does not launch a browser. |
| Identity: `whoOfStatus`, `success`, `identity.test.mjs` | Email/name plus organization are used for labels, with fallback order `devinInfo.accountDisplayName`, `planInfo.teamName`, `userStatus.teamName`. Display edges are trimmed without repeating the same label. IDs remain hashes of selected server and credential, including when no email/name is available. Two keys with the same label remain distinct. |
| CLI account reuse: `cliSignIn` (1024), `live` (3150), `cli-key.test.mjs` | Not imported: no reads of another application's credentials, CLI subprocess identity fallback or automatic credential replacement. An unavailable status endpoint refuses new sign-in instead of trusting a CLI/snapshot. Explicit API-key/session-token input is the self-contained alternative. |
| Catalog: `configsFor`, `ownFamilies`, `familiesFor`, `listed`, `provider.models`; free/prices/images tests | [model projection](../src/codec/outbound_models.go), [selection](../src/codec/model_selection.go), [provider domain tests](../src/provider_domain_test.go): current selected-account discovery, concrete UIDs, families/aliases, ordered efforts, fast/priority tiers, images, limits, known prices and free scopes already exist. Router picks Adaptive/Fusion remain excluded. |
| Catalog fallback and unknown models | Donor caches configs, can use CLI output/static snapshots and retain configured models. Native virtual selections reread the selected account's catalog before acceptance; no snapshot grants unavailable models. Explicit native UIDs retain native semantics. Label-only family names outside SDK ID grammar retain their concrete UID/display label. Failures do not become a fabricated catalog. |
| Messages turn effort: CLIProxyAPI `061ffe6fb` | The last `system` message's `output_config.effort` selects the account's native family variant ahead of the top-level cache placeholder. Disabled thinking keeps the existing default and a concrete model UID stays pinned. Invalid inline configurations are refused before inference. |
| Signed-out usage: `usage`, auth.usage, `usage.test.mjs` | Status is read before models; explicit Connect `unauthenticated` returns a typed refusal and constant sign-in message. Bare HTTP401 stays a transport fact without inferring expired credentials. Catalog4xx refusals propagate; transient/malformed catalog data still defers quota because free scope is unknown. Fresh success recovers current revision/windows. No automatic refresh or replay is added. |
| Signed-out host UI/routing | The host consumes typed `unauthenticated` usage failures through its shared selected-account auth owner. Generation, package/config revision and the current usage-read token fence publication; stale or superseded results cannot clear a newer refusal. The SDK has no `UsageResult.signIn` field. Bare HTTP401 alone is not an inferred credential expiry, and this projection adds no automatic refresh or replay. |
| Account metadata and plan display | Donor refreshes identity metadata during usage and prettifies tier enums into plan names. Native saves identity on explicit auth; usage does not rewrite account metadata. SDK has no top-level plan/expiry result. Plan end uses an informational window/reset; tier prettification and ongoing metadata updates are not ported. |
| Quota/accounting: donor usage/free/prices | [status decoder](../src/outbound/status.go) and [usage tests](../src/provider_usage_test.go) preserve day/week windows, free exclusions and present ACU values. API1.9 includes units, signed micros and plan-end reset; API1.7 compatibility retains its percentage representation. Unlike donor positive-balance dollar text, unknown currency is not inferred. Execution facts/ledger remain separate from quota. |
| Maintained CLIProxy dimensional usage | Sparse field28 Token Usage reaches canonical/SDK/ledger paths without displacing field7 snapshots or inventing missing counts. The [exact comparison and limits](CLIPROXY-PROVIDER-CHECK.md) preserves the donor evidence; shared refusal policy is documented separately in [REQUEST-POLICY.md](REQUEST-POLICY.md). |
| Canonical/native requests and compression: `build`, `buildMessages`, `frame` (1149), `gzip-request.test.mjs` | Existing native and canonical encoders retain selected authority and ordinary supported message/tool/image/sampling fields. [Connect encoder](../src/codec/outbound_response.go) gzips payloads greater than 8KiB. The installed test checks flag, header, selected model/key and decompressed content for Chat, Messages and native requests. |
| Paid background work: `scheduleKeepalive`, `keepalivePing`, keepalive tests | [Cache policy](CACHE-KEEPALIVE.md) retains explicit default-off consent, host-authorized fresh-account scheduling and bounded conversation state. A successful tick with absent or partial cache measurements now stops; it cannot authorize another paid tick. Complete measured hits/misses keep their existing policy. Donor retained-token timers, alias-as-consent and initial502 retry are deliberately excluded. |
| Agent/native bootstrap | Native package owns the finite account/bootstrap operations, `GetAccountManagedPluginBundle` and `devin-native` wiring. Original-client startup with an assigned cloud bundle remains a live verification gate. |
| Scoped 0.1.44 donor changes | Recognizable image bytes determine inline MIME. Empty unmarked user residue is omitted after deferred tool-image association; meaningful text/images remain ordered, and unrepresentable empty cache boundaries refuse explicitly. Signed thoughts retain scoped origin checks. Automatic unsigned retries and length-based context-error guesses are not imported; [SOURCES.md](SOURCES.md) records the exact donor paths and focused tests. |

## Exact-semantics differences retained

These donor behaviors are present in its source but are not ported blindly:

| Donor behavior | Package boundary |
| --- | --- |
| `response_format` / Messages `output_config.format` become prompt instructions (`formatText`, structured-output tests) | Unsupported controls fail explicitly; a prompt suggestion is not a strict output-format guarantee. |
| `objectRoot` folds root schema alternatives; `geminiSchema` and model-specific prompt/tool-description substitutions, system-to-user conversion and billing-line removal | Supported tool schemas and source prompt content remain exact; unsupported shapes are refused rather than weakened. |
| `temperature`/`top_p` zero becomes `1e-6` | Supported explicit zero remains zero. |
| Missing tool results, assistant-tail filler, coercions and `is_error: true` rendered as `Error:` text | No invented user turn/result; unsupported error-result semantics remain explicit. |
| Global signature/type caches, guessed types and whitespace-collapsed thought matching | Existing bounded account/generation/caller/model/conversation scope and origin receipts remain required. Audited multi-thought/metadata behavior is preserved; only the separate usage projection changes in the renderer. |
| Estimated missing/zero output tokens and permissive completion | Measured counts preserve absence/zero; malformed/incomplete/error terminals do not become successful replies or continuation origins. |
| Message-text status heuristics and raw upstream messages | Only explicit status/code is classified. Private provider text is not emitted. 403/429 and message-only claims do not become auth refusals. Post-dispatch uncertainty is retained. |
| Environment server overrides, CLI server selection and returned host hints | Explicit module-config origin and selected credential stay authoritative; redirects cannot move credentials to a different origin. |

Cache marks, inline/deferred images, ordered tool/result content, signed
thinking, genuine Gemini hidden thought state and finite native controls
retain the detailed supported/refused cases in the donor matrix. Those
boundaries do not imply full original-client parity.

## Ordinary package entry points

Install the complete build directory, then use this package's provider ID:

```sh
magpie plugin sdk add /path/to/plugin-distributions/devin
magpie plugin sdk auth start devin-inbound devin \
  '{"method":"api-key","input":{"key":"YOUR_KEY"}}'
magpie plugin sdk auth finish devin-inbound devin \
  '{"continuation":{"id":"HANDLE_FROM_START"}}'
magpie plugin sdk accounts devin-inbound devin --json
magpie plugin sdk call devin-inbound provider.models '{}' \
  --scope '{"provider":"devin","account":"ACCOUNT_ID"}'
magpie plugin sdk call devin-inbound provider.usage '{}' \
  --scope '{"provider":"devin","account":"ACCOUNT_ID"}'
```

Use the model ID returned by the running gateway's `/v1/models` in ordinary
Chat or Messages requests. For reauthentication, supply
`--scope '{"account":"ACCOUNT_ID"}'` to both auth calls; a different
credential fingerprint is a separate account. Browser PKCE and callback
submission are documented in [account setup](REFERENCE.md#account-setup).
The explicit historical inbound-only artifact has no provider; the default
build is the complete artifact described here.

## Verification boundary

The [HTTP classification tests](../src/outbound/client_test.go) and
[SDK auth/usage tests](../src/provider_completion_test.go) cover the selected
provider boundary. The optional
[single-package fixture](../tests/provider_single_package_e2e.py) drives the
actual native library through ordinary host auth, catalog, request and usage
paths. It checks named/anonymous account separation, PKCE mismatch/replay,
auth refusal, catalog deferral/recovery, gzip for all three ingress forms,
and exactly three measured ledger rows without credentials or paid work.

Run only on regular usbox under the pinned offline environment and descendant
network guard, with a temporary HOME/XDG and synthetic loopback vendor:

```sh
# package cwd; root check.py also runs these units and Go checks
python3 -m unittest discover -s tests -p '*_test.py'
# explicit integration test, using a prebuilt host and default package
python3 tests/provider_single_package_e2e.py --magpie /path/to/magpie \
  --distribution /path/to/devin-distribution --artifacts /tmp/devin-provider-results
```

[`provider_cache_measurement_test.go`](../src/provider_cache_measurement_test.go)
checks that absent or partial measured usage cannot schedule another paid
keepalive. Host authentication projection, account/config revocation,
installation, native loading and original-client startup have separate
owners and verification gates.

The [preserved comparison](history/provider-comparison-0.1.41.md#verification-boundary)
records the original v1 failure, successor v2 correction and exact host,
library and receipt identities. The [source map](SOURCES.md) records subsequent
bounded donor comparisons. Those historical results cannot establish
acceptance of a later combined host or library.

No live account/vendor, cross-platform native runtime or complete original
CLI compatibility is established by this reference. Final release checks must
name the actual host source, package source and built distribution.
