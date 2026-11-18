# Paid cache keepalive extraction and acceptance plan

The accepted full-default package remains commit
`7f8fde6cc4de4bb5b3ebc4f4c9a3570312acc077`. This continuation consumes the
generic account-task contract being implemented by Volta after the shared
upstream merge. It changes only this module. No completed Stream, Auth,
HTTP client or credential-bearing native request may be retained.

The independent primitives have full module, scoped race3, build/vet/
provenance and causal mutation evidence in `CACHE-KEEPALIVE-EVIDENCE.md`.
That0.7 preparation remains historical. The resumed0.8 adapter uses the
API9 Task/Key/Job/DueAt contract and ReportAccountTaskUsage snapshots.
Only module-owned declarations change; shared manifest.go, SDK version
and public types remain parent/Volta-owned. The final implementation and
exact installed observations are in `CACHE-KEEPALIVE-EVIDENCE.md`; the
preparation's draft-contract gaps are historical.

## Source boundaries

Community donor: `26ea8faf7793ca66dd8e7a8b90128a5235552a1f`, Devin0.1.40.
`packages/devin/index.mjs` SHA256 is
`c94c6c209f0c25d433da60fd9b1857508c43f47b254f3c85c0f2f9b5ec0f51c0`;
`keepalive.test.mjs` SHA256 is
`a06c09b00cd0620e885772ee240473add4853edeafca7b48b43ee12c4d16a4bf`.
Port byte construction, conversation identity, timing and termination
policy, not OpenCode/Bun timers, retained credentials or aborted-usage loss.

Devinapp: `97a4883009bc988fedcdf0416a50a6c62f8d9c9b`.
Generated API request is `module-432.js:3577`, prompt/cache declarations
are `module-39768.js:2156/2337`, sampling and usage are
`module-98224.js:9990/21694`. The similarly named request in module39768
is a different protobuf type. Its field map is not used here.
The reconstructed Rust cache_keepalive.rs contains log strings, not a
working scheduler. The five-minute TTL and cadence are donor behavior,
not a newly verified live-server guarantee.

## File-level work

1. Add codec/cache_keepalive.go and tests. Extract a bounded immutable,
   credential-free template from the actual encoded request. Preserve the
   order and bytes of original chunks, add one USER continue prompt after
   the final prompt and change only sampling max_tokens to1. Reinsert fresh
   account metadata only during an admitted callback. Refuse malformed,
   duplicate, unknown and unsupported authority-bearing controls.
2. Add provider_cache_policy.go and tests. Keep default-off/options,
   conversation keys, bounded first-message matching, sent-time cadence,
   hit/miss/max policy and bounded template storage independent of timers
   and credentials. Existing encoder, stream parser, usage and diagnostics
   remain the behavior owners.
3. Integrate foreground arm/cancel and a fresh account-task callback once
   Volta supplies the agreed public DTOs. The callback must pass selected
   credentials and proxy through the ordinary public services, emit one
   acceptance before network dispatch, preserve partial measured usage
   during cancellation and never retry an ambiguous tick.
4. Update standard/compatibility manifests and provenance only against the
   final captured contract. The ordinary build continues to produce one
   complete package. Explicit historical compatibility modes remain honest.

## Provider options for the parent's generic editor

```
keepAlive: boolean, default false
cacheKeepaliveInterval: number, minimum 30, maximum 299, default 285 (seconds)
cacheKeepaliveMaxPings: integer, minimum 1, maximum 50, default 6
```

Missing values retain defaults. Invalid explicit values are refused, rather
than accidentally enabling or silently widening paid work. The module owns
the schema and descriptive cost text; the host editor remains generic.

API9 `providers[].options` declares the boolean switch:

```json
{
  "key": "keepAlive",
  "label": "Keep the cache warm",
  "description": "Paid background requests: each ping may bill a cache read of the entire retained context plus generated output.",
  "default": false,
  "confirm": "Enable paid background requests? Each ping may bill a cache read of the entire retained context plus generated output."
}
```

