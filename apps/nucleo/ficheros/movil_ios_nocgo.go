// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios && !cgo

package ficheros

import (
	"os"
	"strings"
)

func imprimirMovil(string) error { return ErrNoDisponible }
func abrirMovil(string) error    { return ErrNoDisponible }
func revelarMovil(string) error  { return ErrNoDisponible }

func leerLocalSeguro(ruta string) ([]byte, error) {
	limpia := strings.TrimPrefix(ruta, "file://")
	return os.ReadFile(limpia)
}
