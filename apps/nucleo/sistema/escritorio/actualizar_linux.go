// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package escritorio

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func instalar(ctx context.Context, ruta string) error {
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		return err
	}
	return instalarEn(ctx, ruta, exe, relanzar)
}

// instalarEn saca de `ruta` (el .tar.xz del bundle) el ejecutable nuevo y lo pone en
// lugar de `exe`. Se extrae en una carpeta junto a `exe` para que el cambio sea un
// rename en el mismo disco, atómico. `abrir` se llama con el ejecutable ya nuevo.
func instalarEn(ctx context.Context, ruta, exe string, abrir func(string)) error {
	tmp, err := os.MkdirTemp(filepath.Dir(exe), ".sytue-actualizar-")
	if err != nil {
		// La carpeta no es de la persona: se le deja extraído con su instalador.
		return extraerYEnsenar(ctx, ruta)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	nuevo, err := extraerEjecutable(ctx, ruta, tmp, filepath.Base(exe))
	if err != nil {
		return err
	}
	if err := os.Chmod(nuevo, 0o755); err != nil { //nolint:gosec // es un ejecutable
		return err
	}
	if err := os.Rename(nuevo, exe); err != nil {
		return err
	}
	if abrir != nil {
		abrir(exe)
	}
	return nil
}

// hermanasDe son los `sytue-<app>` de la carpeta de `exe`, sin contarlo. Su versión no la
// dice nada en Linux (el ejecutable no la lleva aparte): quien llama supone la de esta.
func hermanasDe(exe string) []Hermana {
	candidatas, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), "sytue-*"))
	var out []Hermana
	for _, c := range candidatas {
		info, err := os.Stat(c)
		if c == exe || err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o100 == 0 {
			continue
		}
		if app := AppDeEjecutable(c); app != "" {
			out = append(out, Hermana{App: app, Destino: c})
		}
	}
	return out
}

func instalarHermana(ctx context.Context, ruta, destino string) error {
	return instalarEn(ctx, ruta, destino, nil)
}

// extraerEjecutable saca de `ruta` SOLO el ejecutable `nombre` y dice dónde ha quedado.
// El bundle lo lleva en `sytue-<bundle>/bin/` y el paquete suelto de Fyne de antes, en
// `sytue-<app>/usr/local/bin/`: se pide `*/bin/<nombre>` y vale para los dos. Sacar el
// bundle entero por cada app que se pone serían trece copias de cientos de megas junto
// al ejecutable.
func extraerEjecutable(ctx context.Context, ruta, dir, nombre string) (string, error) {
	salida, err := exec.CommandContext(ctx, "tar", "-xJf", ruta, "-C", dir, "--wildcards", "*/bin/"+nombre).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("escritorio: el paquete no trae %s: %w: %s", nombre, err, salida)
	}
	var nuevo string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() && d.Name() == nombre && filepath.Base(filepath.Dir(p)) == "bin" {
			nuevo = p
			return fs.SkipAll
		}
		return nil
	})
	if nuevo == "" {
		return "", fmt.Errorf("escritorio: el paquete no trae %s", nombre)
	}
	return nuevo, nil
}

// extraer desempaqueta el .tar.xz con el `tar` del sistema, que en cualquier escritorio
// de Linux trae xz: la biblioteca estándar no lo descomprime y no vale una dependencia
// para esto.
func extraer(ctx context.Context, ruta, dir string) error {
	salida, err := exec.CommandContext(ctx, "tar", "-xJf", ruta, "-C", dir).CombinedOutput()
	if err != nil {
		return fmt.Errorf("escritorio: tar: %w: %s", err, salida)
	}
	return nil
}

// extraerYEnsenar deja el paquete extraído en la carpeta temporal y la abre: dentro va
// el instalador del bundle, `sudo ./instalar.sh`.
func extraerYEnsenar(ctx context.Context, ruta string) error {
	dir, err := os.MkdirTemp("", "sytue-actualizar-")
	if err != nil {
		return err
	}
	if err := extraer(ctx, ruta, dir); err != nil {
		return errors.Join(err, os.RemoveAll(dir))
	}
	return exec.CommandContext(ctx, "xdg-open", dir).Start()
}

// relanzar abre la app nueva cuando esta ya ha salido: dos segundos es lo que tarda en
// soltar su enchufe de instancia única, y sin esperar la nueva vería a la vieja viva,
// le pasaría el testigo y saldría (`sesion.Traer`).
//
// ponytail: una espera fija; se releva esperando a que el enchufe deje de contestar.
func relanzar(exe string) {
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", `sleep 2; exec "$0"`, exe) //nolint:contextcheck // tiene que sobrevivir a esta app
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	_ = cmd.Start()
}
