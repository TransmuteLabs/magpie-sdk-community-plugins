package claudedesktop

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func fixtureFiles(bodies map[string]string) []fileSnapshot {
	files := []fileSnapshot{}
	for _, path := range Manifest().Agents[0].ConfigFiles {
		body, exists := bodies[path]
		file := fileSnapshot{Path: path, BeforeHash: "missing", Exists: exists, Mode: 0o600}
		if exists {
			file.Body = []byte(body)
			digest := sha256.Sum256(file.Body)
			file.BeforeHash = hex.EncodeToString(digest[:])
		}
		files = append(files, file)
	}
	return files
}

func fixtureOptions() agentOptions {
	return agentOptions{Gateway: "http://127.0.0.1:3425", Key: "host-owned-key"}
}

func fixtureBinding(connected bool) *sdk.AgentBindingSnapshot {
	revision := "0"
	if connected {
		revision = "1"
	}
	return &sdk.AgentBindingSnapshot{Target: TargetID, Revision: revision, Connected: connected, Values: map[string]string{}}
}

func plan(t *testing.T, action string, files []fileSnapshot, options agentOptions, connected bool) sdk.AgentPlan {
	t.Helper()
	result, err := planAgent(context.Background(), sdk.Call{Params: raw(sdk.AgentPlanRequest{
		ID: AgentID, Action: action, Settings: raw(settingsEnvelope{Files: files}), Options: raw(options), Binding: fixtureBinding(connected),
	})}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return result.(sdk.AgentPlan)
}

func applyFixture(t *testing.T, files []fileSnapshot, changes sdk.AgentPlan) []fileSnapshot {
	t.Helper()
	bodies := map[string]string{}
	for _, file := range files {
		if file.Exists {
			bodies[file.Path] = string(file.Body)
		}
	}
	for _, change := range changes.Files {
		file := files[slices.IndexFunc(files, func(candidate fileSnapshot) bool { return candidate.Path == change.Path })]
		if change.BeforeHash != file.BeforeHash {
			t.Fatal("plan lost the host precondition")
		}
		if change.Remove {
			delete(bodies, change.Path)
		} else {
			if change.Mode != 0o600 {
				t.Fatal("configuration is not private")
			}
			bodies[change.Path] = string(change.Body)
		}
	}
	return fixtureFiles(bodies)
}

func inspect(t *testing.T, files []fileSnapshot, options agentOptions) (sdk.AgentInspectResult, error) {
	t.Helper()
	result, err := inspectAgent(context.Background(), sdk.Call{Params: raw(sdk.AgentInspectRequest{
		ID: AgentID, Settings: raw(settingsEnvelope{Files: files}), Options: raw(options), Binding: fixtureBinding(true),
	})}, nil)
	if err != nil {
		return sdk.AgentInspectResult{}, err
	}
	return result.(sdk.AgentInspectResult), nil
}

func TestAgentOnlyUsesExistingDesktopGateway(t *testing.T) {
	manifest := Manifest()
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Providers) != 0 || len(manifest.Inbounds) != 0 || len(manifest.Services) != 0 || len(manifest.Contract.Capabilities) != 1 || manifest.Contract.Capabilities[0] != sdk.AgentWiring {
		t.Fatal("agent claimed provider or server ownership")
	}
	before := fixtureFiles(nil)
	options := fixtureOptions()
	connected := plan(t, "connect", before, options, false)
	if !connected.Binding.Connected || len(connected.Files) != len(before) {
		t.Fatalf("incomplete configuration: %+v", connected)
	}
	files := applyFixture(t, before, connected)
	checked, err := readFiles(raw(settingsEnvelope{Files: files}))
	if err != nil {
		t.Fatal(err)
	}
	profile := checked[ProfilePath].values
	for key, expected := range profileValues(options) {
		if !same(profile[key], expected) {
			t.Fatalf("missing Desktop contract field %s", key)
		}
	}
	if !same(profile["inferenceGatewayBaseUrl"], raw("http://127.0.0.1:3425/agent/claude-desktop")) || !same(profile["inferenceGatewayApiKey"], raw(options.Key)) {
		t.Fatal("gateway lost Desktop routing identity or host key")
	}
	for _, name := range []string{"selfHostedUrl", "bootstrapUrl", "enterpriseConfig", "chatTabEnabled", "coworkTabEnabled"} {
		if _, exists := profile[name]; exists {
			t.Fatalf("unneeded or deprecated setting %s", name)
		}
	}
	status, err := inspect(t, files, options)
	if err != nil || !status.Connected {
		t.Fatalf("connected inspection: %+v %v", status, err)
	}
	if sync := plan(t, "sync", files, options, true); len(sync.Files) != 0 {
		t.Fatal("unchanged sync rewrote configuration")
	}
	options.Previous = before
	disconnected := plan(t, "disconnect", files, options, true)
	if disconnected.Binding.Connected || len(disconnected.Files) != len(before) {
		t.Fatal("disconnect did not remove exclusively generated files")
	}
	for _, file := range applyFixture(t, files, disconnected) {
		if file.Exists {
			t.Fatalf("generated file remains: %s", file.Path)
		}
	}
}

