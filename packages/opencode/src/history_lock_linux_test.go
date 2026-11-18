package opencode

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
	"golang.org/x/sys/unix"
)

func TestHistoryPOSIXFailedAcquisitionLeavesNoReaderOrPendingLock(t *testing.T) {
	for _, shm := range []bool{false, true} {
		t.Run(fmt.Sprint("shm=", shm), func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "locked")
			if err := os.WriteFile(name, []byte("history"), 0o600); err != nil {
				t.Fatal(err)
			}
			start, length := int64(0x40000002), int64(510)
			if shm {
				start, length = 123, 5
			}
			writerContext, cancelWriter := context.WithCancel(context.Background())
			command := exec.CommandContext(writerContext, "python3", "-c", `
import fcntl, os, sys
fd = os.open(sys.argv[1], os.O_RDWR)
fcntl.lockf(fd, fcntl.LOCK_EX | fcntl.LOCK_NB, int(sys.argv[3]), int(sys.argv[2]))
print("locked", flush=True)
sys.stdin.readline()
os.close(fd)
`, name, fmt.Sprint(start), fmt.Sprint(length))
			input, err := command.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			output, err := command.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				defer cancelWriter()
				input.Close()
				if err := command.Wait(); err != nil {
					t.Error("external writer lock failed", err)
				}
			})
			lines := bufio.NewScanner(output)
			if !lines.Scan() || lines.Text() != "locked" {
				t.Fatal("external writer did not acquire its record lock", lines.Err())
			}
			file, err := os.Open(name)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			locks := newHistoryLockSet(true)
			if err := locks.change(file, shm, true); !errors.Is(err, unix.EACCES) && !errors.Is(err, unix.EAGAIN) {
				t.Fatal("reader did not refuse a conflicting writer", err)
			}
			if err := locks.change(file, shm, false); err != nil || len(locks.inodes) != 0 {
				t.Fatal("failed acquisition retained an inode reader", err)
			}
			if !shm && !historyExternalLockAvailable(t, name, 0x40000000, 1) {
				t.Fatal("failed shared acquisition retained the pending database lock")
			}
		})
	}
}

func historyExternalLockAvailable(t *testing.T, name string, start, length int64) bool {
	t.Helper()
	command := exec.CommandContext(t.Context(), "python3", "-c", `
import errno, fcntl, os, sys
fd = os.open(sys.argv[1], os.O_RDWR)
try:
    fcntl.lockf(fd, fcntl.LOCK_EX | fcntl.LOCK_NB, int(sys.argv[3]), int(sys.argv[2]))
except OSError as error:
    if error.errno not in (errno.EACCES, errno.EAGAIN):
        raise
    print("held")
else:
    print("available")
finally:
    os.close(fd)
`, name, fmt.Sprint(start), fmt.Sprint(length))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatal("external record-lock probe failed", string(output), err)
	}
	switch string(output) {
	case "held\n":
		return false
	case "available\n":
		return true
	default:
		t.Fatal("unexpected external lock probe", string(output))
		return false
	}
}

func TestHistoryPOSIXReadersAndProbesRetainExternalLocks(t *testing.T) {
	for _, shm := range []bool{false, true} {
		t.Run(fmt.Sprint("shm=", shm), func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "inode")
			if err := os.WriteFile(name, []byte("history"), 0o600); err != nil {
				t.Fatal(err)
			}
			locks := newHistoryLockSet(true)
			first, err := os.Open(name)
			if err != nil {
				t.Fatal(err)
			}
			defer locks.close(first)
			second, err := os.Open(name)
			if err != nil {
				t.Fatal(err)
			}
			defer locks.close(second)
			probe, err := os.Open(name)
			if err != nil {
				t.Fatal(err)
			}
			start, length := int64(0x40000002), int64(510)
			if shm {
				start, length = 123, 5
			}
			for _, file := range []*os.File{first, second} {
				if err := locks.change(file, shm, true); err != nil {
					t.Fatal(err)
				}
			}
			if err := locks.close(probe); err != nil {
				t.Fatal(err)
			}
			if historyExternalLockAvailable(t, name, start, length) {
				t.Fatal("probe close dropped process-owned SQLite read locks")
			}
			if err := locks.change(first, shm, false); err != nil {
				t.Fatal(err)
			}
			if err := locks.close(first); err != nil {
				t.Fatal(err)
			}
			if historyExternalLockAvailable(t, name, start, length) {
				t.Fatal("closing one reader released the other reader's SQLite locks")
			}
			if err := locks.change(second, shm, false); err != nil {
				t.Fatal(err)
			}
			if !historyExternalLockAvailable(t, name, start, length) || len(locks.inodes) != 0 {
				t.Fatal("final reader leaked record locks or pending descriptors")
			}
		})
	}
}

