package ompsessions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync/atomic"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type sourceReadRecord struct {
	owned, enabled bool
}

type sourceReadPermission struct {
	instance, version string
	lifetime          context.Context
	ownOff            bool
	record            sourceReadRecord
	valid             bool
	closed            atomic.Bool
}

func sourcePreviewStart(ctx context.Context, value sdk.ModuleContext) (sdk.ModuleReady, func(), error) {
	permission := &sourceReadPermission{instance: value.Instance, version: value.Config.Version, lifetime: ctx}
	permission.ownOff, permission.record, permission.valid = sourceReadConfiguration(value.Config.Values)
	return sdk.ModuleReady{State: permission}, func() { permission.closed.Store(true) }, nil
}

func sourceReadConfiguration(raw json.RawMessage) (bool, sourceReadRecord, bool) {
	root, err := sourcePolicyObject(raw, nil)
	if err != nil || len(root["sessionSource"]) > 8192 {
		return false, sourceReadRecord{}, false
	}
	policy, err := sourcePolicyObject(root["sessionSource"], []string{"ownOff", "records"})
	if err != nil {
		return false, sourceReadRecord{}, false
	}
	ownOff, valid := sourcePolicyBool(policy["ownOff"])
	if !valid {
		return false, sourceReadRecord{}, false
	}
	var records []json.RawMessage
	if json.Unmarshal(policy["records"], &records) != nil || len(records) != 1 {
		return ownOff, sourceReadRecord{}, false
	}
	record, err := sourcePolicyObject(records[0], []string{"sourceId", "owned", "enabled"})
	if err != nil {
		return ownOff, sourceReadRecord{}, false
	}
	var id string
	if json.Unmarshal(record["sourceId"], &id) != nil || id != SourceDescriptorID {
		return ownOff, sourceReadRecord{}, false
	}
	owned, ownedValid := sourcePolicyBool(record["owned"])
	enabled, enabledValid := sourcePolicyBool(record["enabled"])
	return ownOff, sourceReadRecord{owned: owned, enabled: enabled}, ownedValid && enabledValid
}

func sourcePolicyBool(raw json.RawMessage) (bool, bool) {
	var value *bool
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return false, false
	}
	return *value, true
}

func sourcePolicyObject(raw json.RawMessage, allowed []string) (map[string]json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > sdk.MaxHostBytes {
		return nil, errors.New("invalid Source policy object")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("Source policy requires an object")
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		key, stringKey := token.(string)
		if err != nil || !stringKey || len(fields) >= 64 {
			return nil, errors.New("invalid Source policy field")
		}
		if _, duplicate := fields[key]; duplicate {
			return nil, errors.New("duplicate Source policy field")
		}
		if allowed != nil {
			found := false
			for _, name := range allowed {
				found = found || name == key
			}
			if !found {
				return nil, errors.New("unknown Source policy field")
			}
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil, errors.New("invalid Source policy value")
		}
		fields[key] = value
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, errors.New("invalid Source policy object end")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("trailing Source policy data")
	}
	return fields, nil
}

func currentSourceReadPermission(ctx context.Context) (*sourceReadPermission, error) {
	value, found := sdk.ModuleContextFrom(ctx)
	permission, typed := value.State.(*sourceReadPermission)
	if !found || !typed || permission == nil || value.Component != sdk.PrimaryComponent ||
		value.Instance != permission.instance || value.Config.Version != permission.version {
		return nil, sourceReadDenied()
	}
	if err := permission.check(ctx); err != nil {
		return nil, err
	}
	return permission, nil
}

func (permission *sourceReadPermission) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if permission.closed.Load() || permission.lifetime.Err() != nil || !permission.valid ||
		permission.ownOff || !permission.record.owned || !permission.record.enabled {
		return sourceReadDenied()
	}
	return nil
}

func sourceReadDenied() error {
	return &sdk.Error{Code: "permission_denied", Message: "OMP Source read is not permitted by the module record"}
}
