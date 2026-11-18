# Optional paid cache warming

The standard0.8 distribution contains inbound/outbound execution, native
agent/bootstrap, authentication/models/usage and the paid cache task in one
package. It requires SDK API1.9. Build it with `python3 scripts/build.py --output DISTRIBUTION`
on the supported build host. No community or companion plugin is installed.
The explicit paired0.5 and inbound0.2 compatibility artifacts have no task.

The provider's ordinary **Keep the cache warm** switch is off by default.
Enabling it requires the declared paid confirmation: each ping may bill a
cache read of the entire retained context plus generated output. This is
inference spending, separate from idle authentication renewal.

Root module configuration:

```json
{
  "keepAlive": false,
  "cacheKeepaliveInterval": 285,
  "cacheKeepaliveMaxPings": 6
}
```

Keep an existing `server` value when updating the configuration. The boolean
switch uses the parent's generic checked provider editor. The two numeric
values are module configuration, not additional boolean provider switches.
Interval seconds accept fractions from30 through299. Maximum pings must
be an integer from1 through50, per successful foreground reply. Omitted
values use the defaults above. Timing values alone never enable the task.
Explicit nulls, wrong types, alternate spellings and out-of-range values
are refused. The donor's old `cache_keepalive` name is not a consent alias.

Only a strict successful foreground completion with measured positive cache
read/write arms a task. A ping preserves the actual request's prompt/tool/
signature/cache/sampling bytes, appends one unmarked user `continue` with a
new ID and changes only `max_tokens` to1. Repeated pings reuse that prepared
template rather than appending more history. Unsupported native controls
skip optional warming; they do not weaken ordinary execution validation.

Current accepted hostd5b098d8/tree11c2 and the production-identical
parent-built binaryc018 pass three bounded installed hot-config-CAS
repetitions without restart or catalog warmup. They exercise actual30s
cadence, budget/model/readOnly refusals, both paid arrivals and held
cancellation with exactly two measured ACU rows. The strict fixture checks
sampling and named tool choice bytes; no policy or timing was relaxed.
Earlier cold controls remain separate, and the original c018 held38
timeout remains unexplained rather than proven fixed.
`DELIVERY-EVIDENCE.md` pins the binary's actual build tree8fe, accepted
test-only tree differences and all old RED/current GREEN receipts.
Catalog/config revision and paid admission fences stay intact.

The host owns scheduling and resolves current selected-account credentials,
caller/model/budget/quota and proxy policy before each tick. The module
retains a bounded credential-free template, no finished Stream, Auth or HTTP
client. Conversation keys distinguish callers, sessions and subagents.
There are at most32 templates,32MiB aggregate content and256 hashed fallback
conversation identities. Templates are memory-only and disappear on module
restart, shutdown or configuration replacement.

Cadence begins at the actual request send, with an immediate eligible tick
after an overdue reply. There is no cache TTL request knob or verified live
five-minute guarantee. The next real turn cancels and joins a held ping for
that conversation. Turning the option off or revoking host authority stops
scheduled/in-flight work. A measured hit needs positive cache reads at least
as large as fresh input plus cache writes. A successful ping with absent or
partial input/cache measurements stops without scheduling a successor;
unknown-only field28 metrics do not count as measured usage. Two completed
measured misses, the maximum
or401/402/429 stop warming. Any transport/protocol failure or ambiguous
completion stops conservatively; the donor's retry after an initial502 is
not reproduced.

Each admitted paid tick reports cumulative measured usage through the
ordinary host ledger under kind `cache_keepalive`. Earlier snapshots survive
held cancellation once under the original operation/caller. Native cost
stays explicitly in ACU, without an inferred USD conversion. No origin
receipt is issued for background output. Diagnostics contain fixed codes
and phases only, with no raw errors, prompts, headers or credentials.

`CACHE-KEEPALIVE-EVIDENCE.md` separates historical primitive tests, SDK-peer
adapter tests and installed-host acceptance. All use synthetic local peers.
Original Cognition client behavior, vendor cache lifetime, charges and live
model availability require an explicitly authorized live check.
