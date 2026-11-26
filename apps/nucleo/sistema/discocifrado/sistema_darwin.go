// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios

package discocifrado

import (
	"context"
	"os/exec"

	"golang.org/x/sys/unix"
)

func deSistema(ctx context.Context, ruta string) Resultado {
	return deMac(ctx, ruta, entornoMac{volumen: volumen, orden: ordenDelSistema})
}

// volumen es el punto de montaje y el dispositivo de ruta, por statfs(2).
func volumen(ruta string) (punto, dispositivo string, err error) {
	var st unix.Statfs_t
	if err := unix.Statfs(ruta, &st); err != nil {
		return "", "", err
	}
	return unix.ByteSliceToString(st.Mntonname[:]), unix.ByteSliceToString(st.Mntfromname[:]), nil
}

func ocultarVentana(*exec.Cmd) {}