func TestDisconnectRestoresOriginalBytesAndOtherProfiles(t *testing.T) {
	before := fixtureFiles(map[string]string{
		ConfigPath:           "{\"mcpServers\":{\"mine\":{}},\"deploymentMode\":\"1p\"}\n",
		ThirdPartyConfigPath: "{\"theme\":\"dark\"}\n",
		MetaPath:             "{\"entries\":[{\"id\":\"other-profile\",\"name\":\"Mine\"}],\"appliedId\":\"other-profile\",\"future\":true}\n",
		ProfilePath:          "{\"toolSearchEnabled\":false,\"future\":{\"value\":1}}\n",
	})
	options := fixtureOptions()
	files := applyFixture(t, before, plan(t, "connect", before, options, false))
	options.Previous = before
	files = applyFixture(t, files, plan(t, "disconnect", files, options, true))
	for index, file := range files {
		if !bytes.Equal(file.Body, before[index].Body) {
			t.Fatalf("preimage not restored for %s: %s", file.Path, file.Body)
		}
	}
}

func TestDisconnectPreservesUserEditsAndNewSelection(t *testing.T) {
	before := fixtureFiles(nil)
	options := fixtureOptions()
	files := applyFixture(t, before, plan(t, "connect", before, options, false))
	values, _ := readFiles(raw(settingsEnvelope{Files: files}))
	values[ConfigPath].values["deploymentMode"] = raw("1p")
	values[ProfilePath].values["inferenceGatewayBaseUrl"] = raw("https://user.example")
	values[ProfilePath].values["userSetting"] = raw(true)
	values[MetaPath].values["appliedId"] = raw("new-user-profile")
	meta, _ := metaEntries(values[MetaPath].values)
	meta = append(meta, raw(map[string]string{"id": "new-user-profile", "name": "User choice"}))
	values[MetaPath].values["entries"] = raw(meta)
	bodies := map[string]string{}
	for path, value := range values {
		bodies[path] = string(raw(value.values))
	}
	files = fixtureFiles(bodies)
	options.Previous = before
	files = applyFixture(t, files, plan(t, "disconnect", files, options, true))
	values, _ = readFiles(raw(settingsEnvelope{Files: files}))
	if !same(values[ConfigPath].values["deploymentMode"], raw("1p")) || !same(values[MetaPath].values["appliedId"], raw("new-user-profile")) || !same(values[ProfilePath].values["userSetting"], raw(true)) || !same(values[ProfilePath].values["inferenceGatewayBaseUrl"], raw("https://user.example")) {
		t.Fatal("user edits or selection were overwritten")
	}
	entries, _ := metaEntries(values[MetaPath].values)
	if len(entries) != 1 || !bytes.Contains(entries[0], []byte("new-user-profile")) {
		t.Fatal("foreign profile was changed")
	}
}

