// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package archivos

import "golang.org/x/sys/windows"

// espacioDisco es el total y lo libre (para quien usa la app) del disco de ruta.
func espacioDisco(ruta string) (total, libre int64, err error) {
	p, err := windows.UTF16PtrFromString(ruta)
	if err != nil {
		return 0, 0, err
	}
	var disponible, todo, todoLibre uint64
	if err := windows.GetDiskFreeSpaceEx(p, &disponible, &todo, &todoLibre); err != nil {
		return 0, 0, err
	}
	return int64(todo), int64(disponible), nil //nolint:gosec // un disco no pasa de 2^63 bytes
}
