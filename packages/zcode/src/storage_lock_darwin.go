package zcode

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

const historyPlatformPOSIX = true

func historyOFDSet(*os.File, *unix.Flock_t) error {
	return errors.New("Darwin storage requires coordinated POSIX locks")
}

func historyOFDGet(*os.File, *unix.Flock_t) error {
	return errors.New("Darwin storage requires coordinated POSIX locks")
}
