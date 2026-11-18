package devin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	sdk "github.com/yetone/magpie/pkg/extension"
)

type originReceipt struct {
	Kind        string `json:"kind"`
	Digest      string `json:"digest"`
	Format      string `json:"format,omitempty"`
	Model       string `json:"model"`
	ActualModel string `json:"actualModel,omitempty"`
	Caller      string `json:"caller"`
	Generation  uint64 `json:"generation"`
}

type originBucket struct {
	Receipts []originReceipt `json:"receipts"`
}
type signatureHash struct {
	hash hash.Hash
	size int
}
type originCollector struct {
	signatures map[string]*signatureHash
	gemini     map[string]bool
	format     string
	actual     string
}

func originDenied() error {
	return &sdk.Error{Code: "permission_denied", Message: "Native signed history has no matching Devin account, caller and model origin", Outcome: "not-started"}
}

func loadOrigins(ctx context.Context, stream *sdk.Stream, key string) (originBucket, string, error) {
	var value sdk.StateValue
	if err := stream.HostCall(ctx, sdk.MethodStateGet, sdk.StateGetRequest{Key: key}, &value); err != nil {
		return originBucket{}, "", err
	}
	var bucket originBucket
	if value.Found && (json.Unmarshal(value.Value, &bucket) != nil || len(bucket.Receipts) > 256) {
		return originBucket{}, "", originDenied()
	}
	return bucket, value.Version, nil
}

func verifyOrigins(ctx context.Context, stream *sdk.Stream, call sdk.Call, options sdk.ExecuteOptions, model string, claims []codec.NativeClaim) error {
	if len(claims) == 0 {
		return nil
	}
	if options.CallerID == "" || len(claims) > 2048 {
		return originDenied()
	}
	buckets := map[string]originBucket{}
	for _, claim := range claims {
		if len(claim.Digest) != 64 {
			return originDenied()
		}
		key := "origin." + claim.Digest[:1]
		bucket, found := buckets[key]
		if !found {
			var err error
			bucket, _, err = loadOrigins(ctx, stream, key)
			if err != nil {
				return err
			}
			buckets[key] = bucket
		}
		valid := false
		for _, receipt := range bucket.Receipts {
			if receipt.Digest == claim.Digest && receipt.Kind == claim.Kind && receipt.Format == claim.Format &&
				receipt.Generation == call.Scope.Generation && receipt.Caller == options.CallerID && receipt.Model == model &&
				(claim.Model == "" || claim.Model == receipt.ActualModel || claim.Model == receipt.Model) {
				valid = true
				break
			}
		}
		if !valid {
			return originDenied()
		}
	}
	return nil
}

func resolveMessagesSignature(ctx context.Context, stream *sdk.Stream, call sdk.Call, options sdk.ExecuteOptions,
	model, signature string) (codec.NativeSignature, error) {
	if options.CallerID == "" {
		return codec.NativeSignature{}, originDenied()
	}
	digest := codec.SignatureDigest([]byte(signature))
	bucket, _, err := loadOrigins(ctx, stream, "origin."+digest[:1])
	if err != nil {
		return codec.NativeSignature{}, err
	}
	origin := codec.NativeSignature{}
	for _, receipt := range bucket.Receipts {
		if receipt.Kind == "signature" && receipt.Digest == digest && receipt.Caller == options.CallerID &&
			receipt.Generation == call.Scope.Generation && receipt.Model == model && receipt.Format != "" {
			candidate := codec.NativeSignature{Format: receipt.Format, Model: receipt.ActualModel}
			if origin.Format != "" && origin != candidate {
				return codec.NativeSignature{}, originDenied()
			}
			origin = candidate
		}
	}
	if origin.Format == "" {
		return codec.NativeSignature{}, originDenied()
	}
	return origin, nil
}

func (collector *originCollector) add(payload []byte) error {
	delta, err := codec.DecodeOriginDelta(payload)
	if err != nil {
		return err
	}
	if delta.Format != "" {
		if collector.format != "" && collector.format != delta.Format {
			return originDenied()
		}
		collector.format = delta.Format
	}
	if delta.ActualModel != "" {
		if collector.actual != "" && collector.actual != delta.ActualModel {
			return originDenied()
		}
		collector.actual = delta.ActualModel
	}
	if len(delta.Signature) > 0 {
		if collector.signatures == nil {
			collector.signatures = map[string]*signatureHash{}
		}
		entry := collector.signatures[delta.ThinkingID]
		if entry == nil {
			if len(collector.signatures) >= 256 {
				return originDenied()
			}
			entry = &signatureHash{hash: sha256.New()}
			collector.signatures[delta.ThinkingID] = entry
		}
		entry.size += len(delta.Signature)
		if entry.size > 16<<20 {
			return originDenied()
		}
		entry.hash.Write(delta.Signature)
	}
	if len(delta.Gemini) > 0 {
		if collector.gemini == nil {
			collector.gemini = map[string]bool{}
		}
		if len(collector.gemini) >= 256 {
			return originDenied()
		}
		collector.gemini[codec.SignatureDigest(delta.Gemini)] = true
	}
	return nil
}

func (collector *originCollector) persist(ctx context.Context, stream *sdk.Stream, call sdk.Call, options sdk.ExecuteOptions, model string) error {
	if collector.format == "gemini" && collector.actual == "" {
		collector.actual = model
	}
	var pending []originReceipt
	makeReceipt := func(kind, digest string) originReceipt {
		return originReceipt{Kind: kind, Digest: digest, Format: collector.format, Model: model, ActualModel: collector.actual,
			Caller: options.CallerID, Generation: call.Scope.Generation}
	}
	for _, entry := range collector.signatures {
		pending = append(pending, makeReceipt("signature", hex.EncodeToString(entry.hash.Sum(nil))))
	}
	for digest := range collector.gemini {
		pending = append(pending, makeReceipt("gemini", digest))
	}
	if len(pending) == 0 {
		return nil
	}
	if options.CallerID == "" {
		return originDenied()
	}
	return persistOriginRecords(ctx, stream, pending)
}

func persistOriginRecords(ctx context.Context, stream *sdk.Stream, pending []originReceipt) error {
	byBucket := map[string][]originReceipt{}
	for _, receipt := range pending {
		key := "origin." + receipt.Digest[:1]
		byBucket[key] = append(byBucket[key], receipt)
	}
	for key, receipts := range byBucket {
		saved := false
		for attempt := 0; attempt < 8; attempt++ {
			bucket, version, err := loadOrigins(ctx, stream, key)
			if err != nil {
				return err
			}
			bucket.Receipts = append(bucket.Receipts, receipts...)
			if len(bucket.Receipts) > 256 {
				bucket.Receipts = bucket.Receipts[len(bucket.Receipts)-256:]
			}
			value, _ := json.Marshal(bucket)
			var result sdk.StateCASResult
			if err := stream.HostCall(ctx, sdk.MethodStateCAS, sdk.StateCASRequest{Key: key, Version: version, Value: value}, &result); err != nil {
				return err
			}
			if result.Swapped {
				saved = true
				break
			}
		}
		if !saved {
			return &sdk.Error{Code: "conflict", Message: "Native signature origins could not be committed", Outcome: "uncertain"}
		}
	}
	return nil
}
