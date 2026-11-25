// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package montaje

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// MedidaCaptura es una de las medidas de la auditoría estética: el teléfono de 390×844
// con el dedo y el escritorio de 1280×800 con el ratón.
type MedidaCaptura struct {
	Nombre string
	Tam    fyne.Size
	Tactil bool
}

// MedidasAuditoria son las dos medidas con las que se retrató la suite el 2026-11-24.
var MedidasAuditoria = []MedidaCaptura{
	{"telefono", fyne.NewSize(390, 844), true},
	{"escritorio", fyne.NewSize(1280, 800), false},
}

// Capturar deja en `SYTUE_CAPTURAS` la ventana montada bajo la barra de la suite, como
// `cmd/<app>/main.go`, en las ocho combinaciones de la auditoría: teléfono y escritorio,
// Marco y Carril, claro y oscuro. `<nombre>-<medida>-<forma>-<tema>.png`. Sin la variable
// no hace nada: las imágenes son para mirarlas, no comprueban nada por sí solas.
//
// Con el cromo «papel» de las quince apps (ADR 0080), salvo `SYTUE_CAPTURAS_CROMO=color`,
// que pinta el de color de la suite.
//
// `preparar` corre tras cada cambio de medida, antes de pintar (abrir una hoja, llevar
// el desplazamiento a un sitio). Devuelve forma, tema, toque y medida como estaban.
func Capturar(t testing.TB, w fyne.Window, nombre string, producto iconos.Producto, preparar ...func(MedidaCaptura)) {
	t.Helper()
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // la ruta la pone quien lanza el test
		t.Fatal(err)
	}
	tr, err := i18n.New("es", i18n.Common())
	if err != nil {
		t.Fatal(err)
	}
	formaAntes := "marco"
	if !tema.EsMarco() {
		formaAntes = "carril"
	}
	contenido, tamano, relleno := w.Canvas().Content(), w.Canvas().Size(), w.Padded()
	apariencia := fyne.CurrentApp().Settings().Theme()
	barra, restaurar := ui.BarraSuiteDePrueba(tr, producto)
	if os.Getenv("SYTUE_CAPTURAS_CROMO") != "color" {
		tema.ElegirCromo(tema.CromoPapel)
	}
	defer func() {
		fallo := recover()
		restaurar()
		tema.ElegirCromo("")
		tema.ElegirForma(formaAntes)
		tema.Elegir("")
		if fallo != nil {
			panic(fallo) // SetTheme puede conservar un mutex tomado tras el pánico.
		}
		w.SetContent(contenido)
		w.SetPadded(relleno)
		w.Resize(tamano)
		fyne.CurrentApp().Settings().SetTheme(apariencia)
		fyne.DoAndWait(func() {})
	}()
	w.SetPadded(false)
	w.SetContent(barra.Montar(contenido))
	for _, m := range MedidasAuditoria {
		volver := ui.TactilDePrueba(m.Tactil)
		for _, forma := range []string{"marco", "carril"} {
			for _, variante := range []string{"claro", "oscuro"} {
				tema.ElegirForma(forma)
				tema.Elegir(variante)
				fyne.CurrentApp().Settings().SetTheme(apariencia)
				w.Resize(m.Tam)
				w.Canvas().Content().Refresh()
				fyne.DoAndWait(func() {})
				for _, p := range preparar {
					p(m)
				}
				fyne.DoAndWait(func() {})
				ruta := filepath.Join(dir, fmt.Sprintf("%s-%s-%s-%s.png",
					strings.ReplaceAll(nombre, "/", "-"), m.Nombre, forma, variante))
				f, err := os.Create(ruta) //nolint:gosec // la ruta la pone quien lanza el test
				if err != nil {
					t.Fatal(err)
				}
				if err := png.Encode(f, w.Canvas().Capture()); err != nil {
					t.Fatal(err)
				}
				_ = f.Close()
			}
		}
		volver()
	}
}
