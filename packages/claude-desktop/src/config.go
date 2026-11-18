package claudedesktop

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type fileSnapshot struct {
	Path       string `json:"path"`
	Body       []byte `json:"body"`
	BeforeHash string `json:"beforeHash"`
	Exists     bool   `json:"exists"`
	Mode       uint32 `json:"mode"`
}

type settingsEnvelope struct {
	Files []fileSnapshot `json:"files"`
}

type agentOptions struct {
	Gateway        string            `json:"gateway"`
	Key            string            `json:"key"`
	Models         []json.RawMessage `json:"models"`
	PriorModels    []json.RawMessage `json:"priorModels,omitempty"`
	DecisionModels []json.RawMessage `json:"decisionModels,omitempty"`
	Field          string            `json:"field"`
	Value          string            `json:"value"`
	Previous       []fileSnapshot    `json:"previous"`
}

type configFile struct {
	snapshot fileSnapshot
	values   map[string]json.RawMessage
}

func decode(body []byte, target any) error {
	if len(body) > 16<<20 || len(bytes.TrimSpace(body)) == 0 {
		return invalid()
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil || decoder.Decode(new(any)) != io.EOF {
		return invalid()
	}
	return nil
}

func readObject(body []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return nil, invalid()
	}
	values := map[string]json.RawMessage{}
	for decoder.More() {
		key, err := decoder.Token()
		name, valid := key.(string)
		if err != nil || !valid {
			return nil, invalid()
		}
		if _, duplicate := values[name]; duplicate {
			return nil, invalid()
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil, invalid()
		}
		values[name] = value
	}
	if end, err := decoder.Token(); err != nil || end != json.Delim('}') || decoder.Decode(new(any)) != io.EOF {
		return nil, invalid()
	}
	return values, nil
}

func readFiles(body []byte) (map[string]configFile, error) {
	var settings settingsEnvelope
	if decode(body, &settings) != nil {
		return nil, invalid()
	}
	return snapshots(settings.Files)
}

