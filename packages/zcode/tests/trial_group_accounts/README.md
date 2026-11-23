# ZCode Trial group account isolation

This fixture uses the ordinary Magpie Messages route, an installed executable ZCode SDK module and a synthetic loopback vendor. The host test installs one module and saves two accounts with distinct stable IDs, paid credentials, Trial credentials and devices. Each account discovers its models through the adapter and fetches its own paid quota and Trial balance. The group uses aliases and explicit account-qualified members.

| Group order | Expected behavior |
| --- | --- |
| A Trial | A's named Trial reserve is full. Return 429 without sending inference. |
| A Trial → B Trial | B's reserve remains open. Send `GLM-5.3` to Start with B's Trial bearer token and no API key. |
| A Trial → A Preview → B Trial | A's Preview remains open. Send `glm-limited-preview` to Start with A's Trial bearer token and no API key. |
| B Paid → A Paid → B Trial | B's paid usage window is full. Send `GLM-5.3` to the paid endpoint with A's paid token in both authorization headers. |

Both accounts' primary Trial usage is 25%. The separate reserve is shared only by the native `GLM-5.3` Trial model, with 0/25 units available on A and 10/25 on B. A and B's paid quota readings are 20% and 100%. The test verifies fresh quota requests with each account's credentials and device before checking all four routes. Positive cases must return the fixture's response and send exactly one upstream request. Missing endpoints and incorrect quota headers fail the fixture. The original one-account fixture covers an explicitly shared Preview reserve.

## Offline run

Run only on the regular Linux `usbox` in an exclusive slot. Use repositories containing these exact Git objects:

- Magpie `947d03d554ba0855001fa4ca315df1c08d3f584f`, tree `86ea14ea48e579646eb589659eba0ce0f8582542`.
- SDK companion `afe1cd6a4c5b8ee65e596c9ec7a8bdb33bad7206`, tree `494960fb82c42f8b4e830cfa19b40d278d6a68e8`.

```sh
bash packages/zcode/tests/trial_group_accounts/run-linux.sh \
    /path/to/magpie.git /path/to/sdk.git /absolute/new-receipt
```

The runner uses the pinned offline Go 1.26.8 caches, a private home and loopback-only network namespace. It archives the pinned commits into scratch, installs the host-side test only there, and builds the worker against the archived SDK source. It runs the exact group test three times under the race detector and vets the host gateway package. It records raw results, source hashes and process boundaries. The outer deadline is 300 seconds with a five-second kill grace.
