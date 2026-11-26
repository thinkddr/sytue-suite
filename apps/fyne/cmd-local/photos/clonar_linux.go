// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// clonarFichero deja en destino (que no debe existir) un CLON del original: comparte sus
// bloques hasta que uno de los dos cambie (FICLONE: Btrfs, XFS, bcachefs…), así que no
// ocupa más y escribir en uno no toca el otro. Donde el sistema de ficheros no clona,
// error, y quien llama copia.
func clonarFichero(origen, destino string) error {
	o, err := os.Open(origen)
	if err != nil {
		return err
	}
	defer o.Close()
	d, err := os.OpenFile(destino, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	err = unix.IoctlFileClone(int(d.Fd()), int(o.Fd()))
	if cerr := d.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(destino)
	}
	return err
}
