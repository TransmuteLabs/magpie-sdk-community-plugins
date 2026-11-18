package ompsessions

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func nativeSourceFixture(test *testing.T) (sdk.SessionRequest, []byte) {
	test.Helper()
	image := bytes.Repeat([]byte("synthetic-image"), 180)
	document := Document{Schema: 1, ID: sessionID, Cwd: "/work/omp", Timestamp: fixtureTime,
		Items: []Item{
			{Kind: "user", Content: []Content{{Type: "text", Text: "blob:not-an-attachment"}, {Type: "image", MimeType: "image/png", Data: base64.StdEncoding.EncodeToString(image)}}},
			{Kind: "call", ID: "shell", Name: "bash", Shell: "printf native", Workdir: "/work/sub"},
			{Kind: "result", ID: "shell", IsError: true, Content: []Content{{Type: "text", Text: "native result"}, {Type: "image", MimeType: "image/png", Data: base64.StdEncoding.EncodeToString(image)}}},
			{Kind: "assistant", Text: "native completion"},
		}}
	encoded, err := Encode(test.Context(), document, fixtureTime)
	if err != nil {
		test.Fatal(err)
	}
	request := sdk.SessionRequest{ID: "omp-to-claude", From: "omp", To: "claude",
		Options: json.RawMessage(`{"cwd":"/work/omp"}`),
		Files:   []sdk.SessionDocument{{Path: "sessions/source.jsonl", Format: "omp", Body: encoded.Journal}}}
	for _, asset := range encoded.Assets {
		request.Files = append(request.Files, sdk.SessionDocument{Path: "blobs/" + asset.SHA256, Format: "omp", Body: asset.Body})
	}
	return request, image
}

func TestNativeRoutesPreserveLargeAttachmentsWithoutGlobalBlobWrites(test *testing.T) {
	for _, target := range []string{"claude", "codex"} {
		test.Run(target, func(test *testing.T) {
			request, image := nativeSourceFixture(test)
			request.ID, request.To = "omp-to-"+target, target
			if target == "codex" {
				request.Options = json.RawMessage(`{"cwd":"/work/omp","provider":"fixture-provider"}`)
			}
			targetResult, err := Convert(test.Context(), request)
			if err != nil {
				test.Fatal("missing recovered native route:", err)
			}
			if len(targetResult.Files) != 1 || !validUUID(targetResult.ID) {
				test.Fatal("native target did not return one complete journal")
			}
			reverse, err := Convert(test.Context(), sdk.SessionRequest{ID: target + "-to-omp", From: target, To: "omp",
				Files: targetResult.Files, Options: json.RawMessage(`{"cwd":"/work/omp","canonicalCwd":"/work/omp","home":"/home/test","temp":"/tmp"}`)})
			if err != nil {
				test.Fatal("native reverse route unavailable:", err)
			}
			if len(reverse.Files) != 1 || !strings.HasPrefix(reverse.Files[0].Path, "--work-omp--/") {
				test.Fatalf("native reverse requires global blob writes or has the wrong bucket: %+v", reverse)
			}
			if bytes.Contains(reverse.Files[0].Body, []byte("blob:sha256:")) {
				test.Fatal("native reverse did not keep large image bytes inline")
			}
			document, err := Decode(test.Context(), reverse.Files[0].Body, nil)
			if err != nil || document.Cwd != "/work/omp" || document.ID != reverse.ID {
				test.Fatalf("reverse output is not a native OMP journal: %v", err)
			}
			items, err := readItemsWithResultMedia(test.Context(), reverse.Files[0].Body, nil, true)
			if err != nil {
				test.Fatal(err)
			}
			images := 0
			for _, item := range items {
				for _, content := range item.Content {
					if content.Type == "image" {
						raw, err := base64.StdEncoding.DecodeString(content.Data)
						if err != nil || !bytes.Equal(raw, image) {
							test.Fatal("native reverse changed attachment bytes")
						}
						images++
					}
				}
			}
			if images != 2 || !bytes.Contains(reverse.Files[0].Body, []byte("native result")) ||
				!bytes.Contains(reverse.Files[0].Body, []byte("blob:not-an-attachment")) {
				test.Fatal("native reverse lost selected media or context")
			}
		})
	}
}

func TestNativeOMPBucketMatchesRecoveredHomeAndTempRules(test *testing.T) {
	for _, item := range []struct{ cwd, home, temporary, want string }{
		{"/home/me/project", "/home/me", "/tmp", "-project"},
		{"/home/me/tmp/project", "/home/me", "/home/me/tmp", "-tmp-project"},
		{"/tmp", "/home/me", "/tmp", "-tmp"},
		{"/work/omp", "/home/me", "/tmp", "--work-omp--"},
	} {
		request, _ := nativeSourceFixture(test)
		claude, err := Convert(test.Context(), request)
		if err != nil {
			test.Fatal(err)
		}
		options, _ := json.Marshal(map[string]string{"cwd": item.cwd, "canonicalCwd": item.cwd, "home": item.home, "temp": item.temporary})
		result, err := Convert(test.Context(), sdk.SessionRequest{ID: "claude-to-omp", From: "claude", To: "omp", Files: claude.Files, Options: options})
		if err != nil || !strings.HasPrefix(result.Files[0].Path, item.want+"/") {
			test.Fatalf("bucket for %s: %v %v", item.cwd, result, err)
		}
	}
}
