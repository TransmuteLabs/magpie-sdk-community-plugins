package controller

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func fixtureExecution() Execution {
	return Execution{Account: "account", Login: "login-fingerprint", Generation: 1,
		Task: "task", Lane: "offpeak", Model: "GLM-5.3", Operation: "operation", Reference: "reference", State: "unknown"}
}

func TestPendingAdmissionSurvivesRestartAndCredentialGeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	principal := Principal("https://zcode.z.ai", "synthetic-user")
	key := EntryKey("account", "task")
	err = store.Locked(t.Context(), principal, func(transaction *Transaction) error {
		transaction.Journal.Executions[key] = fixtureExecution()
		return transaction.Save()
	})
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err = store.Locked(t.Context(), principal, func(transaction *Transaction) error {
		entry := transaction.Journal.Executions[key]
		if entry.State != "unknown" || entry.Generation != 1 {
			t.Fatal("unconfirmed admission was lost or reset by restart")
		}
		entry.Generation = 2
		transaction.Journal.Executions[key] = entry
		return transaction.Save()
	})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(path, principal+".json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("vendor journal is not private")
	}
}

func TestCorruptStateIsNeverReplacedAsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	principal := Principal("https://zcode.z.ai", "synthetic-user")
	file := filepath.Join(path, principal+".json")
	if os.WriteFile(file, []byte("{broken"), 0600) != nil {
		t.Fatal("fixture write")
	}
	called := false
	err = store.Locked(t.Context(), principal, func(*Transaction) error { called = true; return nil })
	body, _ := os.ReadFile(file)
	if err == nil || called || string(body) != "{broken" {
		t.Fatal("corrupt vendor state became an empty spend allowance")
	}
}

func TestConcurrentControllersDoNotLoseClaims(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	principal := Principal("https://zcode.z.ai", "synthetic-user")
	var wait sync.WaitGroup
	errors := make(chan error, 2)
	for index, store := range []*Store{first, second} {
		wait.Add(1)
		go func(index int, store *Store) {
			defer wait.Done()
			errors <- store.Locked(t.Context(), principal, func(transaction *Transaction) error {
				entry := fixtureExecution()
				if index == 1 {
					entry.Task = "second-task"
				}
				transaction.Journal.Executions[EntryKey(entry.Account, entry.Task)] = entry
				return transaction.Save()
			})
		}(index, store)
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := first.Locked(t.Context(), principal, func(transaction *Transaction) error {
		if len(transaction.Journal.Executions) != 2 {
			t.Fatal("concurrent controllers overwrote an admission")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPrincipalAndJournalCannotBeReboundByCallback(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err = store.Locked(t.Context(), Principal("https://zcode.z.ai", "one"), func(transaction *Transaction) error {
		transaction.Journal.Principal = Principal("https://zcode.z.ai", "two")
		return transaction.Save()
	})
	if err == nil {
		t.Fatal("a callback rebound the principal under another lock")
	}
}

func TestOSLockProtectsAcrossProcesses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	err = store.Locked(t.Context(), Principal("https://zcode.z.ai", "lock-user"), func(*Transaction) error {
		child := exec.CommandContext(t.Context(), binary, "-test.run=^TestJournalProcessHelper$")
		child.Env = append(os.Environ(), "ZCODE_JOURNAL_FIXTURE="+path)
		output, err := child.CombinedOutput()
		if err != nil {
			t.Fatalf("child bypassed the cross-process lock: %s", output)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestJournalProcessHelper(t *testing.T) {
	path := os.Getenv("ZCODE_JOURNAL_FIXTURE")
	if path == "" {
		return
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	entered := false
	err = store.Locked(ctx, Principal("https://zcode.z.ai", "lock-user"), func(*Transaction) error { entered = true; return nil })
	if !errors.Is(err, context.DeadlineExceeded) || entered {
		t.Fatal("another process acquired the held journal")
	}
}
