package devin

import (
	"context"
	"sync"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/outbound"
)

type modelSelectionIndex struct {
	mu    sync.Mutex
	names map[string]bool
	full  bool
}

func (index *modelSelectionIndex) remember(models []codec.NativeModel) {
	index.mu.Lock()
	defer index.mu.Unlock()
	if index.names == nil {
		index.names = map[string]bool{}
	}
	for _, model := range models {
		if !model.Virtual {
			continue
		}
		if len(index.names) >= 16384 {
			index.full = true
			return
		}
		index.names[model.UID] = true
	}
}

func (index *modelSelectionIndex) needsCatalog(model, effort string) bool {
	if effort != "" || codec.IsNativeModelSelection(model) {
		return true
	}
	index.mu.Lock()
	defer index.mu.Unlock()
	return index.full || index.names[model]
}

func (owner *providerOwner) resolveModel(ctx context.Context, client *outbound.Client, credentials outbound.Credentials,
	state *providerState, model, effort string) (string, error) {
	if !state.modelSelections.needsCatalog(model, effort) {
		return model, nil
	}
	models, err := client.Models(ctx, credentials)
	if err != nil {
		return "", moduleFailure(err, "not-started")
	}
	state.modelSelections.remember(codec.ProjectNativeModels(models))
	selected, err := codec.ResolveNativeModel(models, model, effort)
	if err != nil {
		return "", &outbound.Error{Operation: "models", Code: "invalid_params", Outcome: "not-started"}
	}
	return selected, nil
}

func (owner *providerOwner) resolveEncodedModel(ctx context.Context, client *outbound.Client, credentials outbound.Credentials,
	state *providerState, encoded codec.EncodedChat) (codec.EncodedChat, error) {
	model, err := owner.resolveModel(ctx, client, credentials, state, encoded.Model, encoded.Effort)
	if err != nil {
		return codec.EncodedChat{}, moduleFailure(err, "not-started")
	}
	if model != encoded.Model {
		encoded.Payload, err = codec.ReplaceNativeAuthority(encoded.Payload, model, credentials.Key)
		if err != nil {
			return codec.EncodedChat{}, moduleFailure(err, "not-started")
		}
	}
	return encoded, nil
}
