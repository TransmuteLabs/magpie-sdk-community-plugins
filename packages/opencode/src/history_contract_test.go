package opencode

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestHistoryRootGrantIsNotSuppliedByFiniteJSONLSource(t *testing.T) {
	module := NewAgentModule(ModuleOptions{})
	module.Manifest.Contract.Capabilities = append(module.Manifest.Contract.Capabilities, sdk.SessionSource)
	module.Manifest.SessionSources = []sdk.SessionSourceDescriptor{{
		ID: "history-probe", Agent: AgentID, Codec: "history-probe", Container: "jsonl",
		Stores:     []sdk.SessionSourceStore{{Slot: "data", HomeRelativePath: ".local/share/opencode"}},
		Layout:     sdk.SessionSourceLayout{Kind: "flat", IDFrom: "filename-stem"},
		Transcript: true, Resume: "none",
	}}
	module.Handlers[sdk.MethodSessionSourceRead] = func(context.Context, sdk.Call, *sdk.Stream) (any, error) {
		return nil, nil
	}
	if err := module.Validate(); err != nil {
		t.Fatal("public finite JSONL control failed", err)
	}
	for _, container := range []string{"sqlite", "json"} {
		module.Manifest.SessionSources[0].Container = container
		err := module.Validate()
		if err == nil || !strings.Contains(err.Error(), "invalid session source descriptor") {
			t.Fatal("actual vendor container was admitted without a storage grant", container, err)
		}
	}
	for _, layout := range []sdk.SessionSourceLayout{
		{Kind: "flat", IDFrom: "filename-stem"},
		{Kind: "bucketed", IDFrom: "filename-after-first-underscore"},
		{Kind: "directory", IDFrom: "parent-directory", Entry: "log.jsonl"},
	} {
		for _, path := range []string{"opencode.db", "opencode.db-wal", "storage/session/project/ses_fixture.json",
			"storage/message/ses_fixture/msg_fixture.json", "storage/part/msg_fixture/part_fixture.json"} {
			if _, err := layout.SessionID(path); err == nil {
				t.Fatal("actual vendor storage was disguised as a supported JSONL locator", path)
			}
		}
	}
	if NewWindowModule(ModuleOptions{}).Manifest.Contract.Has(sdk.SessionSource) {
		t.Fatal("ordinary package substituted the finite JSONL contract for actual storage")
	}
}
