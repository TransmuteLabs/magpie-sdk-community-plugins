package opencode

import (
	"fmt"
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestHistoryCompletedCatalogsReleaseSnapshotCapacity(t *testing.T) {
	writer, root, _ := historyDatabaseFixture(t)
	historyInsertSession(t, writer, "ses_wal")
	directory, err := root.root.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	module := NewWindowModule(ModuleOptions{})
	harness := serveHarness(t, module, raw(moduleConfig{ProjectID: "global"}))
	harness.storage = directory
	for index := range 40 {
		request := sdk.StorageRequest{Lease: sdk.StorageReadLease{SourceID: HistorySourceID, RootRef: "root",
			SnapshotRef: fmt.Sprintf("snapshot-%d", index), FamilyID: "history", Limits: sdk.DefaultStorageLimits()}, Limit: 256}
		frame, _ := harness.invoke(sdk.MethodStorageList, sdk.Scope{}, request, nil)
		if frame.Error != nil {
			t.Fatal("completed catalog retained a pending snapshot slot", index, frame.Error)
		}
		result, err := sdk.DecodeStorageResult(request, frame.Result)
		if err != nil || result.NextCursor != "" || len(result.Entries) != 1 {
			t.Fatal("complete catalog did not publish its single WAL session", result, err)
		}
	}
	historyInsertSession(t, writer, "ses_z")
	request := sdk.StorageRequest{Lease: sdk.StorageReadLease{SourceID: HistorySourceID, RootRef: "root",
		SnapshotRef: "paged", FamilyID: "history", Limits: sdk.DefaultStorageLimits()}, Limit: 1}
	first, _ := harness.invoke(sdk.MethodStorageList, sdk.Scope{}, request, nil)
	page, err := sdk.DecodeStorageResult(request, first.Result)
	if first.Error != nil || err != nil || page.NextCursor == "" || len(page.Entries) != 1 {
		t.Fatal("first page did not retain its bounded catalog", first.Error, err)
	}
	historyInsertSession(t, writer, "ses_later")
	request.Cursor = page.NextCursor
	last, _ := harness.invoke(sdk.MethodStorageList, sdk.Scope{}, request, nil)
	page, err = sdk.DecodeStorageResult(request, last.Result)
	if last.Error != nil || err != nil || page.NextCursor != "" || len(page.Entries) != 1 || page.Entries[0].Locator.SessionID != "ses_z" {
		t.Fatal("pagination mixed a later database commit into its catalog", last.Error, page, err)
	}
	replay, _ := harness.invoke(sdk.MethodStorageList, sdk.Scope{}, request, nil)
	if replay.Error == nil {
		t.Fatal("completed pagination retained a replayable snapshot")
	}
}

func TestHistoryCatalogRejectsOversizedStoredMetadata(t *testing.T) {
	for _, column := range []string{"id", "title", "directory", "parent_id"} {
		t.Run(column, func(t *testing.T) {
			writer, root, _ := historyDatabaseFixture(t)
			historyInsertSession(t, writer, "ses_bounds")
			limit := 4096
			if column == "id" || column == "parent_id" {
				limit = 128
			}
			if _, err := writer.Exec("UPDATE session_v2 SET "+column+"=?", strings.Repeat("x", limit+1)); err != nil {
				t.Fatal(err)
			}
			reader, err := openHistorySQL(t.Context(), root)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			if _, _, err := reader.catalog(t.Context(), sdk.StorageReadLease{RootRef: "root", Limits: sdk.DefaultStorageLimits()}); err == nil {
				t.Fatal("catalog allocated and published oversized stored metadata", column)
			}
		})
	}
}
