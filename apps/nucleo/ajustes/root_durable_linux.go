//go:build linux || android

// SPDX-License-Identifier: Apache-2.0
package ajustes

import "os"

const plataformaDirectorioDurable = true

func sincronizarDirectorio(dir *os.File) error { return dir.Sync() }
