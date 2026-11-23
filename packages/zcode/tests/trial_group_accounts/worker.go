package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	zcode "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 1 || len(args) > 2 || len(args) == 2 && args[1] != "fixture" {
		return fmt.Errorf("expected loopback origin and optional fixture command")
	}
	origin := args[0]
	address, err := url.Parse(origin)
	if err != nil || address.Scheme != "http" || address.Hostname() != "127.0.0.1" ||
		address.Port() == "" || address.User != nil || address.RequestURI() != "/" ||
		address.Fragment != "" || origin != "http://"+address.Host {
		return fmt.Errorf("expected a local HTTP origin")
	}
	module := zcode.NewModule(zcode.ModuleOptions{Endpoints: zcode.Endpoints{
		Zai: origin + "/zai", BigModel: origin + "/bigmodel", Start: origin + "/start",
		ZaiQuota: origin + "/quota/zai", BigModelQuota: origin + "/quota/bigmodel",
		LegacyBigModelQuota: origin + "/quota/legacy-bigmodel",
	}})
	module.Manifest.Runtime = "exec"
	module.Manifest.Entry = "worker"
	module.Manifest.Args = []string{origin}
	module.Manifest.Compatibility = nil
	if len(args) == 1 {
		return sdk.Serve(context.Background(), os.Stdin, os.Stdout, module)
	}
	type fixtureAccount struct {
		Label string
		ID    string
		Auth  zcode.Credentials
	}
	expiry := time.Now().Add(time.Hour).UnixMilli()
	accounts := make([]fixtureAccount, 0, 2)
	for _, label := range []string{"a", "b"} {
		auth := zcode.Credentials{
			Site: "zai", Kind: "individual", UserID: "trial-group-" + label, Device: "fixture-device-" + label,
			ProjectToken: "fixture-paid-" + label, SessionToken: "fixture-trial-" + label,
			ExpiresAt: expiry, SessionExpiresAt: expiry, Discovery: true,
		}
		accounts = append(accounts, fixtureAccount{Label: label, ID: auth.AccountID(), Auth: auth})
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Manifest sdk.Manifest
		Accounts []fixtureAccount
	}{Manifest: module.Manifest, Accounts: accounts})
}
