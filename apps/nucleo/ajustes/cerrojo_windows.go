// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ajustes

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func intentarCerrojo(f *os.File, compartido bool) (bool, error) {
	var overlapped windows.Overlapped
	mode := uint32(windows.LOCKFILE_FAIL_IMMEDIATELY)
	if !compartido {
		mode |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	err := windows.LockFileEx(windows.Handle(f.Fd()), mode, 0, 1, 0, &overlapped)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return err == nil, err
}
