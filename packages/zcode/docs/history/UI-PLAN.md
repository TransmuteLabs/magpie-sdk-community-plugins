# Selected-account control page

Build a separate native SDK 1.5 distribution against the reviewed MAIN
contract at `c9dd41e12`. Keep the default headless SDK 1.4 artifact and its
runtime manifest unchanged. No auth buttons until the host-owned auth
broker is available; no credentials/configured account tokens in HTML.

1. Promote the already-tested typed state layer into `web/control.ts`.
   Retain a prototype re-export for its existing state regressions.
2. Add a static TS/HTML/CSS page with host-owned account enumeration and
   explicit current-generation selection. Expose only existing fixed
   execution/billing/reset actions. Financial writes require an explicit
   confirmation in the page; no automatic retry or optimistic success.
3. Add a separate native entry/manifest generator mode. Bind provider
   `zcode` in declarations, never client arguments. Build assets from the
   actual public browser SDK without a Bun worker or new dependency.
4. Verify state/DOM behavior, manifest authority, asset packaging and
   actual native-host page/account/action routes with synthetic fixtures
   on usbox. Keep static/fake-bridge proof distinct from browser and host
   route proof. Preserve exact source/artifact/host hashes.
5. Document remaining auth-broker/visual/platform limits and commit this
   coherent slice locally without publication or installed activation.
