//go:build !linux && !android

// SPDX-License-Identifier: Apache-2.0
package ajustes

import "os"

const plataformaDirectorioDurable = false

func sincronizarDirectorio(*os.File) error { return ErrPersistenciaNoSoportada }
