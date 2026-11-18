# Devin consumer verification on the consolidated host

Host source: committed `magpie/devzeus`
`d07078c7926daf6f33af09c40297c941bd39da92`, tree
`ed28dfd65eca68d15e9400b56660bbbf5e7bc319`.
The leader's SDK-alone freeze is tree
`f536d8202e40aae4b0fc36630aa3c1eb496d58d5`, documentation/tests only
over d070. Tested runtime and SDK remain d070.
Keep this module's implementation in its existing repository; read and export
the host from `../magpie`. Do not revive the detached nativeops worktree.

1. `agent_http_e2e.py`: discover the synthetic account model, stop the
   ordinary gateway, then select it through fresh ordinary CLI processes.
   Require the selected binding and no further vendor calls or probes.
   Restart the gateway and retain bootstrap, assignment/no-replay,
   session/resume, cancellation, single-bundle and disconnect checks.
   Keep failed bootstrap/assignment results. Record response errors, binding
   snapshots and actual mock assignment dispatch counts; never retry writes
   or wait for a quiet binding to turn a failed no-replay gate green.
   `paired_http_e2e.py`: exercise all six native catalog endpoints and
   statuses through the actually installed library with a model-restricted
   key. Check per-key aliases, the discovered alias's scoped token budget,
   read-only denial for both discovery and inference, an expired unary deadline
   and the existing upstream deadline/disconnect cancellation checks.
   `http_e2e.py`: place TMPDIR inside the fixture HOME so the consolidated
   host's verified native-image snapshots remain isolated and the existing
   mapped-library SHA256 assertion still checks the actual loaded image.
2. `source-lock.json`, `SOURCES.md`: pin the full consolidated commit and
   its actual built artifact. Watch persistent catalog authority and the
   lowercase ASCII account-operation ID contract alongside existing
   lease/generation/receipt checks.
3. `CONTRACT-BLOCKERS.md`, `outbound/HOST-GAPS.md`, `README.md`: distinguish
   the newly checked cold CLI selection from remaining delegation, literal
   Basic credential recognition and post-reply scheduling limitations.
   Report confirmed duplicate assignment dispatch and unstable bootstrap as
   current generic host defects, with exact lifecycle/journal seams for the
   leader. Keep module runtime unchanged while reconciling these documents.
4. `UNION-EVIDENCE.md`: record exact frozen inputs, offline usbox commands,
   bounded module and actual-host results, artifact hashes and residual
   proof limits. Preserve earlier 9c evidence as historical.

Use the existing usbox caches and isolated HOME/XDG with bounded timeouts,
GOTOOLCHAIN=local, GOPROXY=off and GOSUMDB=off. No new dependencies, vendor
requests, real credentials, host edits, broad host suite, branches or pushes.
The unmodified reconstructed application remains distinct from the original
Cognition client.