type historyProcessWriter struct {
	input io.WriteCloser
	lines *bufio.Scanner
}

func (writer historyProcessWriter) command(t *testing.T, command string) map[string]any {
	t.Helper()
	if err := json.NewEncoder(writer.input).Encode(command); err != nil {
		t.Fatal(err)
	}
	return writer.reply(t)
}

func (writer historyProcessWriter) reply(t *testing.T) map[string]any {
	t.Helper()
	if !writer.lines.Scan() {
		t.Fatal("SQLite writer ended before its reply", writer.lines.Err())
	}
	var reply map[string]any
	if err := json.Unmarshal(writer.lines.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	return reply
}

func TestHistoryPOSIXRealWALReadersSurviveProbeCloseAndWriterReset(t *testing.T) {
	directory := t.TempDir()
	writerContext, cancelWriter := context.WithCancel(context.Background())
	command := exec.CommandContext(writerContext, "python3", "../tests/storage_writer.py", directory)
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		defer cancelWriter()
		json.NewEncoder(input).Encode("stop")
		input.Close()
		if err := command.Wait(); err != nil {
			t.Error("fixture SQLite writer failed", err, stderr.String())
		}
	})
	writer := historyProcessWriter{input: input, lines: bufio.NewScanner(output)}
	initial := writer.reply(t)
	if initial["mainOnly"] != float64(0) {
		t.Fatal("fixture did not preserve WAL-only committed history", initial)
	}
	owned, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer owned.Close()
	root := &historyTestRoot{root: owned, limit: 64 << 20}
	locks := newHistoryLockSet(true)
	open := func() *historySQL {
		t.Helper()
		vfs, err := newHistoryVFSWithLocks(root, locks)
		if err != nil {
			t.Fatal(err)
		}
		reader, err := openHistorySQLVFS(context.Background(), root, vfs)
		if err != nil {
			t.Fatal("POSIX rooted WAL transaction failed", err)
		}
		return reader
	}
	first, second := open(), open()
	defer first.Close()
	defer second.Close()
	lease := sdk.StorageReadLease{Limits: sdk.DefaultStorageLimits()}
	entries, _, err := first.catalog(t.Context(), lease)
	if err != nil || len(entries) != 2 {
		t.Fatal("WAL reader lost committed V2/V1 or dedup semantics", entries, err)
	}
	state := writer.command(t, "state")
	for _, member := range []string{"db", "wal", "shm"} {
		if initial[member] != state[member] {
			t.Fatal("read-only POSIX readers modified source", member)
		}
	}
	writer.command(t, "append")
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if busy := writer.command(t, "checkpoint")["busy"]; busy != float64(1) {
		t.Fatal("closing one VFS/probe released another live WAL snapshot", busy)
	}
	var title string
	if err := second.tx.QueryRow("SELECT title FROM session_v2 WHERE id='ses_wal'").Scan(&title); err != nil || title != "Committed WAL session" {
		t.Fatal("writer append crossed the retained read transaction", title, err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if busy := writer.command(t, "checkpoint")["busy"]; busy != float64(0) {
		t.Fatal("completed POSIX reader leaked WAL-reset locks", busy)
	}
	next := open()
	defer next.Close()
	if err := next.tx.QueryRow("SELECT title FROM session_v2 WHERE id='ses_wal'").Scan(&title); err != nil || title != "Updated WAL session" {
		t.Fatal("fresh transaction lost the reset WAL commit", title, err)
	}
}
