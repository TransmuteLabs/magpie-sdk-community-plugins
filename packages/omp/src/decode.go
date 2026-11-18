package ompsessions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Decode preserves every journal byte. Items is the donor's portable active
// conversation projection, not a replacement for the original journal.
func Decode(ctx context.Context, journal []byte, assets []Asset) (Document, error) {
	if len(journal) == 0 || len(journal) > MaxBytes || len(assets) >= MaxFiles {
		return Document{}, errors.New("OMP journal or asset count exceeds its budget")
	}
	store := map[string][]byte{}
	total := len(journal)
	for _, asset := range assets {
		if !validHash(asset.SHA256) || hashBytes(asset.Body) != asset.SHA256 {
			return Document{}, errors.New("OMP asset digest mismatch")
		}
		if _, exists := store[asset.SHA256]; exists {
			return Document{}, errors.New("duplicate OMP asset")
		}
		total += len(asset.Body)
		if total > MaxBytes {
			return Document{}, errors.New("OMP inputs exceed their byte budget")
		}
		store[asset.SHA256] = asset.Body
	}
	document := Document{Schema: 1, Items: []Item{}}
	header := false
	ids := map[string]bool{}
	var parseErr error
	lineNumber := 0
	err := eachLine(ctx, journal, func(line []byte) {
		lineNumber++
		if parseErr != nil {
			return
		}
		var envelope struct {
			Type      string          `json:"type"`
			ID        string          `json:"id"`
			Version   int             `json:"version"`
			Cwd       string          `json:"cwd"`
			Title     string          `json:"title"`
			Timestamp string          `json:"timestamp"`
			Message   json.RawMessage `json:"message"`
		}
		if json.Unmarshal(line, &envelope) != nil || envelope.Type == "" {
			parseErr = fmt.Errorf("invalid OMP record at line %d", lineNumber)
			return
		}
		if err := checkAttachmentReferences(envelope.Type, envelope.Message, store); err != nil {
			parseErr = fmt.Errorf("OMP line %d: %w", lineNumber, err)
			return
		}
		switch envelope.Type {
		case "session":
			at, err := time.Parse(time.RFC3339Nano, envelope.Timestamp)
			if header || envelope.Version != 3 || !validUUID(envelope.ID) || err != nil {
				parseErr = errors.New("expected exactly one OMP v3 session header with UUID and timestamp")
				return
			}
			header = true
			document.ID, document.Cwd, document.Timestamp = envelope.ID, envelope.Cwd, at
			if envelope.Title != "" {
				document.Title = envelope.Title
			}
		case "title", "title_change":
			if envelope.Type == "title_change" && envelope.ID != "" {
				if ids[envelope.ID] {
					parseErr = fmt.Errorf("duplicate OMP title-change entry ID at line %d", lineNumber)
					return
				}
				ids[envelope.ID] = true
			}
			if envelope.Title != "" {
				document.Title = envelope.Title
			}
		default:
			if envelope.ID == "" || ids[envelope.ID] {
				parseErr = fmt.Errorf("OMP record requires a unique entry ID at line %d", lineNumber)
				return
			}
			ids[envelope.ID] = true
		}
	})
	if err != nil {
		return Document{}, err
	}
	if parseErr != nil {
		return Document{}, parseErr
	}
	if !header {
		return Document{}, errors.New("missing OMP v3 session header")
	}
	items, err := readItems(ctx, journal, store)
	if err != nil {
		return Document{}, err
	}
	if items != nil {
		document.Items = items
	}
	document.Archive = &Archive{Journal: bytes.Clone(journal), JournalSHA256: hashBytes(journal)}
	var hashes []string
	for hash := range store {
		hashes = append(hashes, hash)
	}
	sort.Strings(hashes)
	for _, hash := range hashes {
		document.Archive.Assets = append(document.Archive.Assets, Asset{SHA256: hash, Body: bytes.Clone(store[hash])})
	}
	document.Archive.ProjectionSHA256 = projectionHash(document)
	return document, nil
}

// OMP's carry reader resolves content image blocks and fileMention.files[].image
// (ompCarryMessageItems/ompCarryBlobs.image in the archived donor). Inspect
// those fields on every journal message, before selecting the active branch.
// Text, tool arguments and unrelated metadata remain opaque journal bytes.
func checkAttachmentReferences(kind string, body json.RawMessage, assets map[string][]byte) error {
	return visitAttachmentReferences(kind, body, func(hash string) error {
		if _, exists := assets[hash]; !exists {
			return fmt.Errorf("unresolved OMP blob %s; supply blobs/%s explicitly", hash, hash)
		}
		return nil
	})
}

func visitAttachmentReferences(kind string, body json.RawMessage, visit func(string) error) error {
	if kind != "message" {
		return nil
	}
	var message struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
		Files   json.RawMessage `json:"files"`
	}
	if json.Unmarshal(body, &message) != nil {
		return nil
	}
	switch message.Role {
	case "user", "assistant", "toolResult":
		var blocks []json.RawMessage
		if json.Unmarshal(message.Content, &blocks) != nil {
			return nil // A string content value is ordinary text.
		}
		for _, block := range blocks {
			var image struct {
				Type string `json:"type"`
				Data string `json:"data"`
			}
			if json.Unmarshal(block, &image) == nil && image.Type == "image" {
				if err := visitImageReference(image.Data, visit); err != nil {
					return err
				}
			}
		}
	case "fileMention":
		var files []struct {
			Image json.RawMessage `json:"image"`
		}
		if json.Unmarshal(message.Files, &files) != nil {
			return nil
		}
		for _, file := range files {
			var image struct {
				Data string `json:"data"`
			}
			if json.Unmarshal(file.Image, &image) == nil {
				if err := visitImageReference(image.Data, visit); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func visitImageReference(data string, visit func(string) error) error {
	if len(data) < 5 || !strings.EqualFold(data[:5], "blob:") {
		return nil
	}
	hash, ok := strings.CutPrefix(data, "blob:sha256:")
	if !ok || !validHash(hash) {
		return fmt.Errorf("invalid OMP blob reference %q", data)
	}
	return visit(hash)
}
