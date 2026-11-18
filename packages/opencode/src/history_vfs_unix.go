//go:build linux || darwin

package opencode

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
	"modernc.org/libc"
	sqlite3 "modernc.org/sqlite/lib"
)

type historyVFS struct {
	root   historyRoot
	name   string
	cname  uintptr
	cvfs   uintptr
	token  uintptr
	tls    *libc.TLS
	open   int
	closed bool
	locks  *historyLockSet
	probes map[string]*os.File
}

type historyVFSFile struct {
	methods uintptr
	token   uintptr
}

type historyOpenFile struct {
	vfs       *historyVFS
	file      *os.File
	name      string
	shm       *os.File
	index     []byte
	locked    bool
	shmLocked bool
}

var historyVFSObjects = struct {
	sync.Mutex
	next  uintptr
	roots map[uintptr]*historyVFS
	files map[uintptr]*historyOpenFile
}{roots: map[uintptr]*historyVFS{}, files: map[uintptr]*historyOpenFile{}}

var historyVFSIO = sqlite3.Tsqlite3_io_methods{
	FiVersion: 2,
	FxClose:   vfsFunction(historyVFSCloseFile), FxRead: vfsFunction(historyVFSRead),
	FxWrite: vfsFunction(historyVFSWrite), FxTruncate: vfsFunction(historyVFSTruncate),
	FxSync: vfsFunction(historyVFSSync), FxFileSize: vfsFunction(historyVFSSize),
	FxLock: vfsFunction(historyVFSLock), FxUnlock: vfsFunction(historyVFSUnlock),
	FxCheckReservedLock: vfsFunction(historyVFSReserved), FxFileControl: vfsFunction(historyVFSControl),
	FxSectorSize: vfsFunction(historyVFSSector), FxDeviceCharacteristics: vfsFunction(historyVFSDevice),
	FxShmMap: vfsFunction(historyVFSShmMap), FxShmLock: vfsFunction(historyVFSShmLock),
	FxShmBarrier: vfsFunction(historyVFSShmBarrier), FxShmUnmap: vfsFunction(historyVFSShmUnmap),
}

func vfsFunction(function any) uintptr {
	return (*[2]uintptr)(unsafe.Pointer(&function))[1]
}

func newHistoryVFS(root historyRoot) (*historyVFS, error) {
	return newHistoryVFSWithLocks(root, historyNativeLocks)
}

func newHistoryVFSWithLocks(root historyRoot, locks *historyLockSet) (*historyVFS, error) {
	tls := libc.NewTLS()
	base := sqlite3.Xsqlite3_vfs_find(tls, 0)
	if base == 0 {
		tls.Close()
		return nil, errors.New("SQLite default VFS is unavailable")
	}
	historyVFSObjects.Lock()
	defer historyVFSObjects.Unlock()
	historyVFSObjects.next++
	token := historyVFSObjects.next
	name := fmt.Sprintf("opencode_history_%x", token)
	cname, err := libc.CString(name)
	if err != nil {
		tls.Close()
		return nil, err
	}
	cvfs := libc.Xcalloc(tls, 1, libc.Tsize_t(unsafe.Sizeof(sqlite3.Tsqlite3_vfs{})))
	if cvfs == 0 {
		libc.Xfree(tls, cname)
		tls.Close()
		return nil, errors.New("SQLite VFS allocation failed")
	}
	value := new(sqlite3.Tsqlite3_vfs)
	valueBytes := unsafe.Slice((*byte)(unsafe.Pointer(value)), int(unsafe.Sizeof(*value)))
	copy(valueBytes, libc.GoBytes(base, len(valueBytes)))
	value.FzName, value.FpAppData, value.FpNext = cname, token, 0
	value.FszOsFile, value.FmxPathname = int32(unsafe.Sizeof(historyVFSFile{})), 1024
	value.FxOpen, value.FxDelete = vfsFunction(historyVFSOpen), vfsFunction(historyVFSDelete)
	value.FxAccess, value.FxFullPathname = vfsFunction(historyVFSAccess), vfsFunction(historyVFSPath)
	value.FxDlOpen, value.FxDlSym, value.FxDlClose, value.FxDlError = 0, 0, 0, 0
	value.FxSetSystemCall, value.FxGetSystemCall, value.FxNextSystemCall = 0, 0, 0
	copy(libc.GoBytes(cvfs, len(valueBytes)), valueBytes)
	vfs := &historyVFS{root: root, name: name, cname: cname, cvfs: cvfs, token: token, tls: tls,
		locks: locks, probes: map[string]*os.File{}}
	historyVFSObjects.roots[token] = vfs
	if code := sqlite3.Xsqlite3_vfs_register(tls, cvfs, 0); code != sqlite3.SQLITE_OK {
		delete(historyVFSObjects.roots, token)
		libc.Xfree(tls, cvfs)
		libc.Xfree(tls, cname)
		tls.Close()
		return nil, fmt.Errorf("SQLite VFS registration failed: %d", code)
	}
	return vfs, nil
}

