// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !darwin && !ios

package ajustes

import "os"

// base es la carpeta de configuración del usuario, la misma para todas las apps.
func base() (string, error) { return os.UserConfigDir() }
