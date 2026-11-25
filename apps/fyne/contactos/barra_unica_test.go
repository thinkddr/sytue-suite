// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestLaBarraDelTelefonoDiceElMismoNombre: en el teléfono la barra única decía la marca,
// «Contacts», sobre la lista —que se llama «Contactos» y no lo escribe porque lo dice la
// barra (`RepiteLaBarra`)—, y «Contactos» en la ficha. Ahora dice «Contactos» en las dos.
//
// VISTO EN ROJO sin `Respaldo`: la lista enseña «Contacts» en la barra.
func TestLaBarraDelTelefonoDiceElMismoNombre(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	muestra := pantallaDeMuestra(t)
	s := &servicioFalso{contactos: muestra.Contactos}
	a := appDePrueba(t, s)
	a.Cargar()
	s.correrCola()
	var w fyne.Window
	for _, x := range fyne.CurrentApp().Driver().AllWindows() {
		if x.Canvas() == a.lienzo() {
			w = x
		}
	}
	tr, err := i18n.New("es", i18n.Common())
	if err != nil {
		t.Fatal(err)
	}
	suite, parar := ui.BarraSuiteDePrueba(tr, iconos.Contacts)
	t.Cleanup(parar)
	w.SetContent(suite.Montar(w.Content()))
	w.Resize(fyne.NewSize(390, 844))
	a.repintar()

	nombre := a.Pantalla.t("list.title", nil)
	if nombre == iconos.Contacts.NombreCorto() {
		t.Fatalf("el catálogo de prueba no traduce el nombre (%q): el test no vigila nada", nombre)
	}
	for _, paso := range []string{"lista", "ficha"} {
		if paso == "ficha" {
			a.Pantalla.AlElegirContacto(a.Pantalla.Contactos[0])
			a.repintar()
		}
		barra := textosEnLaBarra(w)
		if !barra[nombre] || barra[iconos.Contacts.NombreCorto()] {
			t.Errorf("en la %s la barra dice %v; tocaba «%s»", paso, barra, nombre)
		}
	}
}

// textosEnLaBarra son los textos que se ven en la franja de arriba, la barra única.
func textosEnLaBarra(w fyne.Window) map[string]bool {
	out := map[string]bool{}
	d := fyne.CurrentApp().Driver()
	var ir func(o fyne.CanvasObject)
	ir = func(o fyne.CanvasObject) {
		if o == nil || !o.Visible() {
			return
		}
		if x, ok := o.(*canvas.Text); ok && x.Text != "" && d.AbsolutePositionForObject(x).Y < ui.AltoBarraSuite() {
			out[x.Text] = true
		}
		switch x := o.(type) {
		case *fyne.Container:
			for _, h := range x.Objects {
				ir(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				ir(h)
			}
		}
	}
	ir(w.Canvas().Content())
	return out
}