func (vfs *historyVFS) Close() error {
	historyVFSObjects.Lock()
	defer historyVFSObjects.Unlock()
	if vfs.closed {
		return nil
	}
	if vfs.open != 0 {
		return errors.New("SQLite VFS still has an active read transaction")
	}
	var closeErr error
	for _, file := range vfs.probes {
		closeErr = errors.Join(closeErr, vfs.locks.close(file))
	}
	vfs.probes = nil
	vfs.closed = true
	code := sqlite3.Xsqlite3_vfs_unregister(vfs.tls, vfs.cvfs)
	delete(historyVFSObjects.roots, vfs.token)
	libc.Xfree(vfs.tls, vfs.cvfs)
	libc.Xfree(vfs.tls, vfs.cname)
	vfs.tls.Close()
	if code != sqlite3.SQLITE_OK {
		return fmt.Errorf("SQLite VFS unregister failed: %d", code)
	}
	return closeErr
}

func historyVFSRoot(pointer uintptr) *historyVFS {
	token := libc.AtomicLoadPUintptr(pointer + unsafe.Offsetof(sqlite3.Tsqlite3_vfs{}.FpAppData))
	historyVFSObjects.Lock()
	defer historyVFSObjects.Unlock()
	return historyVFSObjects.roots[token]
}

func historyVFSFileAt(pointer uintptr) *historyOpenFile {
	token := libc.AtomicLoadPUintptr(pointer + unsafe.Offsetof(historyVFSFile{}.token))
	historyVFSObjects.Lock()
	defer historyVFSObjects.Unlock()
	return historyVFSObjects.files[token]
}

func historyVFSOpen(_ *libc.TLS, pointer, name, output uintptr, flags int32, outFlags uintptr) int32 {
	vfs := historyVFSRoot(pointer)
	if vfs == nil || name == 0 || flags&(sqlite3.SQLITE_OPEN_MAIN_DB|sqlite3.SQLITE_OPEN_WAL) == 0 {
		return sqlite3.SQLITE_CANTOPEN
	}
	relative := libc.GoString(name)
	family := vfs.root.Family()
	if relative != family.Primary && relative != family.Primary+"-wal" {
		return sqlite3.SQLITE_CANTOPEN
	}
	file, err := vfs.root.Open(relative)
	if err != nil {
		return sqlite3.SQLITE_CANTOPEN
	}
	historyVFSObjects.Lock()
	historyVFSObjects.next++
	token := historyVFSObjects.next
	historyVFSObjects.files[token] = &historyOpenFile{vfs: vfs, file: file, name: relative}
	vfs.open++
	historyVFSObjects.Unlock()
	libc.AssignPtrUintptr(output+unsafe.Offsetof(historyVFSFile{}.methods), uintptr(unsafe.Pointer(&historyVFSIO)))
	libc.AssignPtrUintptr(output+unsafe.Offsetof(historyVFSFile{}.token), token)
	if outFlags != 0 {
		libc.AssignPtrInt32(outFlags, sqlite3.SQLITE_OPEN_READONLY)
	}
	return sqlite3.SQLITE_OK
}

func historyVFSCloseFile(_ *libc.TLS, pointer uintptr) int32 {
	file := historyVFSFileAt(pointer)
	if file == nil {
		return sqlite3.SQLITE_IOERR_CLOSE
	}
	if err := historyCloseSHM(file); err != nil {
		return sqlite3.SQLITE_IOERR_CLOSE
	}
	if file.locked {
		if err := file.vfs.locks.shared(file.file, false); err != nil {
			return sqlite3.SQLITE_IOERR_UNLOCK
		}
		file.locked = false
	}
	err := file.vfs.locks.close(file.file)
	historyVFSObjects.Lock()
	delete(historyVFSObjects.files, libc.AtomicLoadPUintptr(pointer+unsafe.Offsetof(historyVFSFile{}.token)))
	file.vfs.open--
	historyVFSObjects.Unlock()
	if err != nil {
		return sqlite3.SQLITE_IOERR_CLOSE
	}
	return sqlite3.SQLITE_OK
}

