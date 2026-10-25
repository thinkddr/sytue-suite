// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package documentos

import (
	"context"
	"os/exec"
)

func abrir(ctx context.Context) error {
	cred, err := cargarCredencial()
	if err != nil {
		return err
	}
	if cred.Clave == "" {
		return ErrNoDisponible
	}
	u, err := ubicacion(cred)
	if err != nil {
		return err
	}
	return exec.CommandContext(ctx, "gio", "open", u).Run()
}