func TestSealedKeyRemainsOpaqueAcrossSync(t *testing.T) {
	options := fixtureOptions()
	files := applyFixture(t, fixtureFiles(nil), plan(t, "connect", fixtureFiles(nil), options, false))
	for index := range files {
		if files[index].Path == ProfilePath {
			values, _ := readObject(files[index].Body)
			values["inferenceGatewayApiKey"] = raw("__sealed__:v1:b3BhcXVlLWtleQ==")
			bodies := map[string]string{}
			for _, file := range files {
				bodies[file.Path] = string(file.Body)
			}
			bodies[ProfilePath] = string(raw(values))
			files = fixtureFiles(bodies)
			break
		}
	}
	status, err := inspect(t, files, options)
	if err != nil || !status.Connected || !strings.Contains(status.Detail, "cannot be verified") {
		t.Fatalf("sealed uncertainty lost: %+v %v", status, err)
	}
	if synced := plan(t, "sync", files, options, true); len(synced.Files) != 0 {
		t.Fatal("sync rewrote sealed secret")
	}
}

func TestSyncNeverTakesBackAnotherSelectedProfile(t *testing.T) {
	options := fixtureOptions()
	files := applyFixture(t, fixtureFiles(nil), plan(t, "connect", fixtureFiles(nil), options, false))
	for index := range files {
		if files[index].Path == MetaPath {
			values, _ := readObject(files[index].Body)
			values["appliedId"] = raw("user-choice")
			files[index] = fixtureFiles(map[string]string{MetaPath: string(raw(values))})[index]
		}
	}
	_, err := planAgent(context.Background(), sdk.Call{Params: raw(sdk.AgentPlanRequest{ID: AgentID, Action: "sync", Settings: raw(settingsEnvelope{Files: files}), Options: raw(options), Binding: fixtureBinding(true)})}, nil)
	var failure *sdk.Error
	if !errors.As(err, &failure) || failure.Code != "configuration_changed" {
		t.Fatalf("background sync took another profile: %v", err)
	}
}

func TestInspectionAndSyncPreservePersistedModeNormalization(t *testing.T) {
	for _, paths := range [][]string{{ConfigPath}, {ThirdPartyConfigPath}, {ConfigPath, ThirdPartyConfigPath}} {
		for _, mutation := range []string{"changed", "removed", "deleted"} {
			t.Run(strings.Join(paths, "+")+"/"+mutation, func(t *testing.T) {
				options := fixtureOptions()
				files := applyFixture(t, fixtureFiles(nil), plan(t, "connect", fixtureFiles(nil), options, false))
				bodies := map[string]string{}
				for _, file := range files {
					bodies[file.Path] = string(file.Body)
				}
				for _, path := range paths {
					switch mutation {
					case "changed":
						bodies[path] = `{"deploymentMode":"1p","userSetting":true}`
					case "removed":
						bodies[path] = `{"userSetting":true}`
					case "deleted":
						delete(bodies, path)
					}
				}
				files = fixtureFiles(bodies)
				status, err := inspect(t, files, options)
				if err != nil || !status.Connected {
					t.Fatalf("persisted mode overrode the forced gateway profile: %+v %v", status, err)
				}
				synced := plan(t, "sync", files, options, true)
				if !synced.Binding.Connected || len(synced.Files) != 0 {
					t.Fatalf("sync reclaimed normalized persisted mode: %+v", synced)
				}
			})
		}
	}
}