func historyVFSRead(_ *libc.TLS, pointer, buffer uintptr, length int32, offset int64) int32 {
	file := historyVFSFileAt(pointer)
	if file == nil || length < 0 || offset < 0 || file.vfs.root.Consume(int64(length)) != nil {
		return sqlite3.SQLITE_IOERR_READ
	}
	bytes := make([]byte, int(length))
	count, err := file.file.ReadAt(bytes, offset)
	if err == nil {
		copy(libc.GoBytes(buffer, len(bytes)), bytes)
		return sqlite3.SQLITE_OK
	}
	if errors.Is(err, io.EOF) {
		clear(bytes[count:])
		copy(libc.GoBytes(buffer, len(bytes)), bytes)
		return sqlite3.SQLITE_IOERR_SHORT_READ
	}
	return sqlite3.SQLITE_IOERR_READ
}

func historyVFSWrite(*libc.TLS, uintptr, uintptr, int32, int64) int32 { return sqlite3.SQLITE_READONLY }
func historyVFSTruncate(*libc.TLS, uintptr, int64) int32              { return sqlite3.SQLITE_READONLY }
func historyVFSSync(*libc.TLS, uintptr, int32) int32                  { return sqlite3.SQLITE_READONLY }
func historyVFSDelete(*libc.TLS, uintptr, uintptr, int32) int32       { return sqlite3.SQLITE_READONLY }
func historyVFSControl(*libc.TLS, uintptr, int32, uintptr) int32      { return sqlite3.SQLITE_NOTFOUND }
func historyVFSSector(*libc.TLS, uintptr) int32                       { return 4096 }
func historyVFSDevice(*libc.TLS, uintptr) int32                       { return 0 }

func historyVFSSize(_ *libc.TLS, pointer, output uintptr) int32 {
	file := historyVFSFileAt(pointer)
	info, err := file.file.Stat()
	if err != nil {
		return sqlite3.SQLITE_IOERR_FSTAT
	}
	libc.AssignPtrInt64(output, info.Size())
	return sqlite3.SQLITE_OK
}

func historyVFSLock(_ *libc.TLS, pointer uintptr, level int32) int32 {
	file := historyVFSFileAt(pointer)
	if file == nil || level > sqlite3.SQLITE_LOCK_SHARED {
		return sqlite3.SQLITE_READONLY
	}
	if !file.locked {
		if err := file.vfs.locks.shared(file.file, true); err != nil {
			return sqlite3.SQLITE_BUSY
		}
		file.locked = true
	}
	return sqlite3.SQLITE_OK
}

func historyVFSUnlock(_ *libc.TLS, pointer uintptr, level int32) int32 {
	if level == sqlite3.SQLITE_LOCK_NONE {
		file := historyVFSFileAt(pointer)
		if file == nil {
			return sqlite3.SQLITE_IOERR_UNLOCK
		}
		if file.locked {
			if err := file.vfs.locks.shared(file.file, false); err != nil {
				return sqlite3.SQLITE_IOERR_UNLOCK
			}
			file.locked = false
		}
	}
	return sqlite3.SQLITE_OK
}

func historyVFSReserved(_ *libc.TLS, pointer, output uintptr) int32 {
	file := historyVFSFileAt(pointer)
	if file == nil {
		return sqlite3.SQLITE_IOERR_CHECKRESERVEDLOCK
	}
	reserved, err := file.vfs.locks.reserved(file.file)
	if err != nil {
		return sqlite3.SQLITE_IOERR_CHECKRESERVEDLOCK
	}
	libc.AssignPtrInt32(output, libc.Bool32(reserved))
	return sqlite3.SQLITE_OK
}

func historyVFSPath(_ *libc.TLS, pointer, input uintptr, length int32, output uintptr) int32 {
	name := libc.GoString(input)
	vfs := historyVFSRoot(pointer)
	if !vfs.root.Family().Allows(name) || int(length) <= len(name) {
		return sqlite3.SQLITE_CANTOPEN
	}
	copy(libc.GoBytes(output, len(name)+1), append([]byte(name), 0))
	return sqlite3.SQLITE_OK
}

