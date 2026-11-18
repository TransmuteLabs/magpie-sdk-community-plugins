package protocol

import (
	"crypto/sha256"
	"fmt"
	"testing"
	"time"
)

func TestStartTemplatePinsVendor3182StaticCLISections(t *testing.T) {
	for _, section := range []struct {
		value, sha256 string
	}{
		{startPrefix, "7779642ea44a46963bcdc9c151aa95d4761f17182929e41cb5bff571110af440"},
		{startIdentity, "aac4ba4cc7b555b3a32704850028e242a6e3045df91d7970f04d18c378e57f40"},
		{startDynamic, "7d843753f3adae93ecc51e6fa2cb9b07bdc0c4b946bc3d2404f0a6db596bf5d0"},
		{startContext, "406a0480d88086cdbb9eadbbcd63c4a95d370d4a89c20b753a4d96da90458b11"},
	} {
		if fmt.Sprintf("%x", sha256.Sum256([]byte(section.value))) != section.sha256 {
			t.Fatal("static CLI prompt diverged from the pinned source sections")
		}
	}
}

func TestStartPreparationIsStableAndRefusesUnknownSystemContent(t *testing.T) {
	prepared, err := PrepareStart([]byte(`{"model":"GLM-5.3","messages":[{"role":"user","content":"text"}]}`), time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := PrepareStart(prepared, time.Unix(86400, 0))
	if err != nil || string(repeated) != string(prepared) {
		t.Fatal("already prepared native CLI request was wrapped again")
	}
	for _, body := range []string{
		`{"messages":[],"system":{"unknown":true}}`,
		`{"messages":[],"system":[{"type":"image","source":{"data":"opaque"}}]}`,
	} {
		if _, err := PrepareStart([]byte(body), time.Unix(0, 0)); err == nil {
			t.Fatal("unknown system content was silently discarded")
		}
	}
}
