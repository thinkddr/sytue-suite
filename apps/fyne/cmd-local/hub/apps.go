// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/apps/hub"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
)

// App es una app de la edición que el Hub sabe lanzar: su carpeta en `cmd-local` (que
// es también el final de su ejecutable, `sytue-office-<app>`), su clave de producto
// (la del esquema, `sytueoffice-<producto>`) y su marca.
type App struct {
	Carpeta  string
	Producto string
	Marca    iconos.Producto
}

// catalogo son las apps de la edición en el orden del lanzador de la suite
// (`hub.Productos`). Una carpeta nueva en `cmd-local` sin fila aquí no sale en el Hub, y
// `TestElCatalogoEsElDeCmdLocal` lo pone en rojo.
var catalogo = []App{
	{"mail", "mail", iconos.Mail},
	{"drive", "drive", iconos.Drive},
	{"calendar", "calendar", iconos.Calendar},
	{"contacts", "contacts", iconos.Contacts},
	{"docus", "docs", iconos.Docs},
	{"calcus", "sheets", iconos.Sheets},
	{"diapos", "slides", iconos.Slides},
	{"certipdf", "pdf", iconos.PDF},
	{"notes", "notes", iconos.Notes},
	{"photos", "photos", iconos.Photos},
	{"rooms", "rooms", iconos.Rooms},
	{"cast", "cast", iconos.Cast},
	{"erp", "erp", iconos.ERP},
	{"bases", "bases", iconos.Bases},
}

// Producto es la tesela del lanzador de la suite para esta app.
func (a App) Tesela() hub.Producto { return hub.Producto{Marca: a.Marca, Nombre: a.Marca.Nombre()} }

// Instalada es una app del catálogo que está en este equipo: Destino es su ejecutable
// (Linux y Windows) o su `.app` (macOS).
type Instalada struct {
	App
	Destino string
}

// Instaladas son las apps de la edición instaladas JUNTO al Hub, en el orden del
// catálogo: el bundle de escritorio las pone todas en la misma carpeta
// (`empaquetar.sh`, `bundle = "office"`).
//
//   - Linux: `sytue-office-<app>` ejecutables en la carpeta del Hub.
//   - Windows: `sytue-office-<app>.exe` en la carpeta del Hub.
//   - macOS: los `.app` de la carpeta del `.app` del Hub cuyo ejecutable es
//     `Contents/MacOS/sytue-office-<app>`.
//
// En el móvil no hay Hub: cada app va por separado.
func Instaladas(exe, goos string) []Instalada {
	if exe == "" {
		return nil
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	dir := filepath.Dir(exe)
	var bundles []string
	if goos == "darwin" {
		propio := filepath.Dir(filepath.Dir(dir)) // …/Sytue Office.app/Contents/MacOS/x
		if !strings.HasSuffix(propio, ".app") {
			return nil
		}
		bundles, _ = filepath.Glob(filepath.Join(filepath.Dir(propio), "*.app"))
	}
	var out []Instalada
	for _, a := range catalogo {
		nombre := espacio.PrefijoEjecutable + a.Carpeta
		switch goos {
		case "darwin":
			for _, b := range bundles {
				if esEjecutable(filepath.Join(b, "Contents", "MacOS", nombre), goos) {
					out = append(out, Instalada{a, b})
					break
				}
			}
		case "windows":
			if c := filepath.Join(dir, nombre+".exe"); esEjecutable(c, goos) {
				out = append(out, Instalada{a, c})
			}
		default:
			if c := filepath.Join(dir, nombre); c != exe && esEjecutable(c, goos) {
				out = append(out, Instalada{a, c})
			}
		}
	}
	return out
}

func esEjecutable(ruta, goos string) bool {
	info, err := os.Stat(ruta)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return goos == "windows" || info.Mode().Perm()&0o111 != 0
}

// ErrNoLanza: este sistema no lanza otras apps desde aquí (el móvil).
var ErrNoLanza = errors.New("hub: en este sistema las apps se abren por separado")

// Orden es lo que se ejecuta para abrir `i` con `enlace` (vacío: su portada). No pasa
// por ningún intérprete: el enlace va como UN argumento, y lo compone el Hub
// (`indice.Elemento.Enlace`, siempre `sytueoffice-<producto>:…`).
func Orden(i Instalada, enlace, goos string) ([]string, error) {
	if enlace != "" && !strings.HasPrefix(enlace, espacio.PrefijoEsquema) {
		return nil, errors.New("hub: enlace ajeno a Sytue Office")
	}
	switch goos {
	case "darwin":
		// `open -a`: si ya está abierta, la trae delante en vez de abrir otra.
		orden := []string{"open", "-a", i.Destino}
		if enlace != "" {
			orden = append(orden, "--args", enlace)
		}
		return orden, nil
	case "linux", "windows", "freebsd", "openbsd", "netbsd":
		orden := []string{i.Destino}
		if enlace != "" {
			orden = append(orden, enlace)
		}
		return orden, nil
	}
	return nil, ErrNoLanza
}

// Lanzar abre la app sin esperar a que se cierre.
func Lanzar(i Instalada, enlace, goos string) error {
	orden, err := Orden(i, enlace, goos)
	if err != nil {
		return err
	}
	if goos == "darwin" {
		ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelar()
		return exec.CommandContext(ctx, orden[0], orden[1:]...).Run() //nolint:gosec // G204: el .app hermano y un enlace de la edición
	}
	// La app abierta vive por su cuenta: un contexto que no se cancela nunca.
	cmd := exec.CommandContext(context.Background(), orden[0], orden[1:]...) //nolint:gosec // G204: el ejecutable hermano y un enlace de la edición
	cmd.Dir = filepath.Dir(orden[0])
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
