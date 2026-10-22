// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios

package ficheros

import "context"

// En iOS todavía no se abre la cámara: quien llama ofrece elegir fotos ya hechas, como
// en escritorio (movil_otros.go). Sin esta definición, iOS no compilaba.
func fotografiar(context.Context) (string, error) { return "", ErrNoDisponible }
