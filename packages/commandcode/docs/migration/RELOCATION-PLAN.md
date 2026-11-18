# CommandCode relocation plan

Preserve the audited CommandCode runtime, Go module identity, manifest capabilities,
browser auth-link UI and native package layout. Originals and parent backups remain
read-only; this author owns only packages/commandcode.

1. Verify the copied bytes against the parent's source map before editing. Keep
   all runtime Go/browser sources and fixture content unchanged.
2. Adapt scripts/build.py to --output and optional --sdk through the root public
   build_environment helper. Run Go from src, compile web assets from the package,
   bundle helpers from the selected SDK, and retain published runtime filenames.
3. Adjust relocated provenance/fixture/watch paths and test/script consumers.
   Move immutable historical receipts from tests into docs/history; retain their
   literal old identities. Update current documentation links and add a short README.
4. Run existing Go tests/race, vet, formatting checks, TypeScript/build/package and
   affected browser/native checks on regular usbox only, using a private workspace,
   temporary HOME/XDG, pinned offline caches and the existing network guard.
   Preserve fixture/hash assertions; add no dependencies or runtime behavior.
5. Record exact changed/moved paths, commands, evidence and limits in RELOCATION.md;
   freeze package source for independent review and parent monorepo validation.

No git staging, refs, commits, merges, push, live configuration or vendor calls.
