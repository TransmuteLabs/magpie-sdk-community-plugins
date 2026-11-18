package ompsessions

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func sdkFixture(t *testing.T, format string) sdk.SessionRequest {
	t.Helper()
	journal, err := os.ReadFile("testdata/omp-v3.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	asset, err := os.ReadFile("testdata/image.bin")
	if err != nil {
		t.Fatal(err)
	}
	return sdk.SessionRequest{ID: "omp-to-document", From: format, To: DocumentFormat,
		Files: []sdk.SessionDocument{
			{Path: "sessions/fixture.jsonl", Format: format, Body: journal},
			{Path: "blobs/" + hashBytes(asset), Format: format, Body: asset},
		}, Options: json.RawMessage(`{"output":"conversation.json"}`)}
}

func TestPublicSDKConvertAliasesAndLosslessOutputs(t *testing.T) {
	for _, format := range []string{"omp", OMPFormat} {
		t.Run(format, func(t *testing.T) {
			request := sdkFixture(t, format)
			journalBefore, assetBefore := bytes.Clone(request.Files[0].Body), bytes.Clone(request.Files[1].Body)
			result, err := Convert(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			if result.ID != sessionID || len(result.Files) != 1 || result.Files[0].Format != DocumentFormat ||
				result.Files[0].Path != "conversation.json" {
				t.Fatalf("SDK document result: %+v", result)
			}
			exported, err := Convert(t.Context(), sdk.SessionRequest{
				ID: "document-to-omp", From: DocumentFormat, To: format, Files: result.Files,
				Options: json.RawMessage(`{"output":"sessions/--work-omp--"}`),
			})
			if err != nil {
				t.Fatal(err)
			}
			if exported.ID != sessionID || len(exported.Files) != 2 ||
				!bytes.Equal(exported.Files[0].Body, journalBefore) || !bytes.Equal(exported.Files[1].Body, assetBefore) ||
				exported.Files[0].Format != format || exported.Files[1].Format != format ||
				!strings.HasPrefix(exported.Files[0].Path, "sessions/--work-omp--/") ||
				exported.Files[1].Path != request.Files[1].Path {
				t.Fatalf("journal/blob checked outputs: %+v", exported)
			}
			if !bytes.Equal(request.Files[0].Body, journalBefore) || !bytes.Equal(request.Files[1].Body, assetBefore) {
				t.Fatal("conversion changed its input byte snapshots")
			}
		})
	}
	if err := Module().Validate(); err != nil {
		t.Fatal("installable native module:", err)
	}
	manifest := Manifest()
	if manifest.Runtime != "native" || manifest.Contract.Version != (sdk.APIVersion{Major: 1, Minor: 10}) ||
		!manifest.Contract.Has(sdk.SessionMigration) || !manifest.Contract.Has(sdk.SessionSource) {
		t.Fatalf("minimum contract changed: %+v", manifest)
	}
}

func TestConvertRejectsUnsafeIncompleteOrUnknownRequests(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*sdk.SessionRequest)
	}{
		{"absolute output", func(r *sdk.SessionRequest) { r.Options = json.RawMessage(`{"output":"/tmp/no"}`) }},
		{"parent output", func(r *sdk.SessionRequest) { r.Options = json.RawMessage(`{"output":"../no"}`) }},
		{"windows output", func(r *sdk.SessionRequest) { r.Options = json.RawMessage(`{"output":"C:\\no"}`) }},
		{"reserved output", func(r *sdk.SessionRequest) { r.Options = json.RawMessage(`{"output":".magpie-extension-state/no"}`) }},
		{"unknown option", func(r *sdk.SessionRequest) { r.Options = json.RawMessage(`{"output":"out.json","ignoreMedia":true}`) }},
		{"source traversal", func(r *sdk.SessionRequest) { r.Files[0].Path = "../source" }},
		{"asset traversal", func(r *sdk.SessionRequest) { r.Files[1].Path = "blobs/../source" }},
		{"duplicate input", func(r *sdk.SessionRequest) { r.Files = append(r.Files, r.Files[0]) }},
		{"empty extra main", func(r *sdk.SessionRequest) {
			r.Files = append([]sdk.SessionDocument{{Path: "empty.jsonl", Format: r.From}}, r.Files...)
		}},
		{"missing asset", func(r *sdk.SessionRequest) { r.Files = r.Files[:1] }},
		{"wrong format", func(r *sdk.SessionRequest) { r.Files[0].Format = "claude" }},
		{"wrong route", func(r *sdk.SessionRequest) { r.To = "claude" }},
		{"wrong descriptor", func(r *sdk.SessionRequest) { r.ID = "another-module" }},
		{"no files", func(r *sdk.SessionRequest) { r.Files = nil }},
		{"too many files", func(r *sdk.SessionRequest) {
			for range MaxFiles {
				r.Files = append(r.Files, r.Files[0])
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := sdkFixture(t, "omp")
			test.edit(&request)
			if _, err := Convert(t.Context(), request); err == nil {
				t.Fatal("unsafe or incomplete request was accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Convert(ctx, sdkFixture(t, "omp")); err != context.Canceled {
		t.Fatalf("lost cancellation: %v", err)
	}
	if _, err := Handler(t.Context(), sdk.Call{Params: json.RawMessage(`{"unexpected":true}`)}, nil); err == nil {
		t.Fatal("handler accepted malformed public parameters")
	}
}
