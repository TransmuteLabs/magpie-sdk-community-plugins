package zcode

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func automaticExternalLock(t *testing.T, name string, start, length int64) bool {
	t.Helper()
	command := exec.CommandContext(t.Context(), "python3", "-I", "-S", "-c", `
import errno, fcntl, os, sys
descriptor = os.open(sys.argv[1], os.O_RDWR)
try:
    fcntl.lockf(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB, int(sys.argv[3]), int(sys.argv[2]))
except OSError as error:
    if error.errno not in (errno.EACCES, errno.EAGAIN):
        raise
    print("held")
else:
    print("available")
finally:
    os.close(descriptor)
`, name, fmt.Sprint(start), fmt.Sprint(length))
	result, err := command.CombinedOutput()
	if err != nil {
		t.Fatal("external SQLite lock probe:", err, string(result))
	}
	if string(result) != "held\n" && string(result) != "available\n" {
		t.Fatal("unexpected SQLite lock probe:", string(result))
	}
	return string(result) == "available\n"
}

func TestAutomaticHistoryPOSIXProbesAndOtherReadersRetainLocks(t *testing.T) {
	for _, shm := range []bool{false, true} {
		t.Run(fmt.Sprint(shm), func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "inode")
			if err := os.WriteFile(name, []byte("synthetic inode"), 0600); err != nil {
				t.Fatal(err)
			}
			locks := newHistoryLockSet(true)
			files := []*os.File{}
			for range 3 {
				file, err := os.Open(name)
				if err != nil {
					t.Fatal(err)
				}
				files = append(files, file)
			}
			t.Cleanup(func() {
				for _, file := range files {
					file.Close()
				}
			})
			for _, file := range files[:2] {
				if err := locks.change(file, shm, true); err != nil {
					t.Fatal(err)
				}
			}
			if err := locks.close(files[2]); err != nil {
				t.Fatal(err)
			}
			start, length := int64(0x40000002), int64(510)
			if shm {
				start, length = 123, 5
			}
			if automaticExternalLock(t, name, start, length) {
				t.Fatal("closing an identity probe released the live POSIX reader lock")
			}
			if err := locks.change(files[0], shm, false); err != nil {
				t.Fatal(err)
			}
			if err := locks.close(files[0]); err != nil {
				t.Fatal(err)
			}
			if automaticExternalLock(t, name, start, length) {
				t.Fatal("closing one SQLite reader released another reader's lock")
			}
			if err := locks.change(files[1], shm, false); err != nil {
				t.Fatal(err)
			}
			if !automaticExternalLock(t, name, start, length) || len(locks.inodes) != 0 {
				t.Fatal("final SQLite reader leaked a lock or retained descriptor")
			}
		})
	}
}
