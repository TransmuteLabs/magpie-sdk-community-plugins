package devin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yetone/magpie/pkg/protocolext"
)

func TestAccountBootstrapDecodesFiniteOperationsWithoutLocalAuthority(t *testing.T) {
	headers := http.Header{"Content-Type": {"application/proto"}, "Connect-Protocol-Version": {"1"}}
	for _, kind := range []AccountKind{UserStatus, TeamSettings, ManagedPlugins, ModelAssignment} {
		t.Run(string(kind), func(t *testing.T) {
			input := fixtureText(1, string(fixtureText(3, "local-only")))
			if kind == ModelAssignment {
				input = append(input, join(fixtureText(2, "model"), fixtureText(3, "session"))...)
			}
			adapter := AccountAdapter{Kind: kind}
			request, err := adapter.Decode(t.Context(), headers, input)
			if err != nil || request.Protocol != protocolext.AccountOperation ||
				request.AccountOperation != string(kind) || request.Executor != protocolext.SameModuleExecutor ||
				bytes.Contains(request.Body, []byte("local-only")) {
				t.Fatal("account operation changed native selection or leaked local credentials", err)
			}
			upstream, err := AccountPayload(kind, request.Body, "model", "selected-only")
			if err != nil || bytes.Contains(upstream, []byte("local-only")) || !bytes.Contains(upstream, []byte("selected-only")) {
				t.Fatal("account operation did not rebuild selected authority", err)
			}
			if _, err := adapter.Decode(t.Context(), headers, append(input, fixtureText(99, "unknown")...)); err == nil {
				t.Fatal("unknown native account input was silently ignored")
			}
			cancelled, cancel := context.WithCancel(t.Context())
			cancel()
			if _, err := adapter.Decode(cancelled, headers, input); err == nil {
				t.Fatal("cancelled native account operation was accepted")
			}
			if kind == ModelAssignment {
				if request.RequestID == "" || request.Session != "session" {
					t.Fatal("assignment lost its bounded replay identity")
				}
				if _, err := AccountPayload(kind, request.Body, "foreign-model", "selected-only"); err == nil {
					t.Fatal("assignment moved away from its host-selected model")
				}
			}
		})
	}
}

func TestAccountNativeResponseAndFailureBoundaries(t *testing.T) {
	for _, kind := range []AccountKind{UserStatus, TeamSettings, ManagedPlugins, ModelAssignment} {
		var body []byte
		switch kind {
		case UserStatus:
			body = fixtureText(1, string(fixtureText(7, "fixture@example.invalid")))
		case TeamSettings:
			body = fixtureNumber(1, 1)
		case ManagedPlugins:
			body = fixtureText(1, `{"plugins":[]}`)
		case ModelAssignment:
			body = fixtureText(1, string(join(fixtureText(1, "opaque"), fixtureText(2, "model"))))
		}
		if err := ValidateAccountResponse(kind, body); err != nil {
			t.Fatal(kind, err)
		}
		recorder := httptest.NewRecorder()
		reply := (AccountAdapter{Kind: kind}).Reply(recorder)
		reply.WriteHeader(http.StatusOK)
		_, _ = reply.Write(body)
		if err := reply.Finish(nil); err != nil || recorder.Code != 200 ||
			!bytes.Equal(recorder.Body.Bytes(), body) || recorder.Header().Get("Content-Type") != "application/proto" {
			t.Fatal("native account reply changed recovered wire semantics", err)
		}
	}
	recorder := httptest.NewRecorder()
	reply := (AccountAdapter{Kind: UserStatus}).Reply(recorder)
	reply.WriteHeader(403)
	_, _ = reply.Write([]byte("private response details"))
	if err := reply.Finish(nil); err != nil || bytes.Contains(recorder.Body.Bytes(), []byte("private")) || recorder.Code != 403 {
		t.Fatal("native account failure leaked private context", err)
	}
	for _, body := range [][]byte{{0}, fixtureNumber(1, 1), append(fixtureText(1, ""), fixtureText(1, "")...)} {
		if ValidateAccountResponse(UserStatus, body) == nil {
			t.Fatal("malformed native status was accepted")
		}
	}
}

