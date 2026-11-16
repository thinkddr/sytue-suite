// SPDX-License-Identifier: Apache-2.0
//go:build !android

package ajustes

import (
	"context"
	"os"
)

func clienteTrabajoPlataforma() (*clienteTrabajoBroker, bool) { return nil, false }
func descriptorTrabajoPlataforma(context.Context, string) (*os.File, error) {
	return nil, ErrTrabajoCompartidoNoDisponible
}

// EstadoTrabajoSesionCompartido preserves the native platform's sharing policy.
func EstadoTrabajoSesionCompartido() (bool, error) { return TrabajoSesionCompartidoDisponible(), nil }
