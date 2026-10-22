// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios && !tienda

package ajustes

import "os"

// Un binario de desarrollo sin firma ni perfil no puede abrir el App Group real.
func baseSinGrupoApple() (string, error) { return os.UserConfigDir() }
