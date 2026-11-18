package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"os"
	"path/filepath"

	minimax "github.com/TransmuteLabs/magpie-sdk-community-plugins/packages/minimax"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func main() {
	executable, err := os.Executable()
	if err != nil || os.Setenv("SSL_CERT_FILE", filepath.Join(filepath.Dir(executable), "fixture-ca.pem")) != nil {
		os.Exit(1)
	}
	configured := minimax.NewModule(minimax.ModuleOptions{})
	configured.Manifest.Runtime = "exec"
	configured.Manifest.Entry = "bin/minimax"
	if configured.Validate() != nil {
		os.Exit(1)
	}
	if len(os.Args) == 2 && os.Args[1] == "--manifest" {
		if json.NewEncoder(os.Stdout).Encode(configured.Manifest) != nil {
			os.Exit(1)
		}
		return
	}
	certificate, err := os.ReadFile(filepath.Join(filepath.Dir(executable), "fixture-ca.pem"))
	roots := x509.NewCertPool()
	if err != nil || !roots.AppendCertsFromPEM(certificate) || os.Setenv("GODEBUG", "x509usefallbackroots=1") != nil {
		os.Exit(1)
	}
	x509.SetFallbackRoots(roots)
	if err := sdk.Serve(context.Background(), os.Stdin, os.Stdout, configured); err != nil {
		os.Exit(1)
	}
}
