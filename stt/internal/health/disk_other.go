//go:build !linux && !windows

package health

import "errors"

func DiskFreeBytes(string) (uint64, error) { return 0, errors.New("disk space check unsupported") }
