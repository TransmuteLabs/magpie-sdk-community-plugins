package zcode

import (
	"encoding/json"
	"testing"
)

func nativeTestProjection(kind string, from, to uint64, payload any) json.RawMessage {
	body, _ := json.Marshal(map[string]any{
		"topic": "conversation/session", "subscriptionId": "subscription", "fromSeq": from, "toSeq": to,
		"payload": map[string]any{"kind": kind, kind: payload},
	})
	return body
}

func TestNativeProjectionRequiresOwnedInitialBeforeCurrentGuardedStop(t *testing.T) {
	wire := &nativeWire{request: nativeRunRequest{SessionID: "session", CommandID: "command"},
		subID: "subscription", epoch: "epoch"}
	snapshot := map[string]any{"sessionId": "session", "logEpoch": "epoch", "seq": 1,
		"control": map[string]any{"canStop": false, "activeWorks": []any{}}}
	if err := wire.projection(nativeTestProjection("snapshot", 0, 1, snapshot)); err != nil || !wire.initial {
		t.Fatal("owned initial snapshot was rejected", err)
	}
	wire.sent = true
	control := map[string]any{"canStop": true, "activeWorks": []any{
		map[string]any{"kind": "primaryTurn", "foregroundExecutionId": "execution", "sourceCommandId": "command"},
	}}
	deltas := []any{map[string]any{"op": "state.updated", "patch": map[string]any{"control": control}}}
	if err := wire.projection(nativeTestProjection("deltas", 1, 2, deltas)); err != nil || wire.execution != "execution" {
		t.Fatal("current native command lost its guarded stop identity", err)
	}
	control = map[string]any{"canStop": false, "activeWorks": []any{}}
	deltas = []any{map[string]any{"op": "state.updated", "patch": map[string]any{"control": control}}}
	if err := wire.projection(nativeTestProjection("deltas", 2, 3, deltas)); err != nil || wire.execution != "" {
		t.Fatal("control replacement retained a stale native stop target", err)
	}
	if err := wire.projection(nativeTestProjection("deltas", 1, 2, deltas)); err == nil {
		t.Fatal("stale native projection was accepted")
	}
}

func TestNativeProjectionRejectsForeignEpochSessionSubscriptionAndGap(t *testing.T) {
	for _, change := range []string{"epoch", "session", "subscription", "gap", "missing-control", "existing-execution"} {
		t.Run(change, func(t *testing.T) {
			wire := &nativeWire{request: nativeRunRequest{SessionID: "session", CommandID: "command"},
				subID: "subscription", epoch: "epoch"}
			snapshot := map[string]any{"sessionId": "session", "logEpoch": "epoch", "seq": 1,
				"control": map[string]any{"canStop": false, "activeWorks": []any{}}}
			switch change {
			case "epoch":
				snapshot["logEpoch"] = "foreign"
			case "session":
				snapshot["sessionId"] = "foreign"
			case "subscription":
				wire.subID = "foreign"
			case "missing-control":
				delete(snapshot, "control")
			case "existing-execution":
				snapshot["control"] = map[string]any{"canStop": true, "activeWorks": []any{
					map[string]any{"kind": "primaryTurn", "foregroundExecutionId": "another", "sourceCommandId": "previous"},
				}}
			case "gap":
				wire.initial, wire.seq = true, 10
				if wire.projection(nativeTestProjection("deltas", 11, 12, []any{})) == nil {
					t.Fatal("native sequence gap was accepted")
				}
				return
			}
			if wire.projection(nativeTestProjection("snapshot", 0, 1, snapshot)) == nil {
				t.Fatal("foreign or unowned native snapshot was accepted", change)
			}
		})
	}
}

func TestNativeEnvelopesDoNotTurnNotificationsOrErrorsIntoReplies(t *testing.T) {
	for _, raw := range []string{
		`{"method":"event","result":{}}`,
		`{"id":"rpc","result":{},"error":{}}`,
		`{"id":"rpc","method":"request","error":{}}`,
		`{"id":null,"result":{}}`,
		`{"id":true,"result":{}}`,
		`{"id":[],"result":{}}`,
		`{"id":1.5,"result":{}}`,
		`{"result":{}}`,
	} {
		var frame nativeFrame
		if json.Unmarshal([]byte(raw), &frame) != nil || validNativeFrame(frame) {
			t.Fatal("invalid native envelope was accepted", raw)
		}
	}
	if !validNativeFrame(nativeFrame{Method: "notification"}) ||
		!validNativeFrame(nativeFrame{ID: json.RawMessage(`"rpc"`), Result: json.RawMessage(`{}`)}) ||
		!validNativeFrame(nativeFrame{ID: json.RawMessage(`42`), Method: "interaction/requestProviderRuntimeHeaders"}) {
		t.Fatal("ordinary native reply/notification was lost")
	}
}

func TestNativeAckIsNotAnUnrelatedOrUnknownCommandResult(t *testing.T) {
	for _, raw := range []string{
		`{"commandId":"another","status":"accepted"}`,
		`{"commandId":"command","status":"rejected"}`,
		`"unknown"`,
		`{}`,
	} {
		if nativeAck(json.RawMessage(raw), "command") {
			t.Fatal("unproved native acknowledgment was accepted", raw)
		}
	}
	if !nativeAck(json.RawMessage(`{"commandId":"command","status":"accepted","revisionAtDecision":1}`), "command") {
		t.Fatal("owned native command acknowledgment was lost")
	}
}
