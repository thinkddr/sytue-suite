// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build unix

package archivos

import "golang.org/x/sys/unix"

// espacioDisco es el total y lo libre (para quien no es administrador) del disco de ruta.
func espacioDisco(ruta string) (total, libre int64, err error) {
	var st unix.Statfs_t
	if err := unix.Statfs(ruta, &st); err != nil {
		return 0, 0, err
	}
	b := int64(st.Bsize)                                   //nolint:gosec,unconvert // el tamaño de bloque cabe de sobra
	return int64(st.Blocks) * b, int64(st.Bavail) * b, nil //nolint:gosec // un disco no pasa de 2^63 bytes
}
