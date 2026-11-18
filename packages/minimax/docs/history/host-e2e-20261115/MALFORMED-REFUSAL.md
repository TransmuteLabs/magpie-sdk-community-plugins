# MM-E2E-1 — malformed vendor codes acquire auth/quota classifications (P2)

Confirmed through the ordinary host CLI and the unmodified production native
distribution dist-v2, library SHA-256
df7c26c7b76c8d47705f7f6d9b670d91cb1e25382c58a0498f5822b4dc4c832f.

Trigger: an HTTP 200 JSON base_resp uses a fractional nonzero status_code,
or an HTTP 403 application/json body is malformed but contains quota wording.
These are synthetic semantic inputs, not captured vendor responses.

| Input | Observed module HTTP status | Vendor dispatches |
| --- | ---: | ---: |
| base_resp.status_code = 1008.5 | 429 | 1 |
| base_resp.status_code = 1000048.5 | 401 | 1 |
| base_resp.status_code = "1008.5" | 402 | 1 |
| HTTP 403 with `{ "message": "insufficient credit"` lacking closing brace | 429 | 1 |

In the frozen source, client.go:321 and client.go:330 truncate parsed numbers
with int(code) before envelopeError; failure.go:62–63 also truncate numeric
codes. failure.go:41 marks BodyComplete before JSON parsing and :46–48 treats
invalid JSON as free-form message facts. The quota matcher can consequently
promote malformed JSON into a recognized quota refusal.

Impact: the public module response reports auth/quota authority unsupported by
the vendor facts. No replay or actual account mutation was observed in this
probe; the existing gateway accepted-operation 409 fence is separate and must
remain intact. This does not claim a request.recover implementation exists.

Evidence on regular usbox:
/tmp/magpie-minimax-host-e2e.fhLVWFGt/classification-v2/{result,commands,fixture}.json,
classification-v2-receipt.json and classification-v2-network.json. The guarded
run used fresh isolated HOME, private-CA TLS CONNECT and the actual production
module. tests/host_e2e.py --classification-only retains all four assertions for
independent closure. Reported to core owner Archimedes; no production edit by
this reviewer.

Independent closure: ACCEPT on dist-v4, library
4ae1687285445c4570e1c08db7c6be229ad775969e57e3344f66cf0e2aedcc38.
The same four classification inputs and assertions now observe 502, 502, 502
and 403, with exactly one dispatch each. classification-v4-final exits 0.
The fixture's own local GET / handling changed separately to record that
request and return 404; the malformed input cases/assertions did not change.
Core correction uses canonical integral codes and prevents malformed JSON
from becoming quota facts. Final source was reviewed against the author's
35-file freeze. The original failing evidence remains unchanged.
