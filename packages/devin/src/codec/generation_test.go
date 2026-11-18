package devin

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestRecoveredCompletionConfiguration(t *testing.T) {
	configuration := join(
		fixtureNumber(1, 1), fixtureNumber(2, 4096),
		fixtureDouble(5, 0.5), fixtureDouble(8, 0.9),
		fixtureText(9, "first-stop"), fixtureText(9, "second-stop"),
		fixtureNumber(10, 42), fixtureText(17, "priority"),
	)
	output := map[string]any{}
	if err := decodeSampling(configuration, output); err != nil {
		t.Fatal(err)
	}
	if output["n"] != 1 || output["max_tokens"] != uint64(4096) || output["seed"] != int64(42) || output["service_tier"] != "priority" {
		t.Fatalf("recovered controls lost: %#v", output)
	}
	patterns, ok := output["stop"].([]string)
	if !ok || len(patterns) != 2 || patterns[0] != "first-stop" || patterns[1] != "second-stop" {
		t.Fatalf("native stop order lost: %#v", output["stop"])
	}
}

func TestRecoveredUnsupportedControlsAreNamed(t *testing.T) {
	for _, test := range []struct {
		name    string
		payload []byte
	}{
		{"max_newlines", fixtureNumber(3, 400)},
		{"top_k", fixtureNumber(7, 40)},
		{"first_temperature", fixtureDouble(6, 0.5)},
		{"last_message_is_partial", fixtureNumber(15, 1)},
		{"return_logprob", fixtureNumber(16, 1)},
		{"response_format_json_schema", fixtureText(19, `{"type":"object"}`)},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := decodeSampling(test.payload, map[string]any{})
			if err == nil || !strings.Contains(err.Error(), test.name) {
				t.Fatalf("native semantic control disappeared: %v", err)
			}
		})
	}
	if err := decodeSampling(fixtureNumber(1, 2), map[string]any{}); err == nil {
		t.Fatal("single-response adapter accepted multiple completions")
	}
	if err := decodeSampling(fixtureNumber(10, math.MaxUint64), map[string]any{}); err == nil {
		t.Fatal("native unsigned seed overflowed canonical representation")
	}
}

func TestRecoveredInactiveControlsKeepProtoDefaults(t *testing.T) {
	configuration := join(
		fixtureNumber(3, 0), fixtureNumber(7, 0), fixtureNumber(15, 0),
		fixtureDouble(4, 0), fixtureDouble(6, math.Copysign(0, -1)),
	)
	if err := decodeSampling(configuration, map[string]any{}); err != nil {
		t.Fatal("inactive native defaults became an unsupported request:", err)
	}
}

func TestRecoveredSessionTrajectoryAndPlanner(t *testing.T) {
	reference := join(fixtureText(1, "trace-fixture"), fixtureNumber(2, 3), fixtureNumber(3, 4), fixtureNumber(4, 14))
	body := fixtureFrame(0, fixtureRequest(
		fixtureText(15, string(reference)), fixtureText(16, "cascade-fixture"),
		fixtureNumber(20, 3), fixtureText(28, "native-session-fixture"),
	))
	request, err := (Adapter{}).Decode(context.Background(), fixtureHeaders(), body)
	if err != nil {
		t.Fatal(err)
	}
	var canonical map[string]any
	if err := json.Unmarshal(request.Body, &canonical); err != nil {
		t.Fatal(err)
	}
	if request.Session != "native-session-fixture" || canonical["tool_choice"] != "none" {
		t.Fatalf("native session or no-tool mode lost: %+v %s", request, request.Body)
	}
	metadata := canonical["metadata"].(map[string]any)
	if metadata["devin_cascade_id"] != "cascade-fixture" || metadata["devin_trajectory_id"] != "trace-fixture" {
		t.Fatalf("correlation metadata lost: %#v", metadata)
	}
}

func TestRecoveredPlannerConstraintsRefuseConflicts(t *testing.T) {
	for _, body := range [][]byte{
		fixtureRequest(fixtureNumber(20, 2)),
		fixtureRequest(fixtureNumber(20, 3), fixtureText(12, string(fixtureText(1, "auto")))),
		fixtureRequest(fixtureText(15, string(fixtureNumber(5, 1)))),
		fixtureRequest(fixtureText(28, "invalid\r\nsession")),
	} {
		if _, err := (Adapter{}).Decode(context.Background(), fixtureHeaders(), fixtureFrame(0, body)); err == nil {
			t.Fatal("native policy or invalid session accepted")
		}
	}
}
