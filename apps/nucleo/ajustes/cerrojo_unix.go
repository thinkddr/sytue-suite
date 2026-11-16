// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !windows && !js

package ajustes

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func intentarCerrojo(f *os.File, compartido bool) (bool, error) {
	mode := unix.LOCK_EX
	if compartido {
		mode = unix.LOCK_SH
	}
	err := unix.Flock(int(f.Fd()), mode|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return false, nil
	}
	return err == nil, err
}