func historyVFSAccess(_ *libc.TLS, pointer, input uintptr, flags int32, output uintptr) int32 {
	libc.AssignPtrInt32(output, 0)
	vfs := historyVFSRoot(pointer)
	name := libc.GoString(input)
	if vfs == nil || flags == sqlite3.SQLITE_ACCESS_READWRITE || strings.HasSuffix(name, "-journal") {
		return sqlite3.SQLITE_OK
	}
	if _, err := vfs.probe(name); err == nil {
		libc.AssignPtrInt32(output, 1)
	}
	return sqlite3.SQLITE_OK
}

func (vfs *historyVFS) probe(name string) (*os.File, error) {
	if file := vfs.probes[name]; file != nil {
		return file, nil
	}
	file, err := vfs.root.Open(name)
	if err == nil {
		vfs.probes[name] = file
	}
	return file, err
}

func historyVFSShmMap(_ *libc.TLS, pointer uintptr, region, size, extend int32, output uintptr) int32 {
	libc.AssignPtrUintptr(output, 0)
	file := historyVFSFileAt(pointer)
	if file == nil || region < 0 || size != 32768 || region > 127 {
		return sqlite3.SQLITE_IOERR_SHMMAP
	}
	if file.shm == nil {
		shm, err := file.vfs.root.Open(file.vfs.root.Family().Primary + "-shm")
		if err != nil {
			return sqlite3.SQLITE_READONLY_CANTINIT
		}
		file.shm = shm
		if file.vfs.locks.shm(shm, true) != nil {
			historyCloseSHM(file)
			return sqlite3.SQLITE_BUSY
		}
		file.shmLocked = true
		info, err := shm.Stat()
		if err != nil || info.Size() < 32768 || info.Size() > 4<<20 || info.Size()%32768 != 0 ||
			file.vfs.root.Consume(info.Size()) != nil {
			historyCloseSHM(file)
			return sqlite3.SQLITE_IOERR_SHMMAP
		}
		index, err := unix.Mmap(-1, 0, int(info.Size()), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANON)
		if err == nil {
			_, err = io.ReadFull(shm, index)
		}
		if err != nil {
			if index != nil {
				unix.Munmap(index)
			}
			historyCloseSHM(file)
			return sqlite3.SQLITE_IOERR_SHMMAP
		}
		file.index = index
	}
	start := int(region) * int(size)
	if start+int(size) > len(file.index) {
		return sqlite3.SQLITE_READONLY_CANTINIT
	}
	libc.AssignPtrUintptr(output, uintptr(unsafe.Pointer(&file.index[start])))
	return sqlite3.SQLITE_OK
}

func historyVFSShmLock(_ *libc.TLS, pointer uintptr, offset, count, flags int32) int32 {
	file := historyVFSFileAt(pointer)
	if file == nil || file.shm == nil || offset < 0 || count < 1 || offset+count > 8 {
		return sqlite3.SQLITE_IOERR_SHMLOCK
	}
	if flags&sqlite3.SQLITE_SHM_LOCK != 0 && flags&sqlite3.SQLITE_SHM_EXCLUSIVE != 0 && offset < 3 {
		return sqlite3.SQLITE_READONLY_CANTLOCK
	}
	return sqlite3.SQLITE_OK
}

func historyVFSShmBarrier(*libc.TLS, uintptr) {}

func historyVFSShmUnmap(_ *libc.TLS, pointer uintptr, deleteFlag int32) int32 {
	file := historyVFSFileAt(pointer)
	if err := historyCloseSHM(file); err != nil {
		return sqlite3.SQLITE_IOERR_SHMLOCK
	}
	return sqlite3.SQLITE_OK
}

func historyCloseSHM(file *historyOpenFile) error {
	if file.index != nil {
		if err := unix.Munmap(file.index); err != nil {
			return err
		}
		file.index = nil
	}
	if file.shm != nil {
		if file.shmLocked {
			if err := file.vfs.locks.shm(file.shm, false); err != nil {
				return err
			}
			file.shmLocked = false
		}
		err := file.vfs.locks.close(file.shm)
		file.shm = nil
		return err
	}
	return nil
}
