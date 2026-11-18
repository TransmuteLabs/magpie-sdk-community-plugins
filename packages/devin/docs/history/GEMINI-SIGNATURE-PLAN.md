# Gemini 0.1.41 extraction

Read-only source is community commit
`191d94bf6ed95ccc4bc62d77e1dd9201a17f5918`, not its local checkout.
The genuine 1451-byte compressed Connect fixture is retained byte-for-byte
after reviewing its decompressed fields. Native prompt fields12/18/22 and
response fields10/21 are independently established by the recovered
devinapp descriptors and the donor fixture.

Before production changes, run an ordinary SDK-peer regression with the
actual donor reply for Chat and Messages, streaming and buffered. The next
request must carry the exact signature/type/model on the tool-call prompt,
preserve the real result, and contain no invented thinking.

File boundaries:

- `codec/gemini_thought.go` collects a complete reply and restores a matching
  assistant turn after ordinary strict request validation. Match exact text,
  native call identities, names and canonical JSON arguments, rather than
  the donor's global call-only or whitespace-free text lookup.
- `provider_gemini.go` owns a bounded, credential-free memory cache scoped
  to selected account, generation, caller and model. Existing origin receipts
  must authorize every restored or explicitly supplied signature.
- `codec/outbound_request.go` accepts only the checked Gemini
  reasoning_details shape through an explicit scoped resolver; unsigned
  compatibility entry points cannot accept signed canonical history.
- `provider_execute.go`, `provider_module.go` and `outbound/messages.go`
  integrate successful completion, origin persistence and Gemini signatures
  without fabricating a Claude thinking block or retaining an SDK stream.
- Dedicated donor-oracle, scope, cancellation/failure and capacity tests
  protect the port. Existing non-Gemini and keepAlive tests remain in scope.

Do not copy the donor's unscoped global cache, infer a missing signature type,
reuse a signature across models, add a retry, or describe this as a proven fix
for live empty responses. A restart/eviction/unknown reply remains unsigned;
explicit foreign signed history fails before acceptance.
