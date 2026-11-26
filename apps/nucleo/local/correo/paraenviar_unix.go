// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build unix

package correo

import (
	"os"
	"syscall"
)

// sinSeguirEnlaces hace que abrir un enlace simbólico falle en vez de seguirlo.
const sinSeguirEnlaces = syscall.O_NOFOLLOW

// unSoloNombre dice que el fichero no tiene otro nombre (un enlace duro) fuera de la
// carpeta: lo que se adjunta es la copia que dejó quien adjunta.
func unSoloNombre(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return !ok || st.Nlink == 1
}
