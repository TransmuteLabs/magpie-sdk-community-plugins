package claudedesktop

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func metaEntries(values map[string]json.RawMessage) ([]json.RawMessage, error) {
	entries := []json.RawMessage{}
	if value, exists := values["entries"]; exists {
		if json.Unmarshal(value, &entries) != nil || entries == nil || len(entries) > 4096 {
			return nil, invalid()
		}
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		fields, err := readObject(entry)
		var id string
		if err != nil || json.Unmarshal(fields["id"], &id) != nil || id == "" || seen[id] {
			return nil, invalid()
		}
		seen[id] = true
	}
	return entries, nil
}

func planAgent(ctx context.Context, call sdk.Call, _ *sdk.Stream) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var request sdk.AgentPlanRequest
	if decode(call.Params, &request) != nil || checkBinding(request.ID, request.Binding) != nil {
		return nil, invalid()
	}
	files, err := readFiles(request.Settings)
	if err != nil {
		return nil, err
	}
	options, err := readOptions(request.Options)
	if err != nil {
		return nil, err
	}
	change := &sdk.AgentBindingChange{Revision: request.Binding.Revision, Connected: request.Binding.Connected, Values: maps.Clone(request.Binding.Values)}
	switch request.Action {
	case "disconnect":
		if !request.Binding.Connected {
			return sdk.AgentPlan{Files: []sdk.FileChange{}, Binding: change}, nil
		}
		previous, err := snapshots(options.Previous)
		if err != nil {
			return nil, err
		}
		change.Connected = false
		return disconnect(files, previous, change)
	case "connect":
		if _, exists := files[MetaPath].values["hybridPointer"]; exists {
			return nil, configurationChanged()
		}
		if err := checkProfileMode(files[ProfilePath].values); err != nil {
			return nil, err
		}
		for _, path := range []string{ConfigPath, ThirdPartyConfigPath} {
			files[path].values["deploymentMode"] = raw("3p")
		}
		change.Connected = true
	case "sync":
		if !request.Binding.Connected {
			return sdk.AgentPlan{Files: []sdk.FileChange{}, Binding: change}, nil
		}
		if _, err := checkConnection(files, options); err != nil {
			return nil, err
		}
	default:
		return nil, invalid()
	}
	applied := map[string]string{}
	for key, value := range profileValues(options) {
		applied[key] = fieldHash(value)
		if key == "inferenceGatewayApiKey" && request.Action == "sync" && sealedKey(files[ProfilePath].values[key]) {
			continue
		}
		files[ProfilePath].values[key] = value
	}
	receipt := files[ReceiptPath]
	receipt.values = map[string]json.RawMessage{"schema": raw(1), "fields": raw(applied)}
	files[ReceiptPath] = receipt
	entries, _ := metaEntries(files[MetaPath].values)
	found := false
	for _, entry := range entries {
		var identity struct {
			ID string `json:"id"`
		}
		json.Unmarshal(entry, &identity)
		found = found || identity.ID == ProfileID
	}
	if !found {
		entries = append(entries, raw(map[string]string{"id": ProfileID, "name": "Magpie SDK"}))
	}
	files[MetaPath].values["entries"] = raw(entries)
	files[MetaPath].values["appliedId"] = raw(ProfileID)
	return changes(files, nil, change)
}

func restoreField(current, previous map[string]json.RawMessage, key string, expected json.RawMessage) {
	if !same(current[key], expected) {
		return
	}
	if value, exists := previous[key]; exists {
		current[key] = value
	} else {
		delete(current, key)
	}
}

func disconnect(files, previous map[string]configFile, binding *sdk.AgentBindingChange) (sdk.AgentPlan, error) {
	applied, err := receiptFields(files[ReceiptPath].values)
	if err != nil {
		return sdk.AgentPlan{}, err
	}
	for _, path := range []string{ConfigPath, ThirdPartyConfigPath} {
		restoreField(files[path].values, previous[path].values, "deploymentMode", raw("3p"))
	}
	for key, digest := range applied {
		if fieldHash(files[ProfilePath].values[key]) == digest {
			if value, exists := previous[ProfilePath].values[key]; exists {
				files[ProfilePath].values[key] = value
			} else {
				delete(files[ProfilePath].values, key)
			}
		}
	}
	receipt := files[ReceiptPath]
	receipt.values = maps.Clone(previous[ReceiptPath].values)
	files[ReceiptPath] = receipt
	meta := files[MetaPath].values
	restoreField(meta, previous[MetaPath].values, "appliedId", raw(ProfileID))
	entries, err := metaEntries(meta)
	if err != nil {
		return sdk.AgentPlan{}, err
	}
	priorEntries, _ := metaEntries(previous[MetaPath].values)
	generated := raw(map[string]string{"id": ProfileID, "name": "Magpie SDK"})
	restored := []json.RawMessage{}
	for _, entry := range entries {
		fields, _ := readObject(entry)
		if !same(fields["id"], raw(ProfileID)) || !sameObject(entry, generated) {
			restored = append(restored, entry)
			continue
		}
		for _, candidate := range priorEntries {
			before, _ := readObject(candidate)
			if same(before["id"], raw(ProfileID)) {
				restored = append(restored, candidate)
			}
		}
	}
	if len(restored) == 0 && previous[MetaPath].values["entries"] == nil {
		delete(meta, "entries")
	} else {
		meta["entries"] = raw(restored)
	}
	return changes(files, previous, binding)
}

func sameObject(left, right []byte) bool {
	leftObject, leftError := readObject(left)
	rightObject, rightError := readObject(right)
	return leftError == nil && rightError == nil && maps.EqualFunc(leftObject, rightObject, same)
}

func changes(files, previous map[string]configFile, binding *sdk.AgentBindingChange) (sdk.AgentPlan, error) {
	result := sdk.AgentPlan{Files: []sdk.FileChange{}, Binding: binding}
	for _, path := range Manifest().Agents[0].ConfigFiles {
		file := files[path]
		if !file.snapshot.Exists && len(file.values) == 0 {
			continue
		}
		body, err := json.MarshalIndent(file.values, "", "  ")
		if err != nil {
			return sdk.AgentPlan{}, err
		}
		body = append(body, '\n')
		remove := previous != nil && !previous[path].snapshot.Exists && len(file.values) == 0
		if previous != nil && previous[path].snapshot.Exists && sameObject(body, previous[path].snapshot.Body) {
			body = previous[path].snapshot.Body
		}
		if remove && !file.snapshot.Exists || !remove && file.snapshot.Exists && (bytes.Equal(body, file.snapshot.Body) || sameObject(body, file.snapshot.Body)) {
			continue
		}
		change := sdk.FileChange{Path: path, BeforeHash: file.snapshot.BeforeHash, Remove: remove}
		if !remove {
			change.Body, change.Mode = body, 0o600
		}
		result.Files = append(result.Files, change)
	}
	return result, nil
}
