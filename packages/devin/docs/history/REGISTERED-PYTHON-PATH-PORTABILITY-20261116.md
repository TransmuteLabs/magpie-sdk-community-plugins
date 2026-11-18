# Registered Python directory portability, 2026-11-16

The full-package QA run against frozen SDK05 exposed seven named distribution
subcase failures in `tests/provenance_test.py`. The selected SDK directory was
correct, but its resolved physical path differed from the fixture's `/tmp`
spelling. The failures covered the default distribution, four CLI modes, and
the two compatibility distributions. Their original log and terminal receipt
remain preserved in the verification packet.

Before changing the assertion owner, an additional regression passed a real
distribution fixture through an explicit directory symlink. The complete
registered check then reported 25 tests and eight failures: the original seven
subcases plus that alias regression, all at the original SDK path assertion.

The shared test helper now requires existing directories, resolves them
strictly, and checks physical identity with `Path.samefile`. Only the build
working-directory and selected-SDK-directory assertions use it. Distinct
directories with the same leaf name are refused in both argument orders;
regular files and missing directories are refused too. Existing exact
argv, configuration, account, version, capability, and non-directory assertions
are unchanged. The Git-reader working-directory assertion remains exact.

The old test SHA256 is
`206a9a706010d3fb625c9d4b722a0cd6077d801410b72f316fc8535eb96426e7`;
the corrected test SHA256 is
`8322d805ff42b082078b7fb66b98d7432f40c4cbfe9b4c47880bf3b66a7f6167`.
This is a test/provenance correction. Package versions remain 0.8.0, 0.5.0,
and 0.2.0 for the full, paired, and inbound variants; their SDK floors and
capabilities remain unchanged. `docs/source-lock.json` retains SHA256
`98751fc8eb91872450156693e79072b4c8c4151d1dc4afd1819e325c001b9460`.
No donor, runtime, builder, manifest, SDK, or wire contract changed.

The required verification is the complete registry command
`python -m unittest discover -s tests -p '*_test.py'`, without filtering or
skips, under an owned `/tmp` alias, its `/private/tmp` spelling, and a separate
physical control directory. Fixtures use isolated HOME/XDG, offline selection,
and the parent Mac network/configuration guard. Test data is physically stored
outside the source checkout. No Go or SDK gates need repeating for this delta.

The exact source manifests, preserved red, environment identities, guarded
terminal receipts, and authored-only patch belong to
`/Users/maratkarimov/work/.magpie-verification-20261116/packets/sol-devin-registered-portability-05-8f270117`.
The unchanged runtime manifest (all `src/`, `scripts/`, and `web/` files) has
SHA256 `275f7da38aa73a9628ee4271b8ffe16d931408588380c8f49a04e42fd8180c2d`.
Earlier reviewed runtime snapshots and whole-QA receipts keep their original
identities; the new complete package identity includes this test/doc delta.
