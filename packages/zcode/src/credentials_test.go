package zcode

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

var fixtureNow = time.Unix(1794564000, 0).UTC()

func fixtureCredentials(kind string) Credentials {
	value := Credentials{Site: "zai", Kind: kind, UserID: "fixture-user", Device: "fixture-device"}
	switch kind {
	case "legacy-key":
		value.Key = "fixture-key"
	case "start":
		value.SessionToken = "fixture-session"
		value.ExpiresAt = fixtureNow.Add(time.Hour).UnixMilli()
	default:
		value.ProjectToken = "fixture-pat"
		value.ExpiresAt = fixtureNow.Add(time.Hour).UnixMilli()
	}
	if kind == "team" {
		value.Organization, value.Project, value.Product = "fixture-org", "fixture-project", "coding"
	}
	return value
}

func TestAccountIdentitySeparatesFamilySelectionAndTeam(t *testing.T) {
	seen := map[string]bool{}
	for _, site := range []string{"zai", "bigmodel"} {
		for _, kind := range []string{"legacy-key", "individual", "team", "start"} {
			value := fixtureCredentials(kind)
			value.Site = site
			if err := value.Validate(fixtureNow); err != nil {
				t.Fatal(err)
			}
			id := value.AccountID()
			if seen[id] || strings.Contains(id, value.token()) {
				t.Fatal("account identity collided or exposed credentials")
			}
			seen[id] = true
		}
	}
	value := fixtureCredentials("team")
	original := value.AccountID()
	value.ProjectToken = "replacement-pat"
	if value.AccountID() != original {
		t.Fatal("PAT rotation changed account identity")
	}
	value.Project = "another-project"
	if value.AccountID() == original {
		t.Fatal("team project switch reused account identity")
	}
}

func TestIncompleteScopeAndMixedCredentialRolesRefused(t *testing.T) {
	for _, change := range []func(*Credentials){
		func(value *Credentials) { value.Organization = "" },
		func(value *Credentials) { value.Project = "" },
		func(value *Credentials) { value.Product = "" },
		func(value *Credentials) { value.UserID = "" },
		func(value *Credentials) { value.Site = "unknown" },
		func(value *Credentials) { value.Device = "" },
		func(value *Credentials) { value.Key = "legacy" },
		func(value *Credentials) { value.SessionToken = "session\r\nunsafe" },
		func(value *Credentials) { value.ProjectToken = "unsafe\r\ncredential" },
	} {
		value := fixtureCredentials("team")
		change(&value)
		if value.Validate(fixtureNow) == nil {
			t.Fatal("accepted invalid team credentials")
		}
	}
	for _, kind := range []string{"individual", "team", "start"} {
		value := fixtureCredentials(kind)
		value.ExpiresAt = fixtureNow.Add(30 * time.Second).UnixMilli()
		if !errors.Is(value.Validate(fixtureNow), ErrExpired) {
			t.Fatal("credential at refresh boundary accepted")
		}
	}
}

func TestHeadersNeverForwardCallerAuthorityOrEncoding(t *testing.T) {
	source := http.Header{
		"authorization": {"foreign"}, "X-Api-Key": {"foreign"}, "Cookie": {"secret"},
		"aCcEpT-EnCoDiNg": {"gzip"}, "X-Off-Peak-Ticket-ID": {"forged"},
		"Bigmodel-Project": {"foreign"}, "Anthropic-Beta": {"tools-2025"},
	}
	for _, kind := range []string{"legacy-key", "individual", "team", "start"} {
		account := fixtureCredentials(kind)
		headers := requestHeaders(source, account, "request")
		if headers.Get("Authorization") != "Bearer "+account.token() ||
			headers.Get("Accept-Encoding") != "" || headers.Get("Cookie") != "" ||
			headers.Get("X-Off-Peak-Ticket-ID") != "" {
			t.Fatal("caller authority or content encoding escaped filtering")
		}
		if kind == "start" && (headers.Get("X-Api-Key") != "" || headers.Get("Anthropic-Beta") != "" || headers.Get("Bigmodel-Project") != "") {
			t.Fatal("Start received paid credentials or paid beta headers")
		}
		if kind == "team" && headers.Get("Bigmodel-Project") != account.Project {
			t.Fatal("Team selection lost")
		}
	}
	if source.Get("Cookie") != "secret" || source["aCcEpT-EnCoDiNg"][0] != "gzip" {
		t.Fatal("caller headers mutated")
	}
}

func TestNativeAliasDetectionDoesNotCaptureOrdinaryHighspeed(t *testing.T) {
	for _, model := range []string{"GLM-5.3-OffPeak", "GLM-5.3-Flash-OffPeak", "glm-5.3-highspeed", "GLM-5.3-Flash-Highspeed"} {
		if _, found := NativeLane(model); !found {
			t.Fatal("special execution alias treated as ordinary")
		}
	}
	if _, found := NativeLane("GLM-5.1-Highspeed"); found {
		t.Fatal("ordinary historical Highspeed model confused with ticket/card alias")
	}
}
