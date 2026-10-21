// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
)

// NAT-08, «avisar y luego borrar» (decisión del dueño, 2026-10-16): cerrar sesión con
// un borrador de Notepads sin subir pregunta antes —subir, descartar o no salir— y,
// descartado, el borrador no se queda en el disco.
func TestCerrarSesionAvisaDeLoQueNotepadsNoHaSubido(t *testing.T) {
	suite := suiteConSesion(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	alm, err := sesion.Almacen()
	if err != nil {
		t.Fatal(err)
	}
	if err := alm.Write("ada/", notas.EspacioBorradores, "nb-1", "sin subir"); err != nil {
		t.Fatal(err)
	}
	c := nuevaCuentaBarra(traductorPrueba(t), suite)
	w := test.NewTempWindow(t, c.boton)
	w.Resize(fyne.NewSize(1280, 800))

	n := sesion.PendientesNotepads()
	if n != 1 {
		t.Fatalf("pendientes %d; quiero 1", n)
	}
	c.salir(n)
	cp, _ := w.Canvas().Overlays().Top().(*capa)
	if cp == nil {
		t.Fatal("cerrar sesión con un borrador sin subir no ha preguntado")
	}
	d, _ := cp.panel.(*Dialogo)
	if d == nil || d.Titulo != c.t.T("account.signOutPending.title", nil) {
		t.Fatalf("el diálogo no es el de lo no subido: %+v", cp.panel)
	}
	if !suite.Estado().Dentro() {
		t.Fatal("ha cerrado sesión sin esperar la respuesta")
	}
	var descartar *Boton
	for _, o := range d.Pie.(*fyne.Container).Objects {
		if b, ok := o.(*Boton); ok && b.Texto == c.t.T("account.signOutPending.discard", nil) {
			descartar = b
		}
	}
	if descartar == nil || len(d.Pie.(*fyne.Container).Objects) != 3 {
		t.Fatal("el diálogo no ofrece cancelar, descartar y subir")
	}
	test.Tap(descartar)
	if suite.Estado().Dentro() {
		t.Error("descartar no ha cerrado la sesión")
	}
	if n := sesion.PendientesNotepads(); n != 0 {
		t.Errorf("tras cerrar sesión quedan %d cosas de Notepads en el disco", n)
	}
}

// Las cuatro claves del aviso, en los 26 idiomas.
func TestElAvisoDeSalirEstaTraducido(t *testing.T) {
	for _, locale := range i18n.Locales {
		tr, err := i18n.New(locale, i18n.Common())
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range []string{"title", "body", "upload", "discard"} {
			clave := "account.signOutPending." + l
			if got := tr.T(clave, i18n.Vars{"count": 2}); got == clave || strings.TrimSpace(got) == "" || l == "body" && !strings.Contains(got, "2") {
				t.Errorf("%s: falta %s (%q)", locale, clave, got)
			}
		}
	}
}