func TestInspectionAndSyncAcceptCompatibleProfileModes(t *testing.T) {
	for _, mode := range []string{"absent", "any", "3p"} {
		t.Run(mode, func(t *testing.T) {
			options := fixtureOptions()
			files := applyFixture(t, fixtureFiles(nil), plan(t, "connect", fixtureFiles(nil), options, false))
			bodies := map[string]string{}
			for _, file := range files {
				values, err := readObject(file.Body)
				if err != nil {
					t.Fatal(err)
				}
				switch file.Path {
				case ConfigPath, ThirdPartyConfigPath:
					values["deploymentMode"] = raw("1p")
				case ProfilePath:
					if mode != "absent" {
						values["deploymentMode"] = raw(mode)
					}
				}
				bodies[file.Path] = string(raw(values))
			}
			files = fixtureFiles(bodies)
			status, err := inspect(t, files, options)
			if err != nil || !status.Connected {
				t.Fatalf("compatible forced profile was refused: %+v %v", status, err)
			}
			if synced := plan(t, "sync", files, options, true); len(synced.Files) != 0 {
				t.Fatalf("sync changed the compatible profile or persisted mode: %+v", synced)
			}
		})
	}
}

func TestInspectionConnectAndSyncRefuseFirstPartyOrInvalidProfileMode(t *testing.T) {
	for name, mode := range map[string]string{
		"first-party": `"1p"`,
		"unknown":     `"other"`,
		"null":        `null`,
		"number":      `1`,
		"boolean":     `true`,
		"object":      `{}`,
		"array":       `[]`,
	} {
		t.Run(name, func(t *testing.T) {
			options := fixtureOptions()
			files := applyFixture(t, fixtureFiles(nil), plan(t, "connect", fixtureFiles(nil), options, false))
			bodies := map[string]string{}
			for _, file := range files {
				bodies[file.Path] = string(file.Body)
			}
			profile, err := readObject([]byte(bodies[ProfilePath]))
			if err != nil {
				t.Fatal(err)
			}
			profile["deploymentMode"] = []byte(mode)
			bodies[ProfilePath] = string(raw(profile))
			files = fixtureFiles(bodies)
			_, err = inspect(t, files, options)
			var failure *sdk.Error
			if !errors.As(err, &failure) || failure.Code != "configuration_changed" {
				t.Fatalf("inspection accepted an overriding or invalid profile mode: %v", err)
			}
			for _, action := range []string{"connect", "sync"} {
				result, err := planAgent(t.Context(), sdk.Call{Params: raw(sdk.AgentPlanRequest{
					ID: AgentID, Action: action, Settings: raw(settingsEnvelope{Files: files}),
					Options: raw(options), Binding: fixtureBinding(action == "sync"),
				})}, nil)
				failure = nil
				if result != nil || !errors.As(err, &failure) || failure.Code != "configuration_changed" {
					t.Fatalf("%s returned a plan for an overriding or invalid profile mode: %+v %v", action, result, err)
				}
			}
		})
	}
}

func TestInspectionAndSyncStillRefuseChooserDrift(t *testing.T) {
	for _, mutation := range []string{"disabled", "removed"} {
		t.Run(mutation, func(t *testing.T) {
			options := fixtureOptions()
			files := applyFixture(t, fixtureFiles(nil), plan(t, "connect", fixtureFiles(nil), options, false))
			bodies := map[string]string{}
			for _, file := range files {
				bodies[file.Path] = string(file.Body)
			}
			profile, err := readObject([]byte(bodies[ProfilePath]))
			if err != nil {
				t.Fatal(err)
			}
			if mutation == "removed" {
				delete(profile, "disableDeploymentModeChooser")
			} else {
				profile["disableDeploymentModeChooser"] = raw(false)
			}
			bodies[ProfilePath] = string(raw(profile))
			files = fixtureFiles(bodies)
			if _, err := inspect(t, files, options); err == nil {
				t.Fatal("inspection accepted a changed owned chooser")
			}
			if result, err := planAgent(t.Context(), sdk.Call{Params: raw(sdk.AgentPlanRequest{
				ID: AgentID, Action: "sync", Settings: raw(settingsEnvelope{Files: files}),
				Options: raw(options), Binding: fixtureBinding(true),
			})}, nil); err == nil || result != nil {
				t.Fatalf("sync reclaimed the changed chooser: %+v %v", result, err)
			}
		})
	}
}

