//go:build !linux && !darwin

package opencode

import (
	"errors"
	"os"
)

type historyVFS struct{ name string }

func newHistoryVFS(historyRoot) (*historyVFS, error) {
	return nil, errors.New("rooted WAL transactions are not supported on this platform")
}

func (*historyVFS) Close() error { return nil }

func (*historyVFS) probe(string) (*os.File, error) {
	return nil, errors.New("rooted WAL transactions are not supported on this platform")
}

func closeHistoryProbe(file *os.File) error { return file.Close() }
