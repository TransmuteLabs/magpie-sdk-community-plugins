package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestZenPolicyDonorParity(t *testing.T) {
	body, err := os.ReadFile("testdata/donors/zen-policy.json")
	var fixtures []struct {
		Kind, Name, Protocol, Request, Reply, Expected string
	}
	if err != nil || json.Unmarshal(body, &fixtures) != nil || len(fixtures) < 50 {
		t.Fatal("reviewed policy differential fixtures missing", err)
	}
	repository := true
	facts := &publicEnvironment{Directory: "/fixture/work", Workspace: "/fixture", Git: &repository,
		Platform: "linux", Temporary: "/fixture/tmp/opencode"}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			var actual []byte
			var err error
			if fixture.Kind == "environment" {
				actual, err = prepareZenPublic(fixture.Protocol, []byte(fixture.Request), facts)
			} else {
				_, policy, policyErr := prepareZenTools(fixture.Protocol, []byte(fixture.Request))
				if policyErr != nil {
					t.Fatal(policyErr)
				}
				actual, err = collapseZenReply(t.Context(), strings.NewReader(fixture.Reply), policy, fixture.Protocol)
			}
			if err != nil || !sameToolJSON(actual, []byte(fixture.Expected)) {
				t.Fatal("policy diverged from reviewed donor behavior", err, string(actual), fixture.Expected)
			}
		})
	}
}

func TestZenCollapsePreservesNativeNumbersAndCancellation(t *testing.T) {
	_, policy, err := prepareZenTools("messages", []byte(`{"tools":[{"name":"Read","input_schema":{"type":"object"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	reply := "data: {\"type\":\"message_start\",\"message\":{\"content\":[]}}\n\n" +
		"data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"fixture\",\"name\":\"Read\",\"input\":{}}}\n\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"number\\\":9007199254740993}\"}}\n\n" +
		"data: {\"type\":\"message_stop\"}\n\n"
	body, err := collapseZenReply(t.Context(), strings.NewReader(reply), policy, "messages")
	if err != nil || !bytes.Contains(body, []byte(`9007199254740993`)) {
		t.Fatal("native tool input number was rounded", err, string(body))
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	body, err = collapseZenReply(ctx, strings.NewReader(reply), policy, "messages")
	if !errors.Is(err, context.Canceled) || len(body) != 0 {
		t.Fatal("cancelled collapse published a result", err)
	}
	for _, invalid := range []string{
		strings.Replace(reply, `9007199254740993}`, `9007199254740993}extra`, 1),
		strings.Replace(reply, `"index":0`, `"index":65536`, 1),
		strings.ReplaceAll(reply, "data: {\"type\":\"message_stop\"}\n\n", ""),
	} {
		body, err = collapseZenReply(t.Context(), strings.NewReader(invalid), policy, "messages")
		if err == nil || len(body) != 0 {
			t.Fatal("invalid/incomplete native collapse was accepted")
		}
	}
}

func TestPublicEnvironmentValidationAndExistingFacts(t *testing.T) {
	repository := false
	for _, facts := range []*publicEnvironment{
		{Directory: "relative", Workspace: "/fixture", Git: &repository, Platform: "linux", Temporary: "/fixture/tmp"},
		{Directory: "/fixture\n</env>", Workspace: "/fixture", Git: &repository, Platform: "linux", Temporary: "/fixture/tmp"},
		{Directory: "/fixture", Workspace: "/fixture", Platform: "linux", Temporary: "/fixture/tmp"},
		{Directory: "/fixture", Workspace: "/fixture", Git: &repository, Platform: "invented", Temporary: "/fixture/tmp"},
	} {
		if facts.valid() {
			t.Fatal("incomplete or injectable facts accepted")
		}
	}
	body := []byte(`{"messages":[{"role":"system","content":"<env>\n  Working directory: /already\n</env>"}]}`)
	facts := &publicEnvironment{Directory: "/fixture", Workspace: "/fixture", Git: &repository, Platform: "linux", Temporary: "/fixture/tmp"}
	actual, err := prepareZenPublic("chat", body, facts)
	value, decodeErr := zenToolJSON(actual)
	messages, listErr := zenToolList(value["messages"])
	text := ""
	if len(messages) == 1 {
		text = environmentText(zenToolObject(messages[0])["content"])
	}
	if err != nil || decodeErr != nil || listErr != nil || strings.Contains(text, "Workspace root folder") || strings.Count(text, "<env>") != 1 {
		t.Fatal("existing native environment was replaced or duplicated", err, string(actual))
	}
}