func TestDisconnectPreservesNormalizedPersistedModes(t *testing.T) {
	for _, mutation := range []string{"changed", "removed", "deleted"} {
		t.Run(mutation, func(t *testing.T) {
			before := fixtureFiles(map[string]string{
				ConfigPath:           `{"deploymentMode":"1p","userSetting":true}`,
				ThirdPartyConfigPath: `{"deploymentMode":"1p","userSetting":true}`,
			})
			options := fixtureOptions()
			files := applyFixture(t, before, plan(t, "connect", before, options, false))
			bodies := map[string]string{}
			for _, file := range files {
				bodies[file.Path] = string(file.Body)
			}
			for _, path := range []string{ConfigPath, ThirdPartyConfigPath} {
				switch mutation {
				case "changed":
					bodies[path] = `{"deploymentMode":"1p","userSetting":true}`
				case "removed":
					bodies[path] = `{"userSetting":true}`
				case "deleted":
					delete(bodies, path)
				}
			}
			files = fixtureFiles(bodies)
			options.Previous = before
			restored := applyFixture(t, files, plan(t, "disconnect", files, options, true))
			for index, file := range restored {
				if file.Path == ConfigPath || file.Path == ThirdPartyConfigPath {
					if file.Exists != files[index].Exists || !bytes.Equal(file.Body, files[index].Body) {
						t.Fatalf("disconnect reclaimed normalized persisted mode: %s", file.Path)
					}
				} else if file.Exists {
					t.Fatalf("unchanged generated file was not removed: %s", file.Path)
				}
			}
		})
	}
}

func TestHybridBootstrapPointerCannotSilentlyOverrideLocalProfile(t *testing.T) {
	options := fixtureOptions()
	before := fixtureFiles(map[string]string{
		MetaPath: `{"entries":[],"hybridPointer":{"bootstrapUrl":"https://managed.example/bootstrap.json"}}`,
	})
	if _, err := planAgent(t.Context(), sdk.Call{Params: raw(sdk.AgentPlanRequest{
		ID: AgentID, Action: "connect", Settings: raw(settingsEnvelope{Files: before}),
		Options: raw(options), Binding: fixtureBinding(false),
	})}, nil); err == nil {
		t.Fatal("connect ignored the active hybrid bootstrap pointer")
	}
	files := applyFixture(t, fixtureFiles(nil), plan(t, "connect", fixtureFiles(nil), options, false))
	bodies := map[string]string{}
	for _, file := range files {
		values, _ := readObject(file.Body)
		if file.Path == MetaPath {
			values["hybridPointer"] = raw(map[string]string{"bootstrapUrl": "https://managed.example/bootstrap.json"})
		}
		bodies[file.Path] = string(raw(values))
	}
	files = fixtureFiles(bodies)
	if _, err := inspect(t, files, options); err == nil {
		t.Fatal("inspection ignored the newly selected hybrid bootstrap")
	}
	if _, err := planAgent(t.Context(), sdk.Call{Params: raw(sdk.AgentPlanRequest{
		ID: AgentID, Action: "sync", Settings: raw(settingsEnvelope{Files: files}),
		Options: raw(options), Binding: fixtureBinding(true),
	})}, nil); err == nil {
		t.Fatal("sync ignored the newly selected hybrid bootstrap")
	}
}

func TestDisconnectUsesAppliedValuesAfterHostAddressAndKeyChange(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "generated"
		bodies := map[string]string{}
		if existing {
			name = "existing"
			bodies[ProfilePath] = `{"inferenceGatewayBaseUrl":"https://original.example","inferenceGatewayApiKey":"original-key","future":true}`
		}
		t.Run(name, func(t *testing.T) {
			before := fixtureFiles(bodies)
			options := fixtureOptions()
			files := applyFixture(t, before, plan(t, "connect", before, options, false))
			options.Gateway, options.Key, options.Previous = "http://127.0.0.1:7000", "rotated-host-key", before
			files = applyFixture(t, files, plan(t, "disconnect", files, options, true))
			for index, file := range files {
				if file.Exists != before[index].Exists || !bytes.Equal(file.Body, before[index].Body) {
					t.Fatalf("original configuration was not restored after host changes: %s", file.Path)
				}
			}
		})
	}
}

