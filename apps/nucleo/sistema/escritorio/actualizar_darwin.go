// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios && !tienda

package escritorio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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
	// …/Sytue Hub.app/Contents/MacOS/sytue-hub → …/Sytue Hub.app
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if !strings.HasSuffix(bundle, ".app") {
		return abrirDMG(ctx, ruta) // no está instalada como app: que la instale la persona
	}
	if err := instalarEn(ctx, ruta, bundle, relanzar); err != nil {
		// Sin permiso donde está instalada, lo de siempre: el DMG en el Finder.
		return errors.Join(err, abrirDMG(ctx, ruta))
	}
	return nil
}

// instalarEn monta el DMG `ruta` sin enseñarlo y pone su .app en lugar de `bundle`. La
// copia va primero junto al instalado y luego se cambian de sitio: así, si algo falla
// a medias, la app que había sigue entera.
func instalarEn(ctx context.Context, ruta, bundle string, abrir func(string)) error {
	montaje, err := os.MkdirTemp("", "sytue-dmg-")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(montaje) }()
	if err := ejecutar(ctx, "hdiutil", "attach", "-nobrowse", "-readonly", "-noautoopen", "-mountpoint", montaje, ruta); err != nil {
		return err
	}
	// El desmontaje, aunque `ctx` ya haya acabado: un DMG montado se queda hasta reiniciar.
	defer func() { _ = ejecutar(context.WithoutCancel(ctx), "hdiutil", "detach", "-force", montaje) }()

	nuevo := filepath.Join(montaje, filepath.Base(bundle))
	if _, err := os.Stat(nuevo); err != nil {
		apps, _ := filepath.Glob(filepath.Join(montaje, "*.app"))
		if len(apps) != 1 {
			return fmt.Errorf("escritorio: el DMG no trae %s", filepath.Base(bundle))
		}
		nuevo = apps[0]
	}
	copia, viejo := bundle+".nuevo", bundle+".viejo"
	_ = os.RemoveAll(copia)
	_ = os.RemoveAll(viejo)
	// `ditto` y no una copia a mano: conserva permisos, enlaces y la firma del bundle.
	if err := ejecutar(ctx, "ditto", nuevo, copia); err != nil {
		_ = os.RemoveAll(copia)
		return err
	}
	if err := os.Rename(bundle, viejo); err != nil {
		_ = os.RemoveAll(copia)
		return err
	}
	if err := os.Rename(copia, bundle); err != nil {
		return errors.Join(err, os.Rename(viejo, bundle))
	}
	_ = os.RemoveAll(viejo)
	if abrir != nil {
		abrir(bundle)
	}
	return nil
}

// hermanasDe son los .app de la carpeta del bundle de `exe` cuyo ejecutable es
// `sytue-<app>`, sin contarlo, con la versión de su Info.plist. Las que puso la Mac App
// Store no (InstaladaPorLaTienda): esas las actualiza la tienda.
func hermanasDe(exe string) []Hermana {
	propio := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if !strings.HasSuffix(propio, ".app") {
		return nil
	}
	bundles, _ := filepath.Glob(filepath.Join(filepath.Dir(propio), "*.app"))
	var out []Hermana
	for _, b := range bundles {
		if b == propio || InstaladaPorLaTienda(b) {
			continue
		}
		ejecutables, _ := filepath.Glob(filepath.Join(b, "Contents", "MacOS", "sytue-*"))
		for _, e := range ejecutables {
			if app := AppDeEjecutable(e); app != "" {
				out = append(out, Hermana{App: app, Destino: b, Version: versionDePlist(filepath.Join(b, "Contents", "Info.plist"))})
				break
			}
		}
	}
	return out
}

// versionPlist es la clave de la versión en el Info.plist XML que escribe `fyne package`.
var versionPlist = regexp.MustCompile(`<key>CFBundleShortVersionString</key>\s*<string>([^<]+)</string>`)

func versionDePlist(ruta string) string {
	b, err := os.ReadFile(ruta) //nolint:gosec // G304: el Info.plist de una app de la suite
	if err != nil {
		return ""
	}
	if m := versionPlist.FindSubmatch(b); m != nil {
		return strings.TrimSpace(string(m[1]))
	}
	return ""
}

func instalarHermana(ctx context.Context, ruta, destino string) error {
	return instalarEn(ctx, ruta, destino, nil)
}

func ejecutar(ctx context.Context, prog string, args ...string) error {
	salida, err := exec.CommandContext(ctx, prog, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("escritorio: %s: %w: %s", prog, err, salida)
	}
	return nil
}

// abrirDMG lo abre en el Finder, con su alias a Aplicaciones al lado.
func abrirDMG(ctx context.Context, ruta string) error {
	return exec.CommandContext(ctx, "open", ruta).Start()
}

// relanzar abre la app nueva cuando esta ya ha salido; ver el de Linux.
//
// ponytail: una espera fija; se releva esperando a que el enchufe deje de contestar.
func relanzar(bundle string) {
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", `sleep 2; exec open "$0"`, bundle) //nolint:contextcheck // tiene que sobrevivir a esta app
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	_ = cmd.Start()
}