Confirmation applies to an off-to-on change. Only a saved, explicit boolean
`true` authorizes paid work; interval and maximum alone never enable it.
The checked configuration is consent for subsequent ticks, so there is no
per-tick prompt. The two numeric fields belong to module configuration
with the bounds above; the current generic provider switch declaration
does not declare numeric controls. Fractional interval seconds are valid.
Explicit `null`, wrong types, nonfinite numbers and values outside the
declared ranges fail validation even when the switch is off. `server` stays
an independent existing module configuration field.

The parent's common root switch is `keepAlive`. The donor name
`cache_keepalive` in the earlier preparation is historical and is refused
as configuration in this adapter; no paid opt-in is inferred from an alias.
The accounting kind remains the donor's `cache_keepalive`.

## Resumed adapter sequence

`provider_cache_task.go` binds/cancels the exact conversation key before
foreground acceptance, then reserves a bounded credential-free template
from the request actually sent. Active callback cancel functions and done
channels live only until that callback returns; finished Streams, Auth and
HTTP clients are never retained. Eviction and later foreground turns cancel
and join active work outside the store lock.

`provider_execute.go` encodes Chat once and reuses that same payload through
`outbound.Client.Complete`; Messages already has that property. Native
templates use the same deterministic authority replacement as Native.
Only successful strict terminals with measured positive cache arm jobs.
`Job` carries a template ticket and monotonic tick, `Key` is the conversation,
and `DueAt` is the absolute send-time cadence. Each callback gets fresh
selected credentials/proxy, accepts once and reports cumulative measured
usage while live. Protocol/transport failures stop conservatively; an
ambiguous tick never rearms. Two valid completed cache misses retain the
donor's two-miss stop policy.

## Policy and authority

Only a validated successful reply with measured cache read/write arms work.
Tool-stop success qualifies. Each ping uses the same prepared template and
one continue prompt, never a growing chain of pings. Due time is the previous
actual send time plus the interval, clamped to now after a slow reply.
At most32 conversations and a bounded aggregate of template bytes are kept;
the no-session prefix index holds at most256 hashed entries.

The key uses host CallerID, reported session/subagent, instructions and tool
names. No-session growth matching stores hashes and lengths, not text.
Generation/model/account belong to task authority, not to a conversation
key that would fail to cancel an old-model job on the next foreground turn.
Host-projected session/subagent headers are grouping hints, never authority.

Stop on the next real turn, off/config/lifecycle cancellation,401/402/429,
the maximum, or two consecutive misses. A measured hit has positive read
tokens and read >= fresh input + cache write. Absent usage is not a hit.
Finished/canceled/malformed replies never create fresh origin receipts.

Every paid tick needs fresh generic host admission, a one-use accepted write,
current account/model/caller/budget/proxy/quota checks, and cancellation.
Own-call usage carries measured token/cache counts, ACU-valued cost/credits,
upstream IDs and a safe cancellation outcome. Unlike the donor's omission
of aborted pings, measured canceled work must remain observable.
Existing idle auth renewal and usage.append are not scheduler authority.

## Bounded verification

All execution is on usbox with isolated HOME/XDG/TMPDIR, pinned offline
caches, existing Go toolchain and synthetic local peers. Pure tests compare
raw interleaved chunks, signatures, marks, unknown/duplicate rejection,
credential absence and fresh reinsertion. Deterministic policy tests cover
session/subagent/permission-check isolation, bounded matching/eviction,
default-off, slow sends, exact hit boundary, two misses and limits.

The agreed-task peer and installed-host fixture must causally cover
registration, one accepted dispatch, duplicate delivery, held cancellation,
partial measured usage, revoked/replaced authority and no vendor request
after denial. Re-run the existing scoped full-default acceptance on the
same frozen final host. Preserve prior receipts; no live vendor/original
Cognition parity is claimed.
