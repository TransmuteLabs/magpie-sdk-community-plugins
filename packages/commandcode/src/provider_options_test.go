package commandcode

import (
	"strings"
	"testing"
)

func TestProviderZDRUsesConfirmedOption(t *testing.T) {
	manifest := Manifest()
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	options := manifest.Providers[0].Options
	if len(options) != 1 || options[0].Key != "zdr" || options[0].Default ||
		options[0].Label != "Zero Data Retention" ||
		!strings.Contains(options[0].Description, "x-cmd-zdr: 1") ||
		!strings.Contains(options[0].Description, "422 cmd_zdr_no_providers") ||
		!strings.Contains(options[0].Confirm, "cost more") ||
		!strings.Contains(options[0].Confirm, "fails instead of falling back") {
		t.Fatalf("ordinary provider editor lacks the confirmed donor ZDR switch: %+v", options)
	}
}
