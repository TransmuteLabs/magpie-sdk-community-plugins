//go:build !linux && !darwin

package controller

import (
	"context"
	"errors"
	"os"
)

func acquireLock(context.Context, *os.File) (func(), error) {
	return nil, errors.New("ZCode durable controller locking is not implemented on this platform")
}
