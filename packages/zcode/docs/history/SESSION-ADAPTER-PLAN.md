# Real offline SessionMigration adapter

Keep the committed SDK 1.4 headless checkpoint intact. The next local
slice uses its existing, tested history functions through a real
`session.convert` handler and the host's explicit file-plan path.

1. Register one manual offline converter descriptor. Do not declare an
   automatic agent reader/resume target or invent a live ZCode session route.
2. Accept one explicit document, matching source/target format and a
   required relative outputPath option. Support only native/legacy sources
   to the checked bundle and exact restoration to the original format.
3. Reject source overwrite, path traversal, mixed/multiple inputs,
   unknown options and unsupported semantic conversions. Keep IO and
   rollback entirely in the existing host ActionPlan owner.
4. Return a stable content-derived result ID rather than trusting an
   imported task/ticket ID as authority. No vendor call or credentials
   are needed for this offline adapter.
5. Add regressions and actual native-host preview/apply/backup/restore
   proof using isolated synthetic files on usbox.
6. SDK 1.5 selected-account UI remains a separate, unbundled prototype;
   no manifest floor change, auth-store duplication or client-selected
   provider is introduced in this 1.4 slice.
