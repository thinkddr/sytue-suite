// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !(darwin && !ios && cgo) && !windows && !(linux && !android)

package portapapeles

// En los móviles, en la web y en macOS sin cgo, no se lee el HTML: se pega el texto, que es lo que
// da el portapapeles de Fyne.
func leer() ([]byte, error) { return nil, ErrNoDisponible }
