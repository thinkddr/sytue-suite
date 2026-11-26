// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/fototeca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/libreta"
)

// contactosDeAntes deja en dir la libreta del almacén de registros (la de las versiones
// de antes de Contacts) con un contacto.
func contactosDeAntes(t *testing.T, dir string) {
	t.Helper()
	rec, err := almacen.Abrir(context.Background(), dir, "contacts", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Cerrar()
	repo, err := contacts.New(rec.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := almacen.Preparar(t.Context(), repo); err != nil {
		t.Fatal(err)
	}
	b, err := repo.CreateAddressBook(t.Context(), "Personal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(t.Context(), b.ID, api.ContactInput{FormattedName: "Ana García"}); err != nil {
		t.Fatal(err)
	}
}

// pulsarBoton pulsa, en la ventana o en lo que tiene encima, el botón de ui o de Fyne
// con ese rótulo.
func pulsarBoton(t *testing.T, w fyne.Window, rotulo string) {
	t.Helper()
	fin := time.Now().Add(10 * time.Second)
	for time.Now().Before(fin) {
		var tocar fyne.Tappable
		fyne.DoAndWait(func() {
			raices := append([]fyne.CanvasObject{w.Content()}, w.Canvas().Overlays().List()...)
			for _, r := range raices {
				for _, o := range test.LaidOutObjects(r) {
					switch b := o.(type) {
					case *ui.Boton:
						if b.Texto == rotulo && b.Visible() {
							tocar = b
						}
					case *widget.Button:
						if b.Text == rotulo && b.Visible() && !b.Disabled() {
							tocar = b
						}
					}
				}
			}
		})
		if tocar != nil {
			fyne.DoAndWait(func() { test.Tap(tocar) })
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no hay un botón %q:\n%s", rotulo, textosEn(w))
}

// Ajustes → Datos ofrece borrar lo de antes de la migración SOLO de la app que migró y
// comprobó (Contacts aquí; Photos, que no tiene fototeca, no sale), dice cuánto ocupa,
// pide cerrar las apps que usan esos datos (no la propia), pide confirmarlo y borra solo
// el almacén viejo: la libreta sigue entera.
func TestBorrarLosDatosDeAntesDesdeAjustes(t *testing.T) {
	t.Setenv("SYTUE_IDIOMA", "es")
	l, w := appConDatosDePrueba(t)
	l.Espacio.ID = "instalacion-1"

	// Sin migración no sale nada.
	dir := l.Espacio.Rutas.Producto("contacts")
	contactosDeAntes(t, dir)
	fyne.DoAndWait(func() { w.SetContent(l.ajustesDatos(w)) })
	time.Sleep(300 * time.Millisecond)
	if texto := textosEn(w); strings.Contains(texto, "Datos de antes de la migración") {
		t.Fatalf("sin migración se ofrece borrar:\n%s", texto)
	}

	x, err := libreta.Abrir(t.Context(), dir, l.Espacio.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Cerrar()
	medido, err := libreta.MedirAntes(t.Context(), dir)
	if err != nil || medido.Ficheros == 0 {
		t.Fatal(medido, err)
	}
	var preguntadas [][]string
	abiertas := []string{"mail"}
	antes := abiertasDeLaEdicion
	abiertasDeLaEdicion = func(cs []string) ([]string, bool) {
		preguntadas = append(preguntadas, cs)
		return abiertas, true
	}
	t.Cleanup(func() { abiertasDeLaEdicion = antes })

	fyne.DoAndWait(func() { w.SetContent(l.ajustesDatos(w)) })
	texto := esperar(t, w, "Contacts: lo de antes ocupa")
	if strings.Contains(texto, "Photos:") {
		t.Errorf("sale Photos, que no ha migrado nada:\n%s", texto)
	}
	// Con Mail abierta, se pide cerrarla y no se borra nada.
	pulsarBoton(t, w, "Borrar los datos de antes de la migración")
	esperar(t, w, "cierra estas apps de Sytue Office: Mail")
	if len(preguntadas) != 1 || !slices.Equal(preguntadas[0], []string{"contacts", "hub", "mail", "calendar"}) {
		t.Errorf("pregunta por %v", preguntadas)
	}
	if !almacen.EsAlmacen(dir) {
		t.Fatal("con Mail abierta se ha borrado")
	}
	fyne.DoAndWait(func() {
		for _, o := range w.Canvas().Overlays().List() {
			w.Canvas().Overlays().Remove(o)
		}
	})

	// Cerrada: se confirma y se borra.
	abiertas = nil
	pulsarBoton(t, w, "Borrar los datos de antes de la migración")
	esperar(t, w, "Se borrarán los datos de Contacts de antes de la migración")
	pulsarBoton(t, w, "Borrar")
	esperar(t, w, "Contacts: datos de antes borrados")
	if almacen.EsAlmacen(dir) {
		t.Error("el almacén de antes sigue")
	}
	if _, err := os.Stat(filepath.Join(dir, "objetos")); !os.IsNotExist(err) {
		t.Error("objetos/ del almacén de antes sigue:", err)
	}
	todos, _, err := x.All(t.Context(), 10, 0)
	if err != nil || len(todos) != 1 || todos[0].FormattedName != "Ana García" {
		t.Fatalf("la libreta después de borrar: %v, %v", todos, err)
	}
	// Ya no hay nada que ofrecer.
	fyne.DoAndWait(func() { w.SetContent(l.ajustesDatos(w)) })
	time.Sleep(300 * time.Millisecond)
	if texto := textosEn(w); strings.Contains(texto, "Datos de antes de la migración") {
		t.Errorf("borrado, se sigue ofreciendo:\n%s", texto)
	}
}

// appConID es una app de prueba con el identificador de una app de la edición.
type appConID struct {
	fyne.App
	id string
}

func (a appConID) UniqueID() string { return a.id }

// La app propia no se pide cerrar: Photos, en sus Ajustes, solo pregunta por el Hub.
func TestLaPropiaAppNoSePideCerrar(t *testing.T) {
	t.Setenv("SYTUE_IDIOMA", "es")
	l, w := appConDatosDePrueba(t)
	l.A = appConID{App: l.A, id: "eu.sytue.office.photos"}
	var preguntadas []string
	antes := abiertasDeLaEdicion
	abiertasDeLaEdicion = func(cs []string) ([]string, bool) {
		preguntadas = cs
		return []string{"hub"}, true
	}
	t.Cleanup(func() { abiertasDeLaEdicion = antes })
	d := datosDeAntes{app: "photos", producto: "photos", medir: fototeca.MedirAntes, borrar: fototeca.BorrarAntes, usan: []string{"photos", "hub"}}
	fyne.DoAndWait(func() {
		l.borrarDeAntes(w, d, l.Espacio.Rutas.Producto("photos"), almacen.Antes{Migrada: true, Ficheros: 1, Bytes: 10}, widget.NewLabel(""), widget.NewButton("", nil))
	})
	esperar(t, w, "cierra estas apps de Sytue Office: Hub")
	if !slices.Equal(preguntadas, []string{"hub"}) {
		t.Errorf("pregunta por %v", preguntadas)
	}
}
