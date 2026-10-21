// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// # El actualizador propio
//
// Qué versión hay publicada y dónde está el instalador lo dice el Hub
// (`GET /api/v1/instaladores`); bajarlo y comprobar su SHA-256 lo hace la sesión
// (`apps/fyne/sesion/actualizar.go`). Aquí queda lo que depende del sistema: cómo se
// llama la plataforma en el catálogo, qué app es este ejecutable y cómo se pone el
// instalador ya comprobado.
//
// Lo que se publica en escritorio es un instalador por BUNDLE («Sytue One», «Sytue
// Drive»; `apps/fyne/scripts/empaquetar.sh`), con varias apps dentro. Aquí se pone la
// app que toca a partir del de su bundle:
//
//   - macOS: se monta el DMG sin enseñarlo, se copia SU .app junto al instalado y se
//     cambian de sitio (renombrar un bundle en marcha es seguro: el proceso sigue con
//     el viejo hasta salir), y la app se vuelve a abrir. Si no se puede escribir donde
//     está instalada —una cuenta sin permisos en /Applications—, se abre el DMG y la
//     persona lo arrastra, como la primera vez.
//   - Windows: se lanza el instalador de NSIS del bundle, que ya sabe reemplazar sus
//     apps; quien llama sale enseguida para que el ejecutable no esté en uso.
//   - Linux: se saca del .tar.xz SOLO su ejecutable (`sytue-<bundle>/bin/`), junto al
//     instalado, y se renombra encima (Linux deja reemplazar un ejecutable en marcha),
//     y la app se vuelve a abrir. Si la carpeta no es de la persona (`/usr/local/bin`,
//     instalado con `sudo ./instalar.sh`), se abre lo extraído en el gestor de ficheros
//     con su instalador.
//
// En los tres, quien llama cierra la app en cuanto [Instalar] vuelve sin error.

// Plataforma es el nombre de este sistema en el catálogo de instaladores: macos,
// windows o linux. Vacía en los que no se actualizan así: el móvil, que tiene tienda, y
// la variante de la Mac App Store ([DeTienda]), donde actualizarse por su cuenta está
// prohibido y además no se podría (el sandbox no deja escribir en /Applications).
func Plataforma() string {
	if DeTienda {
		return ""
	}
	return plataformaDe(runtime.GOOS)
}

func plataformaDe(goos string) string {
	switch goos {
	case "darwin":
		return "macos"
	case "windows", "linux":
		return goos
	}
	return ""
}

// AppDeEjecutable es la clave de la app en el catálogo (`hub`) sacada del nombre del
// ejecutable que pone `empaquetar.sh` (`sytue-hub`, `sytue-hub.exe`). Vacía si no es
// uno de esos: un `go run` no se actualiza.
func AppDeEjecutable(exe string) string {
	// Las dos barras, para que dé lo mismo en cualquier sistema qué ruta se le pase.
	app, ok := strings.CutPrefix(strings.TrimSuffix(path.Base(strings.ReplaceAll(exe, `\`, "/")), ".exe"), "sytue-")
	if !ok || app == "" {
		return ""
	}
	for _, c := range app {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return ""
		}
	}
	return app
}

// MasNueva dice si la versión `publicada` es posterior a la `actual`, las dos x.y.z
// (con «v» delante o sin ella). Una que no se entiende no es más nueva que nada: mejor
// no actualizar que instalar una versión que no se sabe cuál es.
func MasNueva(actual, publicada string) bool {
	a, okA := version(actual)
	p, okP := version(publicada)
	if !okA || !okP {
		return false
	}
	for i := range a {
		if p[i] != a[i] {
			return p[i] > a[i]
		}
	}
	return false
}

func version(v string) ([3]int, bool) {
	var out [3]int
	partes := strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".")
	if len(partes) != 3 {
		return out, false
	}
	for i, p := range partes {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// InstaladaPorLaTienda dice si el bundle `app` (un .app de macOS) lo puso la Mac App
// Store: lleva su recibo en `Contents/_MASReceipt/receipt`. El actualizador del .dmg no lo
// toca nunca. Las apps de la tienda se llaman igual y van a la misma carpeta que las del
// .dmg, así que el Hub del .dmg las tomaría por hermanas y las cambiaría por su copia con
// Developer ID: la tienda dejaría de ponerlas al día y la sesión, que en la variante de
// tienda va en otro llavero, se perdería. Ver «Mac App Store» en docs/operations.md.
func InstaladaPorLaTienda(app string) bool {
	_, err := os.Stat(filepath.Join(app, "Contents", "_MASReceipt", "receipt"))
	return err == nil
}

// Hermana es otra app de la suite instalada junto a esta: el Hub la pone al día a la vez.
type Hermana struct {
	// App es la clave del catálogo (`mail`) y Destino dónde está instalada (el .app en
	// macOS, el ejecutable en Linux).
	App, Destino string
	// Version es la instalada, si el sistema la dice (el Info.plist en macOS); vacía si
	// no se sabe.
	Version string
}

// Hermanas son las apps de la suite instaladas junto a esta, sin contarla:
//
//   - macOS: los .app de su misma carpeta cuyo ejecutable es `sytue-<app>`.
//   - Linux: los `sytue-<app>` de su misma carpeta (`~/.local/bin`, `/usr/local/bin`).
//   - Windows: las de los OTROS bundles, por su entrada de desinstalación de NSIS en el
//     registro, con la versión que dejó su instalador; se ponen con el instalador en
//     silencio (`/S`, actualizar_nsis.go). Las del bundle propio las pone el instalador
//     de esta. ponytail: sin verlo en un Windows.
func Hermanas() []Hermana {
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		return nil
	}
	return hermanasDe(exe)
}

// InstalarHermana pone el instalador `ruta`, ya comprobado, sobre la hermana `destino`
// sin abrirla ni cerrar esta. Si está abierta, sigue con la versión vieja hasta que se
// vuelva a abrir.
func InstalarHermana(ctx context.Context, ruta, destino string) error {
	return instalarHermana(ctx, ruta, destino)
}

// Instalar pone el instalador `ruta`, ya comprobado, sobre esta app. Si vuelve sin
// error, quien llama cierra la app: el instalador (o la copia nueva, que se abre sola)
// termina el trabajo. `ctx` acota lo que se hace aquí (montar, extraer, copiar); lo que
// se lanza para después —el instalador, la app nueva— sigue aunque acabe.
func Instalar(ctx context.Context, ruta string) error { return instalar(ctx, ruta) }
