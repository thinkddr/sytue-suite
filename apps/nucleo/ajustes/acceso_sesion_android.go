// SPDX-License-Identifier: Apache-2.0
//go:build android

package ajustes

import (
	"context"
	"errors"
	"os"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

var trabajoAndroid = clienteTrabajoBroker{
	disponible: EstadoTrabajoSesionCompartido,
	pedir: func(ctx context.Context, accion, datos string) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return sistema.TrabajoSesionAndroid(accion, datos)
	},
}

// EstadoTrabajoSesionCompartido never treats a failed new broker as an old one.
func EstadoTrabajoSesionCompartido() (bool, error) {
	habilitada, err := sistema.SesionAndroidHabilitada()
	if err != nil {
		return false, errors.Join(ErrProtocoloTrabajoSesion, err)
	}
	if !habilitada {
		return false, nil
	}
	version, err := sistema.CapacidadTrabajoSesionAndroid()
	if err != nil {
		return false, errors.Join(ErrProtocoloTrabajoSesion, err)
	}
	switch version {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, ErrProtocoloTrabajoSesion
	}
}

func clienteTrabajoPlataforma() (*clienteTrabajoBroker, bool) { return &trabajoAndroid, true }
func descriptorTrabajoPlataforma(ctx context.Context, datos string) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fd, err := sistema.CerrojoTrabajoSesionAndroid(datos)
	if err != nil {
		return nil, err
	}
	if fd < 0 {
		return nil, ErrProtocoloTrabajoSesion
	}
	file := os.NewFile(uintptr(fd), "sytue-session-private-work")
	if err := ctx.Err(); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}
