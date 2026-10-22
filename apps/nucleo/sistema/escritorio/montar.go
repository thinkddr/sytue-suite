// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import (
	"context"
	"errors"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

// # Drive como disco, con lo que ya trae el sistema
//
// `apps/desktop` montaba Drive con FUSE, que pedía un controlador por sistema (macFUSE,
// WinFsp) y privilegios para instalarlo. Los tres escritorios ya saben montar WebDAV
// solos, con su propio diálogo de contraseña y su propia caché, así que aquí se le pide
// al explorador de ficheros que abra la dirección, y es él quien la monta:
//
//   - macOS: `mount volume` de AppleScript, que la deja en /Volumes y en la barra lateral.
//   - Windows: el Explorador con la ruta UNC de WebDAV (`\\host@SSL\ruta`), que lleva el
//     servicio WebClient de serie.
//   - Linux: el gestor de ficheros del escritorio sobre GVfs (`davs://`) o KIO
//     (`webdavs://`, Dolphin).
//
// ponytail: no hay caché sin conexión ni «disponible sin conexión» por carpeta, que es
// lo que daba FUSE con su propia caché. Se releva con un proveedor de ficheros del
// sistema (File Provider en macOS, Cloud Files en Windows), que pide un Mac y un Windows.

// errNoWebDAV es que la dirección no es la de un Drive que montar.
var errNoWebDAV = errors.New("escritorio: la dirección de WebDAV no es https")

// MontarWebDAV le pide al explorador de ficheros que monte `direccion` (la
// `webdav_url` de Sytue ID, https). Vuelve en cuanto lo ha lanzado: la contraseña la pide
// el sistema, en su ventana.
func MontarWebDAV(direccion string) error {
	prog, args, err := ordenMontar(runtime.GOOS, direccion, exec.LookPath)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(context.Background(), prog, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// ordenMontar es la orden que monta `direccion` en `goos`. `buscar` es `exec.LookPath`:
// en Linux se usa el gestor de ficheros que haya, y la prueba decide cuál.
func ordenMontar(goos, direccion string, buscar func(string) (string, error)) (string, []string, error) {
	u, err := url.Parse(direccion)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", nil, errNoWebDAV
	}
	switch goos {
	case "darwin":
		// Entre comillas de AppleScript: la dirección ya viene parseada, pero no se le
		// pasa al intérprete nada sin escapar.
		cita := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(u.String())
		return "osascript", []string{"-e", `mount volume "` + cita + `"`}, nil
	case "windows":
		unc := `\\` + u.Hostname() + "@SSL"
		if p := u.Port(); p != "" && p != "443" {
			unc += "@" + p
		}
		// La ruta UNC va DESCODIFICADA: el Explorador no entiende `%20`.
		return "explorer.exe", []string{unc + strings.ReplaceAll(strings.TrimSuffix(u.Path, "/"), "/", `\`)}, nil
	case "linux":
		for _, g := range [...]struct{ prog, esquema string }{
			{"nautilus", "davs"}, {"nemo", "davs"}, {"caja", "davs"}, {"thunar", "davs"}, {"dolphin", "webdavs"},
		} {
			if _, err := buscar(g.prog); err == nil {
				v := *u
				v.Scheme = g.esquema
				return g.prog, []string{v.String()}, nil
			}
		}
		v := *u
		v.Scheme = "davs"
		return "xdg-open", []string{v.String()}, nil
	}
	return "", nil, ErrNoDisponible
}