func TestReceiptUsesHashesAndRefusesMissingOrDamagedOwnership(t *testing.T) {
	options := fixtureOptions()
	files := applyFixture(t, fixtureFiles(nil), plan(t, "connect", fixtureFiles(nil), options, false))
	bodies := map[string]string{}
	for _, file := range files {
		bodies[file.Path] = string(file.Body)
	}
	receipt := bodies[ReceiptPath]
	if strings.Contains(receipt, options.Key) || strings.Contains(receipt, options.Gateway) {
		t.Fatal("receipt duplicated the endpoint or credential")
	}
	for _, damaged := range []string{"", "{}", `{"schema":1,"fields":{}}`, strings.Replace(receipt, `"schema": 1`, `"schema": 2`, 1)} {
		broken := map[string]string{}
		for path, body := range bodies {
			if path != ReceiptPath || damaged != "" {
				broken[path] = body
			}
		}
		if damaged != "" {
			broken[ReceiptPath] = damaged
		}
		candidates := fixtureFiles(broken)
		if _, err := inspect(t, candidates, options); err == nil {
			t.Fatal("inspection accepted missing or damaged ownership evidence")
		}
		options.Previous = fixtureFiles(nil)
		for _, action := range []string{"sync", "disconnect"} {
			if _, err := planAgent(t.Context(), sdk.Call{Params: raw(sdk.AgentPlanRequest{
				ID: AgentID, Action: action, Settings: raw(settingsEnvelope{Files: candidates}),
				Options: raw(options), Binding: fixtureBinding(true),
			})}, nil); err == nil {
				t.Fatalf("%s guessed missing or damaged ownership evidence", action)
			}
		}
	}
}

func TestDisconnectPreservesEditedCredentialAfterHostRotation(t *testing.T) {
	before := fixtureFiles(nil)
	options := fixtureOptions()
	files := applyFixture(t, before, plan(t, "connect", before, options, false))
	bodies := map[string]string{}
	for _, file := range files {
		bodies[file.Path] = string(file.Body)
	}
	profile, _ := readObject([]byte(bodies[ProfilePath]))
	profile["inferenceGatewayApiKey"] = raw("user-edited-key")
	bodies[ProfilePath] = string(raw(profile))
	options.Key, options.Previous = "new-host-key", before
	files = applyFixture(t, fixtureFiles(bodies), plan(t, "disconnect", fixtureFiles(bodies), options, true))
	values, err := readFiles(raw(settingsEnvelope{Files: files}))
	if err != nil || !same(values[ProfilePath].values["inferenceGatewayApiKey"], raw("user-edited-key")) {
		t.Fatal("disconnect erased a key edited after connection", err)
	}
}

func TestSealedKeyDoesNotAuthorizeHostAddressOrKeyChange(t *testing.T) {
	options := fixtureOptions()
	files := applyFixture(t, fixtureFiles(nil), plan(t, "connect", fixtureFiles(nil), options, false))
	bodies := map[string]string{}
	for _, file := range files {
		bodies[file.Path] = string(file.Body)
	}
	profile, _ := readObject([]byte(bodies[ProfilePath]))
	profile["inferenceGatewayApiKey"] = raw("__sealed__:v1:b3BhcXVlLWtleQ==")
	bodies[ProfilePath] = string(raw(profile))
	files = fixtureFiles(bodies)
	for _, changed := range []agentOptions{
		{Gateway: options.Gateway, Key: "new-host-key"},
		{Gateway: "http://127.0.0.1:7000", Key: options.Key},
	} {
		if _, err := inspect(t, files, changed); err == nil {
			t.Fatal("opaque key was treated as authority for a new host credential or destination")
		}
	}
}

