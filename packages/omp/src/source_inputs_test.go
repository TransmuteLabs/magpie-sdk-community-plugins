package ompsessions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestSourceInputsOnlyPlansActualAttachments(t *testing.T) {
	request := sdkFixture(t, "omp")
	request.Files = request.Files[:1]
	request.Options = nil
	hash := strings.TrimPrefix(sdkFixture(t, "omp").Files[1].Path, "blobs/")
	callSourceInputs := func(t *testing.T, request sdk.SessionRequest) []struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	} {
		t.Helper()
		action := Module().Actions["session.inputs.v1"]
		if action == nil {
			t.Fatal("native module cannot declare its attachment inputs")
		}
		params, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		value, err := action(t.Context(), sdk.Call{Params: params}, nil)
		if err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Version int `json:"version"`
			Files   []struct {
				Path   string `json:"path"`
				SHA256 string `json:"sha256"`
			} `json:"files"`
		}
		if json.Unmarshal(body, &result) != nil || result.Version != 1 {
			t.Fatalf("unversioned attachment plan: %s", body)
		}
		return result.Files
	}
	files := callSourceInputs(t, request)
	if len(files) != 1 || files[0].Path != "blobs/"+hash || files[0].SHA256 != hash {
		t.Fatalf("actual image not declared exactly once: %+v", files)
	}
	request.Files[0].Body = bytes.ReplaceAll(request.Files[0].Body, []byte("blob:sha256:"+hash), []byte("inline-image-data"))
	request.Files[0].Body = bytes.ReplaceAll(request.Files[0].Body, []byte("human request"), []byte("blob:not-an-attachment"))
	if files := callSourceInputs(t, request); len(files) != 0 {
		t.Fatalf("ordinary text became an attachment: %+v", files)
	}
}

func TestSourceInputsIncludesInactiveAndFileMentionAttachments(t *testing.T) {
	request := sdkFixture(t, "omp")
	request.Files = request.Files[:1]
	extraHash := strings.Repeat("a", 64)
	record := fmt.Sprintf(`{"type":"message","id":"off-branch-image","parentId":"u","message":{"role":"fileMention","files":[{"image":{"data":"blob:sha256:%s"}}]}}`, extraHash)
	request.Files[0].Body = append(request.Files[0].Body, []byte(record+"\n")...)
	result, err := PlanSourceInputs(t.Context(), request)
	if err != nil || len(result.Files) != 2 {
		t.Fatalf("inactive file-mention image was not planned: %+v %v", result, err)
	}
	if result.Files[0].SHA256 != extraHash || result.Files[0].Path != "blobs/"+extraHash {
		t.Fatal("attachment references were not deduplicated and sorted:", result)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := PlanSourceInputs(ctx, request); err != context.Canceled {
		t.Fatal("input planning ignored cancellation:", err)
	}
}

func TestSourceInputsHasBoundedInputAndReferenceCount(t *testing.T) {
	request := sdkFixture(t, "omp")
	request.Files = request.Files[:1]
	for index := range MaxFiles {
		record := fmt.Sprintf(`{"type":"message","id":"image-%d","message":{"role":"user","content":[{"type":"image","data":"blob:sha256:%064x"}]}}`, index, index)
		request.Files[0].Body = append(request.Files[0].Body, []byte(record+"\n")...)
	}
	if _, err := PlanSourceInputs(t.Context(), request); err == nil {
		t.Fatal("unbounded attachment plan was accepted")
	}
	request.Files[0].Body = make([]byte, MaxBytes+1)
	if _, err := PlanSourceInputs(t.Context(), request); err == nil {
		t.Fatal("oversized journal reached input planning")
	}
}

func TestSourceInputsRejectsInvalidReferences(t *testing.T) {
	request := sdkFixture(t, "omp")
	hash := strings.TrimPrefix(request.Files[1].Path, "blobs/")
	request.Files = request.Files[:1]
	request.Files[0].Body = bytes.ReplaceAll(request.Files[0].Body, []byte("blob:sha256:"+hash), []byte("blob:sha256:../../secret"))
	action := Module().Actions["session.inputs.v1"]
	if action == nil {
		t.Fatal("native module cannot declare its attachment inputs")
	}
	params, _ := json.Marshal(request)
	if _, err := action(t.Context(), sdk.Call{Params: params}, nil); err == nil {
		t.Fatal("malformed attachment reference was accepted")
	}
}
