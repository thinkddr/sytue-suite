// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
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

type transporteFunc func(*http.Request) (*http.Response, error)

func (f transporteFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestCerrarSesionSubeLoPendienteYApagaDescartarMientrasSube: elegir «Subir y cerrar
// sesión» envía el borrador antes de salir y, mientras sube, apaga también «Descartar y
// cerrar sesión» para que un clic a destiempo no borre el almacén a mitad de subida.
func TestCerrarSesionSubeLoPendienteYApagaDescartarMientrasSube(t *testing.T) {
	suite := suiteConSesion(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	alm, err := sesion.Almacen()
	if err != nil {
		t.Fatal(err)
	}
	if err := alm.Write("ada/", notas.EspacioBorradores, "nb-1", "borrador por subir"); err != nil {
		t.Fatal(err)
	}
	llegada := make(chan struct{})
	soltar := make(chan struct{})
	var subidas atomic.Int32
	prev := api.Transporte
	t.Cleanup(func() { api.Transporte = prev })
	api.Transporte = transporteFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notebooks/nb-1/notes") {
			if subidas.Add(1) == 1 {
				close(llegada)
			}
			<-soltar
			return &http.Response{
				StatusCode: http.StatusCreated,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":"n-1"}`)),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{}`)),
		}, nil
	})

	c := nuevaCuentaBarra(traductorPrueba(t), suite)
	w := test.NewTempWindow(t, c.boton)
	w.Resize(fyne.NewSize(1280, 800))

	c.salir(sesion.PendientesNotepads())
	cp, _ := w.Canvas().Overlays().Top().(*capa)
	if cp == nil {
		t.Fatal("no se abrió el diálogo de aviso al salir")
	}
	d := cp.panel.(*Dialogo)
	var descartar, subir *Boton
	for _, o := range d.Pie.(*fyne.Container).Objects {
		b, ok := o.(*Boton)
		if !ok {
			continue
		}
		switch b.Texto {
		case c.t.T("account.signOutPending.discard", nil):
			descartar = b
		case c.t.T("account.signOutPending.upload", nil):
			subir = b
		}
	}
	if descartar == nil || subir == nil {
		t.Fatal("faltan los botones de descartar o subir")
	}
	test.Tap(subir)
	select {
	case <-llegada:
	case <-time.After(5 * time.Second):
		t.Fatal("la subida del borrador no llegó a empezar")
	}
	if !descartar.Inerte {
		t.Error("mientras sube lo pendiente, «Descartar y cerrar sesión» sigue activo")
	}
	test.Tap(descartar)
	if !suite.Estado().Dentro() {
		t.Fatal("pulsar «Descartar» mientras sube cerró la sesión a mitad de subida")
	}
	close(soltar)
	limite := time.Now().Add(5 * time.Second)
	for suite.Estado().Dentro() && time.Now().Before(limite) {
		fyne.DoAndWait(func() {})
		time.Sleep(10 * time.Millisecond)
	}
	if suite.Estado().Dentro() {
		t.Error("tras terminar de subir, la sesión sigue abierta")
	}
	if subidas.Load() != 1 {
		t.Errorf("subidas = %d; quiero 1", subidas.Load())
	}
	if n := sesion.PendientesNotepads(); n != 0 {
		t.Errorf("tras subir y salir quedan %d pendientes en el disco", n)
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
