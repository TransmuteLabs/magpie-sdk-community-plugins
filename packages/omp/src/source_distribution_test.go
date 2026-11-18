package ompsessions

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestOrdinaryDistributionIncludesSourceAndEveryMigration(test *testing.T) {
	module := Module()
	manifest := module.Manifest
	if manifest.Contract.Version.Minor != 10 || manifest.Compatibility.HostAPI != "^1.10" ||
		!slices.Contains(manifest.Contract.Capabilities, sdk.SessionSource) ||
		!slices.Contains(manifest.Contract.Capabilities, sdk.SessionRelatedInputs) ||
		len(manifest.SessionSources) != 1 || manifest.SessionSources[0].ID != "omp-history" ||
		module.Handlers[sdk.MethodSessionSourceRead] == nil || module.Start == nil {
		test.Fatal("ordinary distribution omitted the Source 1.9 descendant reader or its read veto")
	}
	var routes []string
	for _, descriptor := range manifest.Sessions {
		routes = append(routes, descriptor.ID)
	}
	if !reflect.DeepEqual(routes, []string{"omp-to-document", "document-to-omp", "omp-to-claude",
		"omp-to-codex", "claude-to-omp", "codex-to-omp"}) {
		test.Fatal("unified distribution lost a migration route", routes)
	}
	if err := manifest.Validate(); err != nil {
		test.Fatal(err)
	}
}

func sourceArtifact(test *testing.T, id, model, answer string, input int) []byte {
	test.Helper()
	body := sourceJournal(test, message("a", "", "assistant", map[string]any{
		"model": model, "content": answer, "usage": map[string]int{"input": input, "output": 3},
	}))
	_, body, _ = bytes.Cut(body, []byte{'\n'})
	return bytes.ReplaceAll(body, []byte(sessionID), []byte(id))
}

func TestSourceReadsSuppliedArtifactJournalsWithoutFlatteningHistory(test *testing.T) {
	request := sourceRequest(test, "summary", sourceFixture(test, "omp-source-usage-v3.jsonl"))
	artifactRoot := strings.TrimSuffix(request.Locator.Path, ".jsonl")
	request.Documents = append(request.Documents,
		sdk.SessionDocument{Path: artifactRoot + "/0-Explore.jsonl", Format: OMPFormat,
			Body: sourceArtifact(test, "019a0000-0000-7000-8000-00000000001a", "child", "private child answer", 31)},
		sdk.SessionDocument{Path: artifactRoot + "/0-Explore/__advisor.jsonl", Format: OMPFormat,
			Body: sourceArtifact(test, "019a0000-0000-7000-8000-00000000002a", "advisor", "private advisor answer", 7)})
	request.Descendants = []string{request.Documents[1].Path, request.Documents[2].Path}
	before, _ := json.Marshal(request)
	result, err := ReadSessionSource(test.Context(), request)
	if err != nil {
		test.Fatal("checked related OMP JSONL artifacts were rejected", err)
	}
	if result.Summary.Title != "Recovered summary" || len(result.Summary.Usage) != 5 ||
		result.Summary.Usage[3].Model != "child" || result.Summary.Usage[3].Input != 31 ||
		result.Summary.Usage[4].Model != "advisor" || result.Summary.Usage[4].Input != 7 {
		test.Fatal("related journals lost their own model usage or replaced main metadata", result.Summary)
	}
	if !reflect.DeepEqual(result.Summary.Subs, []sdk.SessionSourceIdentity{
		{Path: request.Documents[1].Path, SessionID: "019a0000-0000-7000-8000-00000000001a"},
		{Path: request.Documents[2].Path, SessionID: "019a0000-0000-7000-8000-00000000002a"},
	}) {
		test.Fatal("child usage lost its ordinary gateway dedup identities", result.Summary.Subs)
	}
	request.Mode = "transcript"
	result, err = ReadSessionSource(test.Context(), request)
	if err != nil {
		test.Fatal(err)
	}
	wire, _ := json.Marshal(result)
	if bytes.Contains(wire, []byte("private child answer")) || bytes.Contains(wire, []byte("private advisor answer")) {
		test.Fatal("child history was flattened into the main conversation")
	}
	request.Mode = "summary"
	after, _ := json.Marshal(request)
	if !bytes.Equal(before, after) {
		test.Fatal("Source decoding changed supplied journal/artifact bytes")
	}
}

func TestSourceRejectsUnrelatedAndAliasedArtifactJournals(test *testing.T) {
	for _, name := range []string{"other/child.jsonl", "bucket/other/child.jsonl",
		strings.TrimSuffix(sourceRequest(test, "summary", nil).Locator.Path, ".jsonl") + "/btw-history/child.jsonl"} {
		request := sourceRequest(test, "summary", sourceFixture(test, "omp-source-usage-v3.jsonl"))
		request.Documents = append(request.Documents, sdk.SessionDocument{Path: name, Format: OMPFormat,
			Body: sourceArtifact(test, "019a0000-0000-7000-8000-00000000001a", "child", "child", 1)})
		request.Descendants = []string{name}
		if _, err := ReadSessionSource(test.Context(), request); err == nil {
			test.Fatal("unrelated or side-question history was accepted", name)
		}
	}
	request := sourceRequest(test, "summary", sourceFixture(test, "omp-source-usage-v3.jsonl"))
	request.Documents = append(request.Documents, sdk.SessionDocument{
		Path: strings.TrimSuffix(request.Locator.Path, ".jsonl") + "/alias.jsonl", Format: OMPFormat,
		Body: sourceArtifact(test, sessionID, "child", "aliased main journal", 1)})
	request.Descendants = []string{request.Documents[1].Path}
	if _, err := ReadSessionSource(test.Context(), request); err == nil {
		test.Fatal("the same session identity was billed twice through an artifact alias")
	}
}

func TestSourceArtifactUnknownUsageAndAggregateOverflow(test *testing.T) {
	request := sourceRequest(test, "summary", sourceFixture(test, "omp-source-usage-v3.jsonl"))
	body := sourceArtifact(test, "019a0000-0000-7000-8000-00000000001a", "child", "child", 31)
	body = bytes.Replace(body, []byte(`"usage":{"input":31,"output":3}`), []byte(`"usage":null`), 1)
	request.Documents = append(request.Documents, sdk.SessionDocument{
		Path: strings.TrimSuffix(request.Locator.Path, ".jsonl") + "/child.jsonl", Format: OMPFormat, Body: body})
	request.Descendants = []string{request.Documents[1].Path}
	result, err := ReadSessionSource(test.Context(), request)
	if err != nil || result.Summary.Usage != nil {
		test.Fatal("unknown descendant usage was reported as a complete measured total", result.Summary, err)
	}
	body = sourceArtifact(test, "019a0000-0000-7000-8000-00000000001a", "child", "child", 31)
	body = bytes.Replace(body, []byte(`"input":31`), []byte(`"input":9223372036854775800`), 1)
	request.Documents[1].Body = body
	if _, err := ReadSessionSource(test.Context(), request); err == nil {
		test.Fatal("aggregate usage overflow was accepted across individually bounded journals")
	}
}
