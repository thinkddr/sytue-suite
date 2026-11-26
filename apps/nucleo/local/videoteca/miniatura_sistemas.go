// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package videoteca

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

// miniaturaDelSistema reparte por sistema (ver miniatura.go).
func miniaturaDelSistema(ctx context.Context, abs string) ([]byte, string, error) {
	switch runtime.GOOS {
	case "android":
		datos, err := miniaturaAndroid(ctx, abs)
		return datos, "jpg", err
	case "linux", "freebsd", "openbsd", "netbsd", "dragonfly":
		if datos, ok := enCacheFreedesktop(abs); ok {
			return datos, "png", nil
		}
		datos, err := conMiniaturizador(ctx, abs, carpetasMiniaturizadores())
		return datos, "png", err
	case "darwin":
		datos, err := quickLook(ctx, abs)
		return datos, "png", err
	case "windows":
		datos, err := miniaturaExplorador(ctx, abs)
		return datos, "png", err
	}
	return nil, "", ErrSinMiniatura
}

// quickLook pide la miniatura a Quick Look: `qlmanage -t -s 512 -o <carpeta> <fichero>`
// deja `<carpeta>/<nombre>.png`.
func quickLook(ctx context.Context, abs string) ([]byte, error) {
	ql, err := exec.LookPath("qlmanage")
	if err != nil {
		return nil, ErrSinMiniatura
	}
	dir, err := os.MkdirTemp("", "sytue-miniatura-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	c, fin := context.WithTimeout(ctx, 30*time.Second)
	defer fin()
	cmd := exec.CommandContext(c, ql, "-t", "-s", strconv.Itoa(TamanoMiniatura), "-o", dir, abs) //nolint:gosec // G204: Quick Look del sistema, ruta como argumento
	if err := cmd.Run(); err != nil {
		return nil, ErrSinMiniatura
	}
	datos, err := os.ReadFile(filepath.Join(dir, filepath.Base(abs)+".png"))
	if err != nil || len(datos) == 0 {
		return nil, ErrSinMiniatura
	}
	return datos, nil
}