func TestInvalidSnapshotsFailBeforeAnyPlan(t *testing.T) {
	for name, mutate := range map[string]func([]fileSnapshot) []fileSnapshot{
		"missing-file":   func(files []fileSnapshot) []fileSnapshot { return files[1:] },
		"duplicate-path": func(files []fileSnapshot) []fileSnapshot { files[1] = files[0]; return files },
		"foreign-path": func(files []fileSnapshot) []fileSnapshot {
			files[0].Path = ".config/magpie/providers.json"
			return files
		},
		"forged-hash": func(files []fileSnapshot) []fileSnapshot { files[0].BeforeHash = strings.Repeat("0", 64); return files },
		"absent-body": func(files []fileSnapshot) []fileSnapshot { files[0].Body = []byte("{}"); return files },
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := snapshots(mutate(fixtureFiles(nil))); err == nil {
				t.Fatal("invalid host snapshot accepted")
			}
		})
	}
	for _, body := range []string{"null", "[]", "{} {}", "{\"deploymentMode\":\"1p\",\"deploymentMode\":\"3p\"}"} {
		if _, err := snapshots(fixtureFiles(map[string]string{ConfigPath: body})); err == nil {
			t.Fatalf("invalid configuration accepted: %s", body)
		}
	}
	for _, body := range []string{"{\"entries\":null}", "{\"entries\":[{\"id\":\"same\"},{\"id\":\"same\"}]}", "{\"entries\":[[]]}"} {
		if _, err := snapshots(fixtureFiles(map[string]string{MetaPath: body})); err == nil {
			t.Fatalf("invalid config-library metadata accepted: %s", body)
		}
	}
}

func TestGatewayEndpointUsesDesktopIdentityAndHostCredential(t *testing.T) {
	for _, address := range []string{"http://127.0.0.1:3425", "http://[::1]:3425/", "http://localhost:3425", "http://LOCALHOST:3425", "https://gateway.example:443", "https://[2001:db8::1]:65535"} {
		t.Run(address, func(t *testing.T) {
			actual, err := gatewayURL(address)
			if err != nil || actual != strings.TrimRight(address, "/")+"/agent/claude-desktop" {
				t.Fatalf("valid endpoint %s: %s %v", address, actual, err)
			}
		})
	}
	for _, address := range []string{
		"http://127.0.0.2:3425", "http://127.0.0.1.nip.io:3425", "http://[::ffff:127.0.0.1]:3425",
		"http://[localhost]:3425", "http://[127.0.0.1]:3425", "http://localhost:65536",
		"https://[gateway.example]:443", "https://[::1%25lo0]:443", "https://2001:db8::1:443", "https://gateway.example:99999",
		"https://:443",
		"http://gateway.example", "https://user:secret@host", "https://host/#ignored",
		"https://host/?key=secret", "https://host/v1", "file:///tmp/key", "https://host?",
	} {
		t.Run(address, func(t *testing.T) {
			if _, err := gatewayURL(address); err == nil {
				t.Fatalf("unsupported Desktop endpoint accepted: %s", address)
			}
		})
	}
}

func TestCancellationAndBindingAreChecked(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := planAgent(ctx, sdk.Call{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %v", err)
	}
	if _, err := inspectAgent(ctx, sdk.Call{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("inspection cancellation ignored: %v", err)
	}
	for _, binding := range []*sdk.AgentBindingSnapshot{nil, {Target: "anthropic", Revision: "1", Connected: true, Values: map[string]string{}}, {Target: TargetID, Revision: "01", Values: map[string]string{}}} {
		if err := checkBinding(AgentID, binding); err == nil {
			t.Fatal("foreign or malformed binding accepted")
		}
	}
}
