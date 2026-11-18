package controller

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeIntentSurvivesReopenAlongsideFinancialOwners(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "private")
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	principal, workspace, request := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	key := NativeKey(workspace, "session", "command")
	err = store.Locked(t.Context(), principal, func(transaction *Transaction) error {
		transaction.Journal.Claims["plan"] = Claim{Plan: "plan", Operation: "claim-operation",
			Reference: "claim-reference", State: "unknown", Generation: 1}
		transaction.Journal.Reset["account"] = Reset{Pending: &ResetClaim{Key: "reset", Operation: "reset-operation",
			Window: "FIVE_HOUR", Generation: 1}}
		transaction.Journal.Native = map[string]NativeCommand{key: {
			Account: "account", Generation: 1, Operation: "native-operation", Workspace: workspace,
			Session: "session", Command: "command", RequestHash: request, State: "pending",
		}}
		return transaction.Save()
	})
	store.Close()
	if err != nil {
		t.Fatal(err)
	}
	store, err = Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Locked(context.Background(), principal, func(transaction *Transaction) error {
		command := transaction.Journal.Native[key]
		if command.State != "pending" || command.Operation != "native-operation" ||
			transaction.Journal.Claims["plan"].State != "unknown" ||
			transaction.Journal.Reset["account"].Pending.Operation != "reset-operation" {
			t.Fatal("native intent or financial uncertainty disappeared after reopen", transaction.Journal)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
