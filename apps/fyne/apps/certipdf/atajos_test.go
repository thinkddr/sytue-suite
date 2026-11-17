// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestLaHojaDeAtajosEsLoEnganchado: los Ctrl+tecla de la hoja de «?» son, por IGUALDAD,
// los que `Atajos` deja en `p.atajos`. Enganchar uno sin ponerlo en la hoja (o quitarlo
// y dejar la fila) sale en rojo aquí, que es lo que la hoja existe para evitar.
func TestLaHojaDeAtajosEsLoEnganchado(t *testing.T) {
	p := NuevaPantalla(traductorES(t))
	w := test.NewWindow(container.NewStack())
	defer w.Close()
	fyne.DoAndWait(func() { p.Atajos(w.Canvas()) })

	var enLaHoja []fyne.KeyName
	for _, g := range gruposDelVisor {
		for _, f := range g.filas {
			enLaHoja = append(enLaHoja, f.ctrl...)
		}
	}
	enganchados := make([]fyne.KeyName, 0, len(p.atajos))
	for k := range p.atajos {
		enganchados = append(enganchados, k)
	}
	slices.Sort(enLaHoja)
	slices.Sort(enganchados)
	if !slices.Equal(enLaHoja, enganchados) {
		t.Errorf("la hoja enseña Ctrl+%v y el visor engancha Ctrl+%v", enLaHoja, enganchados)
	}
}

// TestLaHojaDeAtajosEsLaDeLaWeb compara las filas con `GRUPOS_VISOR` de `pdf/atajos.ts`,
// en el mismo orden: grupos y descripciones.
func TestLaHojaDeAtajosEsLaDeLaWeb(t *testing.T) {
	t.Parallel()
	fuente := webref.Read(t, "apps/frontend/packages/office/src/pdf/atajos.ts")
	inicio := strings.Index(fuente, "GRUPOS_VISOR")
	if inicio < 0 {
		t.Fatal("no está `GRUPOS_VISOR` en atajos.ts: ¿ha cambiado su forma?")
	}
	filas := regexp.MustCompile(`(?:titulo|descripcion): '([a-zA-Z.]+)'`).FindAllStringSubmatch(fuente[inicio:], -1)
	web := make([]string, 0, len(filas))
	for _, m := range filas {
		web = append(web, m[1])
	}
	var nativa []string
	for _, g := range gruposDelVisor {
		nativa = append(nativa, g.titulo)
		for _, f := range g.filas {
			nativa = append(nativa, f.descripcion)
		}
	}
	if !slices.Equal(web, nativa) {
		t.Errorf("la hoja de Fyne es\n  %v\ny la de la web\n  %v", nativa, web)
	}
}

// TestInterrogacionAbreLaHojaDeAtajos: «?» sin foco abre la hoja, traducida y con las
// teclas del sistema; con una capa ya delante, la tecla es de la capa y no se apila otra.
func TestInterrogacionAbreLaHojaDeAtajos(t *testing.T) {
	p := NuevaPantalla(traductorES(t))
	w := test.NewWindow(container.NewStack())
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	c := w.Canvas()
	fyne.DoAndWait(func() {
		p.Atajos(c)
		c.OnTypedRune()('a')
	})
	if c.Overlays().Top() != nil {
		t.Fatal("una letra cualquiera abre una capa")
	}
	fyne.DoAndWait(func() { c.OnTypedRune()('?') })
	capa := c.Overlays().Top()
	if capa == nil {
		t.Fatal("«?» no abre la hoja de atajos")
	}
	var textos []string
	todos(capa, func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.Tecla:
			textos = append(textos, x.Texto)
		case *canvas.Text:
			textos = append(textos, x.Text)
		}
	})
	todo := strings.Join(textos, "|")
	for _, quiero := range []string{
		p.t("pdf.shortcuts.title"), p.t("pdf.shortcuts.nextPage"), p.t("pdf.zoom.width"),
		p.t("pdf.menu.print"), p.t("slides.present.key.pageDown"), ui.AtajoDelSistema("Ctrl+F"),
	} {
		if !strings.Contains(todo, quiero) {
			t.Errorf("la hoja no lleva %q: %s", quiero, todo)
		}
	}
	if strings.Contains(todo, "pdf.shortcuts.") {
		t.Errorf("la hoja enseña claves sin traducir: %s", todo)
	}
	n := len(c.Overlays().List())
	fyne.DoAndWait(func() { c.OnTypedRune()('?') })
	if got := len(c.Overlays().List()); got != n {
		t.Errorf("«?» con la hoja abierta apila otra capa: %d → %d", n, got)
	}
}
