# Fixed official remote MCP execution

Keep the SDK 1.4 runtime and separate UI 1.5 prototype unchanged in scope.
No live endpoint, dependency, publication or credential-store workaround.

Evidence:

- Recovered client-phase-1 spec §12.3 fixes the route
  `/api/v1/mcp/server/image_search`, with no trailing slash, remote tool
  `search_image` and normally stateless HTTP. New groups are not assumed.
- Recovered adapter fixture calls `search_image` with `query`.
- Recovered local protocol fixture pins `2025-06-18`; this implementation
  supports that explicit protocol cut and rejects other negotiated versions.
- Official MCP 2025-06-18 transport/lifecycle/tool specifications define
  JSON-RPC IDs, initialize/initialized, JSON/SSE responses and cancellation.
  They are protocol evidence, not proof of live vendor availability.

Implementation:

1. Pure bounded codec: JSON-RPC identity, JSON/SSE framing, notifications,
   structured errors and exact terminal result. No reconnect/replay.
2. Fixed local-owner `zcode_image_search` declaration maps only to the
   recovered endpoint/tool. Caller cannot set URL/tool/headers/protocol/IDs.
3. Resolve one selected immutable credential/proxy snapshot, verify the
   profile and use the separate official session/business JWT channel on
   every protocol request. No PAT/free/refresh fallback or redirects.
4. Initialize, notify initialized, verify the fixed tool in bounded list
   pages, then send one accepted tools/call. Preserve server session ID if
   issued; stateless absence is valid. Reject changed IDs/sessions/version.
5. Bound IO and lifetime; on cancellation close the request and send one
   best-effort cancellation notification without replaying the tool.
6. Preserve MCP content blocks/isError. Carry extra result fields in a
   namespaced text block because SDK 1.4 ToolResult exposes only content
   and isError. Do not claim full structured-output parity.
7. Synthetic regressions and actual native-host selected-account tool
   proof on usbox, then exact source/artifact hashes and local Lore commit.

Remaining dynamic tools, modern protocol eras, external unbound MCP service,
UI/auth broker and SQLite import are separate. This is not a dynamic tool
registry or full vendor parity claim.
