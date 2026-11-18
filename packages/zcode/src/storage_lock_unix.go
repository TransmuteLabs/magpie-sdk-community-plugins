//go:build linux || darwin

package zcode

import (
	"errors"
	"io"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

type historyInode struct {
	device uint64
	number uint64
}

type historyInodeLocks struct {
	db       int
	shm      int
	deferred []*os.File
}

type historyLockSet struct {
	mu     sync.Mutex
	posix  bool
	inodes map[historyInode]*historyInodeLocks
}

var historyNativeLocks = newHistoryLockSet(historyPlatformPOSIX)

func newHistoryLockSet(posix bool) *historyLockSet {
	return &historyLockSet{posix: posix, inodes: map[historyInode]*historyInodeLocks{}}
}

func historyFileInode(file *os.File) (historyInode, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		return historyInode{}, err
	}
	return historyInode{device: uint64(stat.Dev), number: uint64(stat.Ino)}, nil
}

func (locks *historyLockSet) set(file *os.File, kind int16, start, length int64) error {
	lock := unix.Flock_t{Type: kind, Whence: int16(io.SeekStart), Start: start, Len: length}
	if locks.posix {
		return unix.FcntlFlock(file.Fd(), unix.F_SETLK, &lock)
	}
	return historyOFDSet(file, &lock)
}

func (locks *historyLockSet) reserved(file *os.File) (bool, error) {
	lock := unix.Flock_t{Type: unix.F_RDLCK, Whence: int16(io.SeekStart), Start: 0x40000001, Len: 1}
	var err error
	if locks.posix {
		err = unix.FcntlFlock(file.Fd(), unix.F_GETLK, &lock)
	} else {
		err = historyOFDGet(file, &lock)
	}
	return lock.Type != unix.F_UNLCK, err
}

func (locks *historyLockSet) change(file *os.File, shm, acquire bool) error {
	locks.mu.Lock()
	defer locks.mu.Unlock()
	var state *historyInodeLocks
	var inode historyInode
	if locks.posix {
		var err error
		inode, err = historyFileInode(file)
		if err != nil {
			return err
		}
		state = locks.inodes[inode]
		if state == nil && !acquire {
			return nil
		}
		if state != nil {
			readers := &state.db
			if shm {
				readers = &state.shm
			}
			if acquire && *readers > 0 {
				*readers++
				return nil
			}
			if !acquire && *readers != 1 {
				if *readers > 1 {
					*readers--
				}
				return nil
			}
		}
	}
	kind := int16(unix.F_UNLCK)
	if acquire {
		kind = unix.F_RDLCK
	}
	start, length := int64(0x40000002), int64(510)
	if shm {
		start, length = 123, 5
	} else if acquire {
		if err := locks.set(file, unix.F_RDLCK, 0x40000000, 1); err != nil {
			return err
		}
	}
	err := locks.set(file, kind, start, length)
	if !shm && acquire {
		pendingErr := locks.set(file, unix.F_UNLCK, 0x40000000, 1)
		if err == nil && pendingErr != nil {
			err = locks.set(file, unix.F_UNLCK, start, length)
		}
		err = errors.Join(err, pendingErr)
	}
	if err != nil {
		return err
	}
	if locks.posix {
		if state == nil {
			state = &historyInodeLocks{}
			locks.inodes[inode] = state
		}
		readers := &state.db
		if shm {
			readers = &state.shm
		}
		if acquire {
			*readers++
		} else {
			*readers--
		}
		if state.db == 0 && state.shm == 0 {
			for _, pending := range state.deferred {
				err = errors.Join(err, pending.Close())
			}
			delete(locks.inodes, inode)
		}
	}
	return err
}

func (locks *historyLockSet) shared(file *os.File, acquire bool) error {
	return locks.change(file, false, acquire)
}

func (locks *historyLockSet) shm(file *os.File, acquire bool) error {
	return locks.change(file, true, acquire)
}

func (locks *historyLockSet) close(file *os.File) error {
	locks.mu.Lock()
	defer locks.mu.Unlock()
	if locks.posix {
		inode, err := historyFileInode(file)
		if err != nil {
			return err
		}
		if state := locks.inodes[inode]; state != nil && (state.db > 0 || state.shm > 0) {
			state.deferred = append(state.deferred, file)
			return nil
		}
	}
	return file.Close()
}
