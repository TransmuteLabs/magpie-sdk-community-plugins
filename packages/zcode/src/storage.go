package zcode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/extension/native"
)

const storageSourceID = "zcode-native-history"

type historyRoot interface {
	Open(string) (*os.File, error)
	Consume(int64) error
	Family() sdk.StorageFamily
}

func historyDescriptor() sdk.StorageSourceDescriptor {
	return sdk.StorageSourceDescriptor{ID: storageSourceID, Agent: "zcode", Codec: "zcode-sqlite-v1",
		Roots: []sdk.SessionSourceStore{{Slot: "cli", HomeRelativePath: ".zcode/cli/db"}},
		Families: []sdk.StorageFamily{{ID: "cli", Primary: "db.sqlite",
			Sidecars: []string{"db.sqlite-wal", "db.sqlite-shm", "db.sqlite-journal"}}},
		Transcript: true, Resume: "agent",
		AgentResume: &sdk.SessionResumeRecipe{Agent: AgentID, Args: []string{"--resume", "{sessionId}"}}}
}

func historyDigest(value any) string {
	body, _ := json.Marshal(value)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func (reader *historySQL) locator(lease sdk.StorageReadLease, id string) sdk.StorageSessionLocator {
	return sdk.StorageSessionLocator{RootRef: lease.RootRef, SessionID: id,
		Key: "sqlite:" + historyDigest([]string{lease.SourceID, lease.RootRef, lease.FamilyID, reader.incarnation, id})}
}

func historyPage(request sdk.StorageRequest, entries []sdk.StorageEntry, cut bool) (sdk.StorageResult, error) {
	binding := historyDigest(struct {
		Lease   sdk.StorageReadLease
		Entries []sdk.StorageEntry
		Cut     bool
	}{request.Lease, entries, cut})
	offset := 0
	if request.Cursor != "" {
		prefix, position, valid := strings.Cut(request.Cursor, ":")
		var err error
		offset, err = strconv.Atoi(position)
		if !valid || prefix != binding || err != nil || offset < 1 || offset >= len(entries) ||
			position != strconv.Itoa(offset) {
			return sdk.StorageResult{}, errors.New("stale or foreign ZCode storage cursor")
		}
	}
	end := min(offset+request.Limit, len(entries))
	result := sdk.StorageResult{SnapshotRef: request.Lease.SnapshotRef, Entries: entries[offset:end], Cut: cut}
	if end < len(entries) {
		result.NextCursor = binding + ":" + strconv.Itoa(end)
	}
	return result, nil
}

func automaticHistory(ctx context.Context, call sdk.Call, stream *sdk.Stream) (response any, readErr error) {
	request, err := sdk.DecodeStorageRequest(ControlManifest(), call.Method, call.Scope, call.Auth, call.Params)
	if err != nil {
		return nil, invalid("Automatic ZCode history requires its exact selected storage lease")
	}
	if stream == nil {
		return nil, invalid("Automatic ZCode history requires its rooted storage broker")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := stream.CheckStorage(ctx, request.Lease); err != nil {
		return nil, err
	}
	root, err := native.OpenStorage(ctx, stream, request.Lease)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := root.Close(); err != nil {
			response, readErr = nil, errors.Join(readErr, err)
		}
	}()
	reader, err := openHistorySQL(ctx, root, request.Lease.Limits)
	if err != nil {
		return nil, err
	}
	result, err := reader.result(ctx, request)
	closeErr := reader.Close()
	if err != nil || closeErr != nil {
		return nil, errors.Join(err, closeErr)
	}
	if err := stream.CheckStorage(ctx, request.Lease); err != nil {
		return nil, err
	}
	return result, nil
}