func snapshots(files []fileSnapshot) (map[string]configFile, error) {
	paths := Manifest().Agents[0].ConfigFiles
	if len(files) != len(paths) {
		return nil, invalid()
	}
	result := map[string]configFile{}
	for _, snapshot := range files {
		if _, duplicate := result[snapshot.Path]; duplicate || !slices.Contains(paths, snapshot.Path) || len(snapshot.Body) > 4<<20 {
			return nil, invalid()
		}
		file := configFile{snapshot: snapshot, values: map[string]json.RawMessage{}}
		if snapshot.Exists {
			digest := sha256.Sum256(snapshot.Body)
			if snapshot.BeforeHash != hex.EncodeToString(digest[:]) {
				return nil, invalid()
			}
			var err error
			file.values, err = readObject(snapshot.Body)
			if err != nil {
				return nil, err
			}
		} else if snapshot.BeforeHash != "missing" || len(snapshot.Body) != 0 {
			return nil, invalid()
		}
		result[snapshot.Path] = file
	}
	if _, err := metaEntries(result[MetaPath].values); err != nil {
		return nil, err
	}
	if result[ReceiptPath].snapshot.Exists {
		if _, err := receiptFields(result[ReceiptPath].values); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func readOptions(body []byte) (agentOptions, error) {
	var options agentOptions
	if decode(body, &options) != nil || options.Field != "" || options.Value != "" || len(options.Key) == 0 || len(options.Key) > 8192 || strings.ContainsAny(options.Key, "\r\n\x00") {
		return options, invalid()
	}
	if _, err := gatewayURL(options.Gateway); err != nil {
		return options, err
	}
	return options, nil
}

func gatewayURL(address string) (string, error) {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.Opaque != "" || parsed.RawPath != "" || parsed.Path != "" && parsed.Path != "/" {
		return "", invalid()
	}
	hostname := parsed.Hostname()
	ip, _ := netip.ParseAddr(hostname)
	if ip.Zone() != "" {
		return "", invalid()
	}
	if port := parsed.Port(); port != "" {
		if _, err := strconv.ParseUint(port, 10, 16); err != nil {
			return "", invalid()
		}
	}
	localHTTP := strings.EqualFold(hostname, "localhost") || hostname == "127.0.0.1" || ip == netip.IPv6Loopback()
	if parsed.Scheme != "https" && (parsed.Scheme != "http" || !localHTTP) {
		return "", invalid()
	}
	return strings.TrimRight(parsed.String(), "/") + "/agent/" + TargetID, nil
}

func profileValues(options agentOptions) map[string]json.RawMessage {
	address, _ := gatewayURL(options.Gateway)
	return map[string]json.RawMessage{
		"inferenceProvider":             raw("gateway"),
		"inferenceCredentialKind":       raw("static"),
		"inferenceGatewayBaseUrl":       raw(address),
		"inferenceGatewayApiKey":        raw(options.Key),
		"inferenceGatewayAuthScheme":    raw("bearer"),
		"disableDeploymentModeChooser":  raw(true),
		"desktopHome":                   raw("standard"),
		"isClaudeCodeForDesktopEnabled": raw(true),
	}
}

func same(left, right json.RawMessage) bool {
	var compactLeft, compactRight bytes.Buffer
	return json.Compact(&compactLeft, left) == nil && json.Compact(&compactRight, right) == nil && bytes.Equal(compactLeft.Bytes(), compactRight.Bytes())
}

func sealedKey(value json.RawMessage) bool {
	var text string
	if json.Unmarshal(value, &text) != nil || !strings.HasPrefix(text, "__sealed__:v1:") {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(text, "__sealed__:v1:"))
	return err == nil && len(decoded) != 0
}

func fieldHash(value json.RawMessage) string {
	var compact bytes.Buffer
	if json.Compact(&compact, value) != nil {
		return ""
	}
	digest := sha256.Sum256(compact.Bytes())
	return hex.EncodeToString(digest[:])
}

func receiptFields(values map[string]json.RawMessage) (map[string]string, error) {
	encoded, err := readObject(values["fields"])
	if len(values) != 2 || !same(values["schema"], raw(1)) || err != nil {
		return nil, configurationChanged()
	}
	expected := profileValues(agentOptions{})
	if len(encoded) != len(expected) {
		return nil, configurationChanged()
	}
	fields := map[string]string{}
	for key := range expected {
		var value string
		if json.Unmarshal(encoded[key], &value) != nil {
			return nil, configurationChanged()
		}
		fields[key] = value
		digest, err := hex.DecodeString(fields[key])
		if err != nil || len(digest) != sha256.Size || fields[key] != strings.ToLower(fields[key]) {
			return nil, configurationChanged()
		}
	}
	return fields, nil
}

func checkProfileMode(profile map[string]json.RawMessage) error {
	if mode, exists := profile["deploymentMode"]; exists && !same(mode, raw("any")) && !same(mode, raw("3p")) {
		return configurationChanged()
	}
	return nil
}

func checkConnection(files map[string]configFile, options agentOptions) (bool, error) {
	if _, exists := files[MetaPath].values["hybridPointer"]; exists {
		return false, configurationChanged()
	}
	applied, err := receiptFields(files[ReceiptPath].values)
	if err != nil {
		return false, err
	}
	profile := files[ProfilePath].values
	if err := checkProfileMode(profile); err != nil {
		return false, err
	}
	for key, expected := range profileValues(options) {
		if applied[key] != fieldHash(expected) {
			return false, configurationChanged()
		}
		if key == "inferenceGatewayApiKey" && sealedKey(profile[key]) {
			continue
		}
		if !same(profile[key], expected) {
			return false, configurationChanged()
		}
	}
	if !same(files[MetaPath].values["appliedId"], raw(ProfileID)) {
		return false, configurationChanged()
	}
	return sealedKey(profile["inferenceGatewayApiKey"]), nil
}

func configurationChanged() error {
	return &sdk.Error{Code: "configuration_changed", Message: "Claude Desktop uses a different local profile or gateway; reconnect explicitly or disconnect to restore the previous selection", Outcome: "not-started"}
}
