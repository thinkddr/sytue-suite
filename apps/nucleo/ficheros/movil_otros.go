// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android && !ios

package ficheros

import (
	"context"
	"os"
	"strings"
)

func fotografiar(context.Context) (string, error) { return "", ErrNoDisponible }

// imprimirMovil solo lo usa sistema_otros.go, que en el navegador no imprime.
func imprimirMovil(string) error { return ErrNoDisponible } //nolint:unused // solo en los destinos de sistema_otros.go
func abrirMovil(string) error    { return ErrNoDisponible } //nolint:unused // solo en los destinos de sistema_otros.go
func revelarMovil(string) error  { return ErrNoDisponible } //nolint:unused // solo en los destinos de sistema_otros.go

func leerLocalSeguro(ruta string) ([]byte, error) {
	limpia := strings.TrimPrefix(ruta, "file://")
	return os.ReadFile(limpia)
}
