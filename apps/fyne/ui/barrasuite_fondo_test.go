// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLaBarraNoAbreElFlujoSinEscena: con la app lanzada por iOS solo para refrescar su
// widget (sin escena), montar la barra no pide los avisos ni abre el SSE del Hub; al
// conectar la escena, sí (sesion.Suite.CuandoHayaEscena).
//
// VISTO EN ROJO: con `b.Arrancar()` directo en Montar, la lista se pide y el flujo se
// abre sin escena.
func TestLaBarraNoAbreElFlujoSinEscena(t *testing.T) {
	hub := &hubDeMentira{enviar: make(chan string, 4), conexion: make(chan struct{}, 4)}
	srv := httptest.NewServer(hub)
	t.Cleanup(srv.Close)
	s := suiteConSesion(t)
	escena := s.ProbarSinEscena()
	s.Hub = api.NewHub(srv.URL, sesionDePrueba{}, nil)
	t.Cleanup(func() { fyne.CurrentApp().Lifecycle().SetOnEnteredForeground(nil) })
	b := NuevaBarraSuite(traductorPrueba(t), s, iconos.Mail)
	b.cuenta.cuenta = api.NewAccount(srv.URL, sesionDePrueba{}, nil)
	t.Cleanup(b.Parar)
	w := test.NewWindow(b.Montar(widget.NewLabel("contenido")))
	t.Cleanup(w.Close)

	fyne.DoAndWait(func() {})
	time.Sleep(200 * time.Millisecond)
	var listas int
	hub.leer(func() { listas = hub.listas })
	select {
	case <-hub.conexion:
		t.Fatal("sin escena, la barra ha abierto el flujo del Hub")
	default:
	}
	if listas != 0 {
		t.Fatalf("sin escena, la barra ha pedido %d veces los avisos", listas)
	}

	escena.Conectar()
	esperarConexion(t, hub)
}

// TestLaBarraCierraElFlujoAlFondoEnIOS: con la app mandada al fondo (sin deslizarla),
// iOS la reanuda para su widget y el flujo se reconectaba sin nadie mirando. Allí
// (sesion.Suite.PausaEnFondo) la barra cierra el flujo al dejar el primer plano, no lo
// abre mientras tanto, y al volver lo abre y vuelve a pedir la lista.
//
// VISTO EN ROJO: sin apuntar dormir a AlFondo, el flujo sigue abierto con la app al fondo.
func TestLaBarraCierraElFlujoAlFondoEnIOS(t *testing.T) {
	hub := &hubDeMentira{enviar: make(chan string, 4), conexion: make(chan struct{}, 4)}
	var abiertos atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/stream") {
			abiertos.Add(1)
			defer abiertos.Add(-1)
		}
		hub.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	s := suiteConSesion(t)
	escena := s.ProbarSinEscena()
	s.Hub = api.NewHub(srv.URL, sesionDePrueba{}, nil)
	t.Cleanup(func() { fyne.CurrentApp().Lifecycle().SetOnEnteredForeground(nil) })
	b := NuevaBarraSuite(traductorPrueba(t), s, iconos.Mail)
	b.cuenta.cuenta = api.NewAccount(srv.URL, sesionDePrueba{}, nil)
	t.Cleanup(b.Parar)
	w := test.NewWindow(b.Montar(widget.NewLabel("contenido")))
	t.Cleanup(w.Close)
	escena.Conectar()
	esperarConexion(t, hub)

	esperar := func(que string, f func() bool) {
		t.Helper()
		limite := time.Now().Add(5 * time.Second)
		for !f() {
			if time.Now().After(limite) {
				t.Fatal(que)
			}
			fyne.DoAndWait(func() {})
			time.Sleep(10 * time.Millisecond)
		}
	}
	fyne.DoAndWait(func() { s.ProbarPrimerPlano(false) })
	esperar("con la app al fondo, el flujo del Hub sigue abierto", func() bool { return abiertos.Load() == 0 })
	var listas int
	hub.leer(func() { listas = hub.listas })
	time.Sleep(1500 * time.Millisecond) // lo que tardaría en reconectar tras un corte
	select {
	case <-hub.conexion:
		t.Fatal("con la app al fondo, la barra ha vuelto a abrir el flujo")
	default:
	}
	if abiertos.Load() != 0 {
		t.Fatal("con la app al fondo, la barra ha vuelto a abrir el flujo")
	}

	fyne.DoAndWait(func() { s.ProbarPrimerPlano(true) })
	esperarConexion(t, hub)
	esperar("al volver no se vuelve a pedir la lista", func() bool {
		var n int
		hub.leer(func() { n = hub.listas })
		return n > listas
	})
}
