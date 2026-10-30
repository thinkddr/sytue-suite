// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build (darwin || windows || linux) && !ios && !android

package ficheros

import (
	"context"
	"errors"
	"os/exec"
)

// Solo en escritorio: en el móvil y en el navegador no hay programas del sistema que
// lanzar, y ahí estas dos no las llama nadie.

// lanzar corre un programa del sistema y NO lo espera: el visor, el gestor de ficheros y
// el diálogo de impresión tienen que sobrevivir a esta llamada. Sin plazo a propósito, por
// lo mismo.
func lanzar(orden string, args ...string) error {
	return lanzarConContexto(context.Background(), orden, args...)
}

func lanzarConContexto(ctx context.Context, orden string, args ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return exec.CommandContext(context.WithoutCancel(ctx), orden, args...).Start() //nolint:gosec // G204: la ruta la ha validado local(); no hay intérprete de órdenes
}

// elegirCon corre el selector de ficheros del sistema y devuelve las rutas que escribe,
// una por línea. Cerrarlo sin elegir no es un error: devuelve nada.
//
// BLOQUEA hasta que la persona cierra el diálogo, así que se llama desde una rutina
// propia, nunca desde la de la interfaz.
func elegirCon(orden string, args ...string) ([]string, error) {
	salida, err := exec.CommandContext(context.Background(), orden, args...).Output() //nolint:gosec // G204: órdenes fijas del sistema, sin intérprete ni datos de nadie
	var salidaMala *exec.ExitError
	if errors.As(err, &salidaMala) && salidaMala.ExitCode() == 1 {
		return nil, nil // los tres selectores salen con 1 al cancelar
	}
	if err != nil {
		return nil, err
	}
	return lineas(salida), nil
}
