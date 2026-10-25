// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
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
	esperarCierreCuenta(t, c)
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
	esperarCierreCuenta(t, c)
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

func TestCerrarSesionMuestraFalloCompartidoYPermiteReintentar(t *testing.T) {
	suite := suiteConSesion(t)
	intentos := 0
	almacen := (&auth.OIDC{Issuer: suite.URL("id")}).Almacen()
	suite.Sesion = auth.NewSession(auth.Tokens{Refresh: "sintetico", Subject: "ana"}, nil, func(tokens auth.Tokens) error {
		intentos++
		if intentos == 1 {
			return errors.New("ruta no escribible")
		}
		return almacen.Guardar(tokens)
	})
	c := nuevaCuentaBarra(traductorPrueba(t), suite)
	w := test.NewTempWindow(t, c.boton)
	w.Resize(fyne.NewSize(1280, 800))
	c.salir(0)
	esperarCierreCuenta(t, c)
	cp, _ := w.Canvas().Overlays().Top().(*capa)
	if cp == nil {
		t.Fatal("el cierre fallido no avisa a la persona")
	}
	d, _ := cp.panel.(*Dialogo)
	if d == nil || d.Titulo != c.t.T("account.signOut", nil) {
		t.Fatal("no muestra el fallo de cierre")
	}
	p, ok := d.Cuerpo.(*Parrafo)
	if !ok || p.Texto != c.t.T("account.signOutFailed", nil) {
		t.Fatal("no explica el acceso compartido pendiente")
	}
	if suite.Estado().Dentro() {
		t.Fatal("el error ha restaurado la sesión local")
	}
	var reintentar *Boton
	for _, o := range d.Pie.(*fyne.Container).Objects {
		if b, ok := o.(*Boton); ok && b.Texto == c.t.T("action.retry", nil) {
			reintentar = b
		}
	}
	if reintentar == nil {
		t.Fatal("no ofrece reintentar")
	}
	test.Tap(reintentar)
	esperarCierreCuenta(t, c)
	if intentos != 2 || w.Canvas().Overlays().Top() != nil || suite.Estado().Dentro() {
		t.Fatal("reintentar no completó el cierre")
	}
}

func esperarCierreCuenta(t *testing.T, c *cuentaBarra) {
	t.Helper()
	terminado := func() bool { c.mu.Lock(); defer c.mu.Unlock(); return !c.cerrando }
	esperar(t, terminado)
	if !terminado() {
		t.Fatal("el cierre compartido no terminó")
	}
}

func TestCerrarSesionNoBloqueaUIYNoDuplicaRetirada(t *testing.T) {
	suite := suiteConSesion(t)
	llegada, soltar := make(chan struct{}), make(chan struct{})
	var intentos atomic.Int32
	var liberar sync.Once
	t.Cleanup(func() { liberar.Do(func() { close(soltar) }) })
	almacen := (&auth.OIDC{Issuer: suite.URL("id")}).Almacen()
	suite.Sesion = auth.NewSession(auth.Tokens{Refresh: "sintetico", Subject: "ana"}, nil, func(tokens auth.Tokens) error {
		if intentos.Add(1) == 1 {
			close(llegada)
		}
		<-soltar
		return almacen.Guardar(tokens)
	})
	c := nuevaCuentaBarra(traductorPrueba(t), suite)
	w := test.NewTempWindow(t, c.boton)
	w.Resize(fyne.NewSize(1280, 800))
	vuelve := make(chan struct{})
	go func() { c.cerrarSesion(); close(vuelve) }()
	select {
	case <-vuelve:
	case <-time.After(5 * time.Second):
		t.Fatal("cerrar sesión bloquea la interfaz esperando el almacén")
	}
	select {
	case <-llegada:
	case <-time.After(5 * time.Second):
		t.Fatal("el cierre no llegó al almacén")
	}
	c.cerrarSesion()
	if intentos.Load() != 1 {
		t.Fatal("una segunda pulsación duplicó la retirada")
	}
	liberar.Do(func() { close(soltar) })
	esperarCierreCuenta(t, c)
	if suite.Estado().Dentro() || intentos.Load() != 1 {
		t.Fatal("el cierre no terminó una sola vez")
	}
}
