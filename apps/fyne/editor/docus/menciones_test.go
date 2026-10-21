// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/color"
	"regexp"
	"strconv"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// La mención que se escribe es la que el servicio reconoce para avisar: `@correo`.
// Las mismas reglas que Menciones.test.ts de la web.
func TestLasMenciones(t *testing.T) {
	if q, ok := consultaMencion("Hola @an"); !ok || q != "an" {
		t.Errorf("«Hola @an» = %q %v", q, ok)
	}
	for _, x := range []string{"escribe a ana@x.es", "Hola @ana@x.es ya"} {
		if _, ok := consultaMencion(x); ok {
			t.Errorf("%q no es una mención a medias", x)
		}
	}
	if got := ponerMencion("Hola @an", "ana@x.es"); got != "Hola @ana@x.es " {
		t.Errorf("ponerMencion = %q", got)
	}
	g := []api.Miembro{{Email: "ana@x.es", Nombre: "Ana Pérez"}, {Email: "luis@x.es"}}
	if c := candidatosMencion(g, "pér"); len(c) != 1 || c[0].Email != "ana@x.es" {
		t.Errorf("por nombre: %+v", c)
	}
}

// Con la lista abierta, el teclado la recorre sin sacar el foco del área: flechas
// (dando la vuelta), Intro escribe la activa y Escape la cierra hasta la siguiente tecla.
func TestLasMencionesConElTeclado(t *testing.T) {
	campo := ui.NuevaAreaTexto("")
	m := nuevasMenciones(&App{}, campo)
	m.gente = []api.Miembro{{Email: "ana@x.es"}, {Email: "luis@x.es"}, {Email: "eva@x.es"}}
	campo.PonerTexto("Hola @")
	m.cambio(campo.Texto())
	tecla := func(k fyne.KeyName) { campo.TypedKey(&fyne.KeyEvent{Name: k}) }

	tecla(fyne.KeyUp) // de la primera a la última
	if m.activa != 2 {
		t.Fatalf("Arriba desde la primera = %d, quiero 2", m.activa)
	}
	tecla(fyne.KeyDown) // y de la última a la primera
	if m.activa != 0 {
		t.Fatalf("Abajo desde la última = %d, quiero 0", m.activa)
	}
	tecla(fyne.KeyEscape)
	if len(m.caja.Objects) != 0 {
		t.Fatal("Escape no cierra la lista")
	}
	campo.PonerTexto("Hola @l")
	m.cambio(campo.Texto())
	if len(m.lista) != 1 {
		t.Fatalf("al seguir escribiendo la lista vuelve: %+v", m.lista)
	}
	tecla(fyne.KeyReturn)
	if got := campo.Texto(); got != "Hola @luis@x.es " {
		t.Errorf("Intro escribe %q", got)
	}
	if moverActiva(0, 0, fyne.KeyDown) != 0 {
		t.Error("sin candidatos no hay activa que mover")
	}
}

// TestLaListaDeMencionesPintaComoLaWeb lee las clases de `useMenciones` (Menciones.tsx)
// y exige que Fyne pinte con los mismos tokens: panel, relleno, la opción activa, el
// nombre y el correo. Si la web cambia un color o un relleno, esto se pone en rojo.
func TestLaListaDeMencionesPintaComoLaWeb(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/packages/office/src/editor/panels/Menciones.tsx")
	hoja := webref.Read(t, "apps/frontend/packages/config/src/tokens/appearance.css")
	clase := func(re, que string) []string {
		m := regexp.MustCompile(re).FindStringSubmatch(fuente)
		if m == nil {
			t.Fatalf("Menciones.tsx ya no tiene %s (%s)", que, re)
		}
		return m[1:]
	}
	panel := clase(`class="sytue-menu-surface border-([\w-]+) flex flex-col rounded-md border p-(\d+) shadow-\[var\(--shadow-md\)\]"`, "el panel opaco del menú")
	superficie := regexp.MustCompile(`(?s)\.sytue-menu-surface,\s*\.sytue-popover-surface \{([^}]+)\}`).FindStringSubmatch(hoja)
	if superficie == nil {
		t.Fatal("appearance.css ya no define la superficie compartida de los menús")
	}
	token := func(propiedad string) string {
		m := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(propiedad) + `: var\(--sytue-([\w-]+)\);`).FindStringSubmatch(superficie[1])
		if m == nil {
			t.Fatalf("la superficie del menú ya no asigna %s a un token", propiedad)
		}
		return m[1]
	}
	opcion := clase(`rounded-sm px-(\d+) py-(\d+) text-left \$\{n === i \? 'bg-([\w-]+)' : ''\}`, "la opción")
	nombre := clase(`class="text-([\w-]+) w-full truncate text-sm font-medium"`, "el nombre")
	correo := clase(`class="text-([\w-]+) w-full truncate text-xs"`, "el correo")
	num := func(s string) float32 { n, _ := strconv.Atoi(s); return float32(n) }
	for _, c := range []struct{ web, fyne, que string }{
		{panel[0], bordePanelMenciones, "borde del panel"},
		{token("background"), fondoPanelMenciones, "fondo del panel"},
		{token("--color-" + opcion[2]), fondoOpcionActiva, "fondo de la opción activa dentro del menú"},
		{nombre[0], textoNombreMencion, "texto del nombre"},
		{correo[0], textoCorreoMencion, "texto del correo"},
	} {
		if c.web != c.fyne {
			t.Errorf("%s: la web pinta %q y Fyne %q", c.que, c.web, c.fyne)
		}
	}
	if num(panel[1]) != rellenoPanelMenciones || num(opcion[0]) != rellenoXOpcion || num(opcion[1]) != rellenoYOpcion {
		t.Errorf("rellenos: web p-%s px-%s py-%s, Fyne %v/%v/%v", panel[1], opcion[0], opcion[1],
			rellenoPanelMenciones, rellenoXOpcion, rellenoYOpcion)
	}

	// Y la que se pinta activa es la que dice el teclado, con esos colores.
	campo := ui.NuevaAreaTexto("")
	m := nuevasMenciones(&App{}, campo)
	m.gente = []api.Miembro{{Email: "ana@x.es", Nombre: "Ana"}, {Email: "luis@x.es"}}
	campo.PonerTexto("@")
	m.cambio(campo.Texto())
	campo.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	p := m.caja.Objects[0].(*panelMenciones)
	r := test.TempWidgetRenderer(t, p).(*panelMencionesRenderer)
	p.Resize(fyne.NewSize(240, r.MinSize().Height))
	t.Cleanup(func() { tema.Elegir("") })
	filas := p.filas.(*fyne.Container).Objects
	for _, apariencia := range []string{"claro", "oscuro", "traslucido"} {
		tema.Elegir(apariencia)
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		r.Refresh()
		if got, quiero := r.caja.Objetos()[1].(*canvas.Rectangle).FillColor, tema.Color("menu-surface", v); got != quiero || quiero.A != 0xff {
			t.Errorf("%s: el panel pinta %v y debe ser el menú opaco %v", apariencia, got, quiero)
		}
		for i, f := range filas {
			o := f.(*opcionMencion)
			fondo, texto, gris := o.colores(v)
			quiero := color.NRGBA{}
			if i == 1 {
				quiero = tema.Color("menu-hover", v)
			}
			if o.activa != (i == 1) || fondo != quiero || texto != tema.Color("text", v) || gris != tema.Color("text-muted", v) {
				t.Errorf("%s opción %d: activa=%v fondo=%v texto=%v correo=%v", apariencia, i, o.activa, fondo, texto, gris)
			}
		}
	}
}
