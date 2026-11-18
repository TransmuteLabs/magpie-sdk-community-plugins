# Ordinary-host MiniMax fixture plan

This test uses the production native distribution unchanged. The ordinary
host installs it into a fresh private HOME and supplies its configured HTTP
proxy through the public SDK callback. A local CONNECT server terminates TLS
using a private CA supplied through SSL_CERT_FILE. It never forwards traffic.
No Go wrapper, endpoint override, Bun or sibling plugin is installed.

The agreed constructor is NewModule(ModuleOptions{}). Core owner Archimedes
and tools owner Gauss retain production scope. All execution is on regular
usbox with the existing external syscall network guard. The immutable SDK
is /tmp/magpie-astra-combined-20261115-v4/source; the host executable is
/tmp/magpie-monorepo-final-20261115/magpie. Final receipts record actual hashes.

Bounded scenarios:

- CN/global device authorization, PKCE, pending poll, saved account identity,
  foreign-provider continuation refusal and consumed continuation refusal.
- Selected-account model discovery, search, named tool execution and rejection
  of a foreign site argument before vendor dispatch.
- Actual gateway buffered Chat and SSE, fragmented tool arguments, refusal
  without paid-request replay, credential redaction and caller cancellation.
- Image generation returning its URL; video creation followed by actual
  content transfer, with exact bytes and no account credential on the transfer.
- Ordinary host request/usage records for selected regional ownership, token
  totals and caller identity; native library mapping proves the installed
  production artifact actually ran.

Additional requested failure cases distinguish HTTP 403 permission from credit
quota, HTTP 200 nonzero base_resp business refusals in chat and an accepted
tool, and partial SSE followed by EOF without message_stop. They assert no
replay and exactly one accounting receipt. These responses are synthetic
semantic fixtures, not captured vendor responses. Chat expectations were
aligned directly with the core owner's old-red regression and correction:
code 1008 becomes quota 429; unknown nonzero 2013 becomes 502. No case assumes
the proposed request.recover callback exists.

Retain failed attempt receipts. Resolve fixture mistakes only in this script;
send product failures to their owners. Final execution waits for the owners'
frozen distribution. Generic install/remove lifecycle coverage is parent-owned.
These synthetic Linux cases do not establish live vendor, browser, macOS,
Windows, complete tool catalog or all media-format readiness.
