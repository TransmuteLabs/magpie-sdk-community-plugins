//go:build linux || darwin

package opencode

import "os"

func closeHistoryProbe(file *os.File) error {
	return historyNativeLocks.close(file)
}
