// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import "golang.org/x/sys/unix"

// clonarFichero deja en destino (que no debe existir) un clon del original con
// `clonefile` de APFS: comparte sus bloques hasta que uno de los dos cambie, así que no
// ocupa más y escribir en uno no toca el otro. Fuera de APFS, error, y quien llama copia.
func clonarFichero(origen, destino string) error {
	return unix.Clonefile(origen, destino, unix.CLONE_NOFOLLOW)
}
