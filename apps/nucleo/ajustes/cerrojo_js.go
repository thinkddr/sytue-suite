// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ajustes

import "os"

// El navegador no usa el almacén nativo ni tiene procesos que compartan su sesión.
func intentarCerrojo(*os.File, bool) (bool, error) { return false, os.ErrInvalid }
