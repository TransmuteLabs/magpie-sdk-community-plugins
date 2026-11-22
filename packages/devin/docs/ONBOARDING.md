# Devin accounts in Providers

The complete 0.8.3 and paired 0.5.1 packages register Devin in **Providers →
Add → Subscriptions** using the SDK `provider-signin` capability. Both
variants also require the host's `provider-saved-signin` capability for the
import below. The complete package registers the `devin-native` profile in
Agents. Both variants use the standard Providers form for first sign-in and
**Sign in again** on a selected account.

- **Sign in with Devin** uses browser PKCE through the host broker.
- **Use API key** accepts an explicit key or session token in a secret field.

## Import a saved community sign-in

Keep the Community plugin installed and enabled during the transfer. Start
with the compatible SDK package installed and enabled, with the same Devin
origin and an empty SDK account list for this provider. Run import once for
each saved Community account, including paused accounts. Use the exact saved
key, such as `devin` or `devin#work`. Magpie reads and rechecks the current
record from `plugin-auth.json` under the store's writer lock.

```sh
magpie plugin sdk auth import devin-inbound devin '<saved-account-key>'
```

Import verifies the record and returns a ticket and public account summary.
The prepared account stays out of Providers and routing. Tickets expire after
ten minutes or at the credential's earlier expiry. Review each summary and
pass all tickets to a single transfer command while they are valid. For a
two-account subscription:

```sh
magpie plugin sdk auth transfer devin-inbound devin '<first-ticket>' '<second-ticket>'
magpie plugin sdk auth owners
```

Check that the receipt is active and keep its `publicID` and `revision`.
Confirm that Providers shows one Devin subscription under the same ID and
account aliases. Verify account selection, Usage, and a request through the
existing routing configuration. The saved Community records remain in place.

If the checks fail, roll back while the Community plugin is still installed
and enabled and its saved records still match the transfer. Do this before
dispatching sign-out, reauthorization or refresh for those accounts. A
refresh that the host records as `not-started` still permits rollback.

```sh
magpie plugin sdk auth rollback '<publicID>' '<revision>'
```

Check that Community serves the subscription again after rollback. Remove
the Community plugin only after the SDK checks pass and you accept the SDK as
the owner. Its installation remains necessary while rollback is an option.

Devin is a retired built-in subscription. Magpie requires `auth transfer`
to publish its prepared accounts under the public subscription ID.
The CLI-only `saved-signin` method prepares each account.

### Saved sign-in format

The adapter reads one community `type: "api"` record, in this format.

```json
{
  "type": "api",
  "key": "SYNTHETIC_DEVIN_KEY",
  "metadata": {
    "email": "fixture@example.invalid",
    "orgId": "example-organization",
    "teamId": "example-team",
    "plan": "Example plan"
  }
}
```

An explicit `metadata.server` must match the configured origin. Its absence
means `https://server.codeium.com`. Magpie asks that origin for `GetUserStatus`
with the supplied key. Import requires a usable user ID in the response.
The status also supplies the account label.

### CLI-linked records

Community records with `metadata.cli: true` or `"true"` link to the official
Devin CLI sign-in. The saved record identifies a credential and can carry
tenant metadata. The community format has no user ID. Import asks
`GetUserStatus` for the saved credential and the key in the CLI's
`credentials.toml`. Both must prove the same user, team and organization on
the configured API origin. Saved team and organization IDs, if present, must
agree. If the saved key is revoked or either identity is incomplete, the
owner can use browser or API-key sign-in.

Community can fill its saved `orgId` from the team ID. CLI-linked import
requires `GetUserStatus` to return an explicit organization ID.

Publication stores the verified CLI key in the new SDK account generation.
`magpie plugin sdk auth refresh devin-inbound devin <account-id>` reads the
CLI file again and checks the stored subject on the same origin before
publishing a replacement generation. A missing or changed CLI account
leaves the last verified generation in service. Inference uses that
generation. The CLI file has no usable expiration timestamp, so following
changes requires the explicit account refresh. The credential loader accepts
the flat fields written by the CLI and treats a conflict with
`WINDSURF_API_SERVER_URL` as a server mismatch.

## Account identity

Verified server, user ID, team ID and organization ID determine the new account
ID. Labels use the verified email or name and the organization. In an ordinary
browser or API-key sign-in without a usable user ID, the key fingerprint
remains the account ID. Replacing a key in an existing fingerprint slot
requires verifying the old key's subject against the new one. Reauthentication
preserves the chosen slot. An explicit refresh checks the same subject and
updates the label. Sign-out removes the selected generation from Magpie even
after the configured server changes. The module checks the saved slot for a
verified binding. Legacy fingerprint accounts have no saved origin, so their
selection uses the host's exact account and generation. Refresh and inference
still require the credential's original origin. Devin handles vendor
credential issuance and revocation.

## Browser ownership

The [callback owner](../src/provider_auth_callback.go) binds to `127.0.0.1` and
validates the callback method, host, path, state and deadline. It keeps the
received code in the component's memory. The
[auth handler](../src/provider_auth.go) exchanges it once during the broker's
completion poll. The host publishes the account only while the package,
configuration, selected account and discovery policy are current.
CLI callers can supply their own redirect and complete it with the callback
URL. Canceling a broker flow prevents further completion. The module's
loopback receiver closes on completion, expiry or component disposal.

## Verification

[`window_auth_host.py`](../tests/window_auth_host.py) installs one real native
distribution in an isolated host. It uses synthetic credentials through the
ordinary Providers HTTP routes and the saved sign-in CLI. It covers
preparation, the refusal to activate a single saved account, incomplete status, distinct
identities with the same label, preservation of both credential files,
verified CLI-linked preparation, selected logout after a server change,
callback replay, stale config and disabled discovery.
[`provider_single_package_e2e.py`](../tests/provider_single_package_e2e.py)
also checks selected CLI sign-out after changing the configured server.
Run it on `usbox` under the network guard with a Magpie build that supports
`provider-saved-signin`. Supply `--paired-only` for a paired distribution.

```sh
python3 tests/window_auth_host.py --host /path/to/magpie \
  --distribution /path/to/devin-distribution --output /tmp/devin-signin-results
```

The [account tests](../src/provider_identity_test.go) separately exercise
identity and import rejection. The
[subscription transfer fixture](../tests/subscription_transfer_e2e.py) exercises
transfer of three saved accounts, partial-transfer refusal, preservation of
the public ID and aliases, ordinary gateway requests, and rollback. It then
rotates one credential and transfers again. After changing the configured
server, the Providers route refuses the stale owner and generation, signs
out just the selected account, and preserves both source credential files.
The remaining account still serves through the original gateway route. The
fixtures use a local mock, and live Devin sign-in remains unverified.
