package outbound

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"testing"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
)

func TestAccountNativeClientKeepsSelectedAuthorityAndWriteClaim(t *testing.T) {
	var requests, accepted atomic.Int32
	response := append([]byte{10, 10}, []byte{10, 3, 'j', 'w', 't', 18, 3, 'm', 'o', 'd'}...)
	client, _ := fixtureClient(t, func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Method != "POST" || request.URL.Path != codec.ModelAssignment.Path() ||
			request.Header.Get("Authorization") != "Basic selected-key-selected-key" ||
			accepted.Load() != 1 {
			t.Error("native account request changed its credential or acceptance authority")
		}
		body, _ := io.ReadAll(request.Body)
		if !bytes.Contains(body, []byte("selected-key")) || bytes.Contains(body, []byte("local-key")) {
			t.Error("native account request leaked local authority")
		}
		writer.Header().Set("Content-Type", "application/proto")
		_, _ = writer.Write(response)
	})
	body := []byte{18, 3, 'm', 'o', 'd', 26, 1, 's'}
	credentials := Credentials{Key: "selected-key"}
	got, err := client.Account(t.Context(), credentials, codec.ModelAssignment, body, "mod",
		func(context.Context) error { accepted.Add(1); return nil })
	if err != nil || !bytes.Equal(got, response) || requests.Load() != 1 || accepted.Load() != 1 {
		t.Fatal("native assignment did not complete exactly once", err)
	}
	_, err = client.Account(t.Context(), credentials, codec.ModelAssignment, body, "foreign", nil)
	var failure *Error
	if !errors.As(err, &failure) || failure.Outcome != "not-started" || requests.Load() != 1 {
		t.Fatal("invalid native assignment reached the selected vendor", err)
	}
	_, err = client.Account(t.Context(), credentials, codec.ModelAssignment, body, "mod",
		func(context.Context) error { return errors.New("lost durable acknowledgement") })
	if !errors.As(err, &failure) || failure.Outcome != "uncertain" || requests.Load() != 1 {
		t.Fatal("lost acceptance was replay-safe or sent a native request", err)
	}
}

func TestAccountNativeClientRejectsCredentialEchoAndForeignMedia(t *testing.T) {
	for _, name := range []string{"echo", "media", "malformed"} {
		t.Run(name, func(t *testing.T) {
			client, _ := fixtureClient(t, func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/proto")
				body := []byte{}
				if name == "echo" {
					body = []byte("selected-secret")
				}
				if name == "media" {
					writer.Header().Set("Content-Type", "application/json")
				}
				if name == "malformed" {
					body = []byte{0}
				}
				_, _ = writer.Write(body)
			})
			if _, err := client.Account(t.Context(), Credentials{Key: "selected-secret"}, codec.UserStatus, nil, "m", nil); err == nil {
				t.Fatal("native account response leaked credentials or changed wire semantics")
			}
		})
	}
}
