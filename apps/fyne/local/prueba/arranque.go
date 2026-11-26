// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package prueba monta el `main` de verdad de una app de Sytue Office en el driver de
// pruebas de Fyne, recién instalada, para los guardianes de la edición
// (apps/fyne/PLAN-EDICION-LOCAL.md §8.1). Solo lo importan los `_test.go` de
// `cmd-local/*`: ningún binario de la edición lo enlaza
// (TestLasAppsLocalesNoEnlazanCifradoNiMatrix, en `local`).
package prueba

import (
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// Espera es lo que se deja correr a la app tras montarse, para que lo que arranca detrás
// (sincronizar, leer el almacén, los avisos) tenga ocasión de intentar salir a la red.
var Espera = 1500 * time.Millisecond

// Arrancar es TestRecienInstaladaNoAbreNingunaConexion para una app (R-RED, R-CONVIVE):
// con una carpeta personal vacía —recién instalada, sin cuentas, sin IA y sin nube— corre
// principal (el `main` de la app) con la app de Fyne del driver de pruebas (que pide a
// [local.NuevaApp]), la deja arrancar y falla si:
//
//   - algo intenta abrir una conexión por el portero o por el transporte que lo deniega
//     todo (`red.VigilarDePrueba` ve cada intento, se permitiera o no);
//   - el portero no quedó a cargo de la red del proceso (`http.DefaultTransport` y
//     `api.Transporte` son [red.Denegar]);
//   - la ventana no pinta su pantalla inicial (sin contenido, o lisa);
//   - escribe en las carpetas de la edición conectada (`<config>/sytue`, `<caché>/sytue`)
//     o no deja sus ajustes en `<config>/sytue-office` y sus datos en `~/Sytue Office`.
//
// Con SYTUE_ARRANQUE_CAPTURAS=<carpeta>, deja allí la captura de la pantalla inicial.
func Arrancar(t *testing.T, app string, principal func()) {
	t.Helper()
	casa, err := os.MkdirTemp("", "sytue-office-"+app+"-")
	if err != nil {
		t.Fatal(err)
	}
	// Lo que siga corriendo detrás puede escribir aún al borrar: no es un fallo de la
	// prueba que la carpeta no se vaya entera.
	t.Cleanup(func() { _ = os.RemoveAll(casa) })
	config, cache := filepath.Join(casa, ".config"), filepath.Join(casa, ".cache")
	for clave, valor := range map[string]string{
		"HOME": casa, "USERPROFILE": casa, "XDG_CONFIG_HOME": config, "XDG_CACHE_HOME": cache,
		"XDG_DATA_HOME": filepath.Join(casa, ".local", "share"), "XDG_RUNTIME_DIR": filepath.Join(casa, "run"),
		"APPDATA": config, "LOCALAPPDATA": cache, "TMPDIR": filepath.Join(casa, "tmp"),
		"SYTUE_IDIOMA": "es", "SYTUE_OFFICE_DATOS": "",
	} {
		t.Setenv(clave, valor)
	}
	for _, d := range []string{filepath.Join(casa, "run"), filepath.Join(casa, "tmp")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	var (
		mu       sync.Mutex
		intentos []red.Intento
	)
	t.Cleanup(red.VigilarDePrueba(func(i red.Intento) {
		mu.Lock()
		intentos = append(intentos, i)
		mu.Unlock()
	}))
	var a fyne.App
	t.Cleanup(local.AppDePrueba(func(string) fyne.App {
		a = test.NewApp()
		return a
	}))
	transporte, transporteAPI := http.DefaultTransport, api.Transporte
	t.Cleanup(func() { http.DefaultTransport, api.Transporte = transporte, transporteAPI })

	principal()
	if a == nil {
		t.Fatalf("%s: el main no pide la app a local.NuevaApp", app)
	}
	// Lo que la cáscara hace cuando el bucle de la interfaz arranca (alArrancar).
	if ciclo, ok := a.Lifecycle().(interface{ OnStarted() func() }); ok {
		if f := ciclo.OnStarted(); f != nil {
			f()
		}
	}
	time.Sleep(Espera)

	// Las ventanas de la app: las que tienen contenido (el driver de pruebas trae una
	// vacía suya para pintar).
	// (AllWindows da la lista del driver: se copia antes de filtrarla.)
	ventanas := slices.DeleteFunc(slices.Clone(a.Driver().AllWindows()), func(v fyne.Window) bool { return v.Content() == nil })
	if len(ventanas) == 0 {
		t.Fatalf("%s: no abre ninguna ventana con contenido", app)
	}
	w := ventanas[0]
	t.Cleanup(func() {
		for _, v := range ventanas {
			v.Close()
		}
		// Lo que se cierra detrás (almacenes, índices) antes de borrar la carpeta.
		time.Sleep(200 * time.Millisecond)
	})
	foto := w.Canvas().Capture()
	if dir := os.Getenv("SYTUE_ARRANQUE_CAPTURAS"); dir != "" {
		guardar(t, filepath.Join(dir, app+".png"), foto)
	}
	if n := colores(foto); n < 4 {
		t.Errorf("%s: la pantalla inicial no pinta nada (%d colores en %v)", app, n, foto.Bounds().Size())
	}

	mu.Lock()
	vistos := slices.Clone(intentos)
	mu.Unlock()
	for _, i := range vistos {
		t.Errorf("%s: recién instalada intenta una conexión: %+v", app, i)
	}
	comprobarPortero(t, app)
	comprobarCarpetas(t, app, casa, config, cache)
}

// comprobarPortero: la red del proceso, a cargo del portero (lo que no pase por él falla
// cerrado).
func comprobarPortero(t *testing.T, app string) {
	t.Helper()
	if _, ok := http.DefaultTransport.(red.Denegar); !ok {
		t.Errorf("%s: http.DefaultTransport es %T, no red.Denegar: el portero no está a cargo de la red", app, http.DefaultTransport)
	}
	if _, ok := api.Transporte.(red.Denegar); !ok {
		t.Errorf("%s: api.Transporte es %T, no red.Denegar", app, api.Transporte)
	}
}

// comprobarCarpetas es R-CONVIVE: nada en las carpetas de la edición conectada, lo de la
// edición donde va y, sin configurar nada, ningún secreto guardado.
func comprobarCarpetas(t *testing.T, app, casa, config, cache string) {
	t.Helper()
	for _, d := range []string{filepath.Join(config, "sytue"), filepath.Join(cache, "sytue")} {
		if _, err := os.Stat(d); err == nil {
			t.Errorf("%s: escribe en %s, que es de la edición conectada", app, d)
		}
	}
	for _, d := range []string{filepath.Join(config, espacio.Carpeta), filepath.Join(casa, espacio.CarpetaDatos)} {
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			t.Errorf("%s: no está %s: %v", app, d, err)
		}
	}
	// Ningún secreto sin configurar nada: la carpeta de reserva del llavero, vacía o sin hacer.
	if es, _ := os.ReadDir(filepath.Join(config, espacio.Carpeta, "secretos")); len(es) > 0 {
		t.Errorf("%s: recién instalada guarda %d secretos", app, len(es))
	}
}

// colores cuenta los colores distintos de img (hasta 64): una ventana que no pinta nada es
// de uno o dos.
func colores(img image.Image) int {
	vistos := map[color.RGBA]bool{}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 3 {
		for x := b.Min.X; x < b.Max.X; x += 3 {
			c, _ := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			vistos[c] = true
			if len(vistos) >= 64 {
				return len(vistos)
			}
		}
	}
	return len(vistos)
}

func guardar(t *testing.T, ruta string, img image.Image) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil { //nolint:gosec // G301, G703: la carpeta de capturas que pide quien corre la prueba
		t.Log(err)
		return
	}
	f, err := os.Create(ruta) //nolint:gosec // G304: la carpeta de capturas que pide quien corre la prueba
	if err != nil {
		t.Log(err)
		return
	}
	if err := png.Encode(f, img); err != nil {
		t.Log(err)
	}
	if err := f.Close(); err != nil {
		t.Log(err)
	}
}
