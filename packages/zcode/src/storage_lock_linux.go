package zcode

import (
	"os"

	"golang.org/x/sys/unix"
)

const historyPlatformPOSIX = false

func historyOFDSet(file *os.File, lock *unix.Flock_t) error {
	return unix.FcntlFlock(file.Fd(), unix.F_OFD_SETLK, lock)
}

func historyOFDGet(file *os.File, lock *unix.Flock_t) error {
	return unix.FcntlFlock(file.Fd(), unix.F_OFD_GETLK, lock)
}
