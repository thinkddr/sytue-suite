// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package discocifrado

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func deSistema(ctx context.Context, ruta string) Resultado {
	return deLinux(ctx, ruta, entornoLinux{
		leer: os.ReadFile,
		listar: func(dir string) ([]string, error) {
			es, err := os.ReadDir(dir)
			nombres := make([]string, 0, len(es))
			for _, e := range es {
				nombres = append(nombres, e.Name())
			}
			return nombres, err
		},
		resolver: filepath.EvalSymlinks,
		fscrypt:  fscrypt,
		orden:    ordenDelSistema,
	})
}

// fscrypt mira el atributo «cifrado» de statx(2), que el núcleo pone en lo cifrado con
// fscrypt (ext4, f2fs, ubifs). sabido es falso si el sistema de ficheros no lo informa.
func fscrypt(ruta string) (cifrada, sabido bool) {
	var st unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, ruta, unix.AT_STATX_SYNC_AS_STAT, unix.STATX_BASIC_STATS, &st); err != nil {
		return false, false
	}
	if st.Attributes_mask&unix.STATX_ATTR_ENCRYPTED == 0 {
		return false, false
	}
	return st.Attributes&unix.STATX_ATTR_ENCRYPTED != 0, true
}

func ocultarVentana(*exec.Cmd) {}
