// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !unix

package correo

import "os"

// sinSeguirEnlaces: Windows no tiene O_NOFOLLOW; lo que se abre se compara con lo mirado
// sin seguir enlaces (os.SameFile), y un punto de reanálisis no es un fichero normal.
const sinSeguirEnlaces = 0

// unSoloNombre: en Windows no se mira (los enlaces duros piden permisos que una web no da).
func unSoloNombre(os.FileInfo) bool { return true }
