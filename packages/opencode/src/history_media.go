package opencode

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func (projection *historyProjection) attachment(ctx context.Context, reader *historySQL, message historyMessage, role string, file map[string]json.RawMessage) error {
	mime, name, uri := historyString(file, "mime"), historyString(file, "name"), historyString(file, "uri")
	if mime == "" {
		mime = "application/octet-stream"
	}
	if name == "" {
		name = historyString(file, "filename")
	}
	if uri == "" {
		uri = historyString(file, "url")
	}
	var bytes []byte
	if strings.HasPrefix(uri, "media:") && reader != nil && reader.tables["blob"] && reader.tables["blob_ref"] {
		hash := strings.TrimPrefix(uri, "media:")
		decoded, err := hex.DecodeString(hash)
		if err != nil || len(decoded) != sha256.Size || strings.ToLower(hash) != hash {
			return errors.New("malformed OpenCode media reference")
		}
		var stored, length int64
		var storedMIME string
		err = reader.tx.QueryRowContext(ctx, "SELECT b.bytes,length(b.data),b.mime FROM blob b WHERE b.hash=? AND EXISTS (SELECT 1 FROM blob_ref r WHERE r.hash=b.hash AND r.session_id=? AND r.message_id=?)",
			hash, projection.request.Locator.SessionID, message.id).Scan(&stored, &length, &storedMIME)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil {
			if stored < 0 || stored != length || stored > int64(projection.request.Lease.Limits.MaxMediaBytes) {
				return errors.New("OpenCode blob exceeds media budget or stored length")
			}
			if storedMIME != mime {
				return errors.New("OpenCode blob MIME mismatch")
			}
			for _, item := range projection.result.Media {
				if item.ID == "media:"+hash {
					if item.MIME != mime || int64(len(item.Bytes)) != stored {
						return errors.New("OpenCode repeated blob metadata changed")
					}
					projection.add(sdk.SessionSourcePart{Role: role, Kind: "image", Name: item.ID,
						Text: fmt.Sprintf("[Attached %s: %s]", mime, name)})
					return nil
				}
			}
			if stored > int64(projection.request.Lease.Limits.MaxMediaBytes-projection.media) {
				return errors.New("OpenCode media aggregate budget exceeded")
			}
			err = reader.tx.QueryRowContext(ctx, "SELECT data FROM blob WHERE hash=? AND length(data)=?", hash, stored).Scan(&bytes)
			if err != nil {
				return err
			}
			digest := sha256.Sum256(bytes)
			if hex.EncodeToString(digest[:]) != hash || storedMIME != mime {
				return errors.New("OpenCode blob hash or MIME mismatch")
			}
		}
	} else {
		encoded := historyString(file, "data")
		if strings.HasPrefix(uri, "data:"+mime+";base64,") {
			encoded = strings.TrimPrefix(uri, "data:"+mime+";base64,")
		}
		if encoded != "" {
			if base64.StdEncoding.DecodedLen(len(encoded)) > projection.request.Lease.Limits.MaxMediaBytes+2 {
				return errors.New("OpenCode inline media budget exceeded")
			}
			var err error
			bytes, err = base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				return errors.New("malformed OpenCode inline media")
			}
		}
	}
	if bytes == nil {
		label := mime
		if name != "" {
			label = name + " (" + mime + ")"
		}
		projection.add(sdk.SessionSourcePart{Role: role, Kind: "text", Text: "[media unavailable: " + label + "]"})
		return nil
	}
	hash := sha256.Sum256(bytes)
	id := "media:" + hex.EncodeToString(hash[:])
	for _, item := range projection.result.Media {
		if item.ID == id {
			projection.add(sdk.SessionSourcePart{Role: role, Kind: "image", Name: id,
				Text: fmt.Sprintf("[Attached %s: %s]", mime, name)})
			return nil
		}
	}
	projection.media += len(bytes)
	if projection.media > projection.request.Lease.Limits.MaxMediaBytes {
		return errors.New("OpenCode media aggregate budget exceeded")
	}
	projection.result.Media = append(projection.result.Media, sdk.StorageMedia{ID: id, Name: name, MIME: mime, Bytes: bytes})
	projection.add(sdk.SessionSourcePart{Role: role, Kind: "image", Name: id, Text: fmt.Sprintf("[Attached %s: %s]", mime, name)})
	return nil
}
