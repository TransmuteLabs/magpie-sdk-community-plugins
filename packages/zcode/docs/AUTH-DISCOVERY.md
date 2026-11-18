# Headless authentication and discovery

The first verified module checkpoint is commit `c8acfa2`.
This next bounded lane changes vendor behavior only; SDK floor stays 1.3.

## Source and limits

Donors remain read-only. The archived runtime implements CLI init/poll,
callback exchange, paid selection, project-key discovery and provider
configuration rules. Recovered `Agents/ZCode` sources confirm:

- `packages/services/src/oauth/oauthService.ts`: init/poll and selected
  session-generation guards.
- `oauthService.ts:1172`: refresh is rejected when a provider adapter has no
  `refreshToken` implementation.
- `packages/services/src/oauth/providers/{zai,bigmodel}ProviderAdapter.ts`:
  neither supplies an automatic Start-session refresh exchange.

Therefore Start renewal uses explicit headless reauthorization through
`auth.start`/`auth.finish`, bound to the same selected account and generation.
No access token is substituted for a one-time callback code, and no refresh
endpoint is invented. All verification uses synthetic endpoints and tokens.

## Rules

- Preserve `snapshot` import as an independent method.
- Browser authorization is returned to the caller; the module never opens a
  browser or the installed ZCode app.
- Continuation contains only an unguessable opaque ID. Poll credentials,
  ready material and completion receipt remain in private scoped SDK state.
- CAS serializes polling/completion across processes. Consumed one-time
  exchanges cannot be repeated after an uncertain result.
- `auth.finish` returns Pending for a pending/temporarily unavailable poll.
  Poll interval and deadline are honored.
- A completed receipt may be returned again without another vendor
  exchange. Selected-account reauthorization rejects another vendor user,
  selection or credential generation.
- Explicit Personal/Team/Start selection is preserved. Ambiguous Team
  choices are refused until organization/project/product are specified.
- Business, OAuth bootstrap, session JWT and PAT lifetimes remain distinct.
- Project/key discovery uses the existing exact coding-plan key identity;
  a new key is created only during explicit selected paid sign-in.
- Fresh authoritative entitlement/provider configuration controls discovered
  models. Failed discovery is unknown, not an empty catalog or an invented
  static entitlement. Legacy manual import keeps its declared fallback.
- Apply provider model rules in recovered order. Unsupported regexp/schema
  semantics fail explicitly rather than silently dropping a rule.

## Acceptance

1. Init/pending/ready/declined/expired flow; no token in public continuation.
2. CAS contention, interval, lost completion/publication and no exchange
   replay; manual snapshot still works.
3. Start sign-in and explicit same-user renewal; wrong user/generation
   refused; no unsupported silent-refresh claim.
4. Personal and Team key discovery, exact auth forms and no permanent-key
   secret copying; distinct credential lifetimes.
5. Start live/pending/expired/spent buckets, server Date and user-plan
   identity; paid subscription and Team membership distinctions.
6. Ordered config rules, explicit null clearing, model properties/efforts,
   malformed/unavailable discovery and unsupported patterns.
7. Native host CLI init/poll/finish/models with fake vendor only, isolated
   HOME and no installed application.