func TestAssignmentIdentityDependsOnAssignmentContentAndNotInformationalMetadata(t *testing.T) {
	headers := http.Header{"Content-Type": {"application/proto"}, "Connect-Protocol-Version": {"1"}}
	adapter := AccountAdapter{Kind: ModelAssignment}
	assignment := join(fixtureText(2, "model"), fixtureText(3, "cascade"))
	input := join(fixtureText(1, string(fixtureText(3, "local-key"))), assignment)
	baseline, err := adapter.Decode(t.Context(), headers, input)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := range 8 {
		metadata := join(fixtureText(3, "another-local-key"), fixtureNumber(30, uint64(attempt)),
			fixtureText(31, "informational-fingerprint"))
		repeated, err := adapter.Decode(t.Context(), headers, join(fixtureText(1, string(metadata)), assignment))
		if err != nil || repeated.RequestID != baseline.RequestID || repeated.Session != baseline.Session ||
			!bytes.Equal(repeated.Body, baseline.Body) {
			t.Fatal("same assignment acquired a new module request/session identity", err)
		}
	}
	for _, changed := range [][]byte{
		join(fixtureText(2, "another-model"), fixtureText(3, "cascade")),
		join(fixtureText(2, "model"), fixtureText(3, "another-cascade")),
		join(assignment, fixtureText(6, "model")),
	} {
		request, err := adapter.Decode(t.Context(), headers, join(fixtureText(1, ""), changed))
		if err != nil || request.RequestID == baseline.RequestID {
			t.Fatal("changed native assignment reused an earlier write identity", err)
		}
	}
}

func TestAgentAssignmentReceiptIsSeparateFromPairedCompatibility(t *testing.T) {
	payload := fixtureRequest(join(fixtureText(26, "native-assignment"), fixtureText(16, "cascade")))
	wire, _, err := EncodeConnectRequest(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseNativeEnvelope(t.Context(), fixtureHeaders(), wire); err == nil {
		t.Fatal("default paired distribution began accepting native assignment tokens")
	}
	envelope, err := ParseAgentEnvelope(t.Context(), fixtureHeaders(), wire)
	if err != nil || len(envelope.Claims) != 1 || envelope.Claims[0].Kind != "assignment" ||
		envelope.Claims[0].Digest != SignatureDigest([]byte("native-assignment")) || envelope.Claims[0].Format != "cascade" {
		t.Fatal("agent native token bypassed origin receipt validation", err)
	}
}

func TestAccountBootstrapKnownFingerprintAndPackedDisplaysNeverSelectOrLeakAuthority(t *testing.T) {
	metadata := join(fixtureText(3, "local-key"), fixtureText(31, "opaque-local-fingerprint"),
		fixtureText(30, string([]byte{1, 2, 3})))
	headers := http.Header{"Content-Type": {"application/proto"}, "Connect-Protocol-Version": {"1"}}
	request, err := (AccountAdapter{Kind: UserStatus}).Decode(t.Context(), headers, fixtureText(1, string(metadata)))
	if err != nil || len(request.Body) != 0 {
		t.Fatal("known recovered bootstrap metadata was falsely unsupported or forwarded", err)
	}
	wire, compressed, err := EncodeConnectRequest(fixtureRequest(fixtureText(1, string(metadata))))
	if err != nil {
		t.Fatal(err)
	}
	chatHeaders := fixtureHeaders()
	if compressed {
		chatHeaders.Set("Connect-Content-Encoding", "gzip")
	}
	if _, err := ParseAgentEnvelope(t.Context(), chatHeaders, wire); err != nil {
		t.Fatal("recovered agent fingerprint metadata blocked native inference", err)
	}
	if _, err := ParseNativeEnvelope(t.Context(), chatHeaders, wire); err == nil {
		t.Fatal("agent metadata support altered default paired compatibility")
	}
	for _, extra := range [][]byte{
		fixtureText(32, "foreign-team"), fixtureText(22, "forced-team"), fixtureText(29, "impersonated-tier"),
		fixtureNumber(33, 1), fixtureNumber(31, 1), fixtureText(30, string([]byte{0x80})),
	} {
		if _, err := (AccountAdapter{Kind: UserStatus}).Decode(t.Context(), headers,
			fixtureText(1, string(join(metadata, extra)))); err == nil {
			t.Fatal("native bootstrap metadata granted account authority or accepted malformed fields")
		}
	}
}
