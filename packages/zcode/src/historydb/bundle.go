package historydb

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
)

type bundle struct {
	Kind    string `json:"kind"`
	Version int    `json:"version"`
	SHA256  string `json:"sha256"`
	Source  []byte `json:"source"`
}

func Convert(ctx context.Context, from, to string, body []byte) ([]byte, string, error) {
	if from == Format && to == Bundle {
		document, err := Read(ctx, body)
		if err != nil {
			return nil, "", err
		}
		if document.summary.Kind == "archived-controller" {
			return nil, "", ErrSnapshot
		}
		result, err := json.Marshal(bundle{Kind: Bundle, Version: 1, SHA256: document.summary.SHA256, Source: document.source})
		return result, document.summary.SHA256, err
	}
	if from != Bundle || to != Format || len(body) > MaxBytes*2 {
		return nil, "", ErrSnapshot
	}
	if _, err := protocol.ReadObject(body); err != nil {
		return nil, "", ErrSnapshot
	}
	var value bundle
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF ||
		value.Kind != Bundle || value.Version != 1 || value.SHA256 != digest(value.Source) {
		return nil, "", ErrSnapshot
	}
	document, err := Read(ctx, value.Source)
	if err != nil {
		return nil, "", err
	}
	if document.summary.Kind == "archived-controller" {
		return nil, "", ErrSnapshot
	}
	return document.Source(), document.summary.SHA256, nil
}
