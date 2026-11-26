// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !edicionlocal

package ui

import (
	"encoding/json"
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
	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func suiteConSesionSalida(t *testing.T) *sesion.Suite {
	t.Helper()
	suite := suiteConSesion(t)
	if err := suite.Sesion.AdoptarCon(t.Context(), auth.Tokens{Issuer: suite.URL("id"), Cliente: auth.ClientID, Access: "tok", Refresh: "r", Subject: "ada", Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	previous := http.DefaultTransport
	http.DefaultTransport = transporteFunc(func(request *http.Request) (*http.Response, error) {
		var body any = map[string]any{}
		if strings.HasSuffix(request.URL.Path, "/.well-known/openid-configuration") {
			base := suite.URL("id")
			body = map[string]string{"issuer": base, "authorization_endpoint": base + "/authorize", "token_endpoint": base + "/token", "revocation_endpoint": base + "/revoke"}
		}
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(raw))), Request: request}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previous })
	return suite
}

// NAT-08, «avisar y luego borrar» (decisión del dueño, 2026-10-16): cerrar sesión con
// un borrador de Notepads sin subir pregunta antes —subir, descartar o no salir— y,
// descartado, el borrador no se queda en el disco.
func TestCerrarSesionAvisaDeLoQueNotepadsNoHaSubido(t *testing.T) {
	suite := suiteConSesionSalida(t)
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

// TestCerrarSesionSubeLoPendienteYApagaDescartarMientrasSube: elegir «Subir y cerrar
// sesión» envía el borrador antes de salir y, mientras sube, apaga también «Descartar y
// cerrar sesión» para que un clic a destiempo no borre el almacén a mitad de subida.
func TestCerrarSesionSubeLoPendienteYApagaDescartarMientrasSube(t *testing.T) {
	suite := suiteConSesionSalida(t)
	if err := suite.Sesion.AdoptarCon(t.Context(), auth.Tokens{Issuer: suite.URL("id"), Cliente: auth.ClientID, Access: "tok", Refresh: "r", Subject: "ada", Organizacion: "acme", Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	alm, err := sesion.Almacen()
	if err != nil {
		t.Fatal(err)
	}
	if err := alm.Write("ada/acme", notas.EspacioBorradores, "nb-1", "borrador por subir"); err != nil {
		t.Fatal(err)
	}
	llegada := make(chan struct{})
	soltar := make(chan struct{})
	var subidas, stateChecks atomic.Int32
	var liberar sync.Once
	t.Cleanup(func() { liberar.Do(func() { close(soltar) }); fyne.DoAndWait(func() {}) })
	prev := api.Transporte
	t.Cleanup(func() { api.Transporte = prev })
	mock := transporteFunc(func(r *http.Request) (*http.Response, error) {
		var response any
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/private-storage/migration"):
			scope := storage.Scope{Tenant: "acme", Subject: "ada", Product: "notes"}
			if err := storage.ValidateRequestScope(r.Header, scope); err != nil {
				t.Error("missing exact legacy capability scope", err)
			}
			stateChecks.Add(1)
			response = storage.AppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}}
		case strings.HasSuffix(r.URL.Path, "/.well-known/openid-configuration"):
			response = map[string]string{"issuer": suite.URL("id"), "authorization_endpoint": suite.URL("id") + "/authorize", "token_endpoint": suite.URL("id") + "/token", "revocation_endpoint": suite.URL("id") + "/revoke"}
		}
		if response != nil {
			raw, err := json.Marshal(response)
			if err != nil {
				return nil, err
			}
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
		}
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
	api.Transporte = mock
	previousHTTP := http.DefaultTransport
	http.DefaultTransport = mock
	t.Cleanup(func() { http.DefaultTransport = previousHTTP })

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
	liberar.Do(func() { close(soltar) })
	limite := time.Now().Add(5 * time.Second)
	for suite.Estado().Dentro() && time.Now().Before(limite) {
		fyne.DoAndWait(func() {})
		time.Sleep(10 * time.Millisecond)
	}
	if suite.Estado().Dentro() {
		t.Error("tras terminar de subir, la sesión sigue abierta")
	}
	esperarCierreCuenta(t, c)
	if stateChecks.Load() != 1 {
		t.Fatal("legacy capture was not explicitly classified before plaintext submission", stateChecks.Load())
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

func TestCerrarSesionMuestraFalloCompartidoYPermiteReintentar(t *testing.T) {
	suite := suiteConSesionSalida(t)
	var intentos atomic.Int32
	previous := http.DefaultTransport
	http.DefaultTransport = transporteFunc(func(request *http.Request) (*http.Response, error) {
		if strings.HasSuffix(request.URL.Path, "/revoke") && intentos.Add(1) == 1 {
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"temporarily_unavailable"}`)), Request: request}, nil
		}
		return previous.RoundTrip(request)
	})
	t.Cleanup(func() { http.DefaultTransport = previous })
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
	if !suite.Estado().Dentro() {
		t.Fatal("failed revocation erased the active session")
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
	if intentos.Load() != 2 || w.Canvas().Overlays().Top() != nil || suite.Estado().Dentro() {
		t.Fatal("reintentar no completó el cierre")
	}
}

func esperarCierreCuenta(t *testing.T, c *cuentaBarra) {
	t.Helper()
	terminado := func() bool { c.mu.Lock(); defer c.mu.Unlock(); return !c.cerrando }
	// La marca compartida requiere sincronizar el disco, que bajo race y la carga
	// del CI puede tardar más que una animación. El presupuesto de respuesta de la
	// interfaz se comprueba aparte en TestCerrarSesionNoBloqueaUIYNoDuplicaRetirada.
	plazo := time.NewTimer(10 * time.Second)
	defer plazo.Stop()
	paso := time.NewTicker(20 * time.Millisecond)
	defer paso.Stop()
	for {
		fyne.DoAndWait(func() {})
		if terminado() {
			return
		}
		select {
		case <-plazo.C:
			t.Fatal("el cierre compartido no terminó")
		case <-paso.C:
		}
	}
}

func TestCerrarSesionNoBloqueaUIYNoDuplicaRetirada(t *testing.T) {
	suite := suiteConSesionSalida(t)
	release, err := ajustes.CerrojoSesion(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var liberar sync.Once
	t.Cleanup(func() { liberar.Do(release) })
	var intentos atomic.Int32
	previous := http.DefaultTransport
	http.DefaultTransport = transporteFunc(func(request *http.Request) (*http.Response, error) {
		if strings.HasSuffix(request.URL.Path, "/revoke") {
			intentos.Add(1)
		}
		return previous.RoundTrip(request)
	})
	t.Cleanup(func() { http.DefaultTransport = previous })
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
	deadline := time.Now().Add(5 * time.Second)
	account := (&auth.OIDC{Issuer: suite.URL("id")}).Almacen()
	paused := false
	for time.Now().Before(deadline) {
		access, err := ajustes.LeerAccesoSesion(account.Identificador())
		if err == nil && access.Suspendido {
			paused = true
			break
		}
		fyne.DoAndWait(func() {})
		time.Sleep(10 * time.Millisecond)
	}
	if !paused {
		t.Fatal("close did not reach its shared-session barrier")
	}

	c.cerrarSesion()
	if intentos.Load() != 0 {
		t.Fatal("una segunda pulsación duplicó la retirada")
	}
	liberar.Do(release)
	esperarCierreCuenta(t, c)
	if suite.Estado().Dentro() || intentos.Load() != 1 {
		t.Fatal("el cierre no terminó una sola vez")
	}
}

func TestElDialogoDeSalidaConservaSuCuentaOriginalYPermiteRefresh(t *testing.T) {
	for _, scenario := range []string{"other-account", "aba", "shared-refresh"} {
		t.Run(scenario, func(t *testing.T) {
			suite := suiteConSesionSalida(t)
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			journal, err := sesion.Almacen()
			if err != nil {
				t.Fatal(err)
			}
			if err := journal.Write("ada/", notas.EspacioBorradores, "retained-a", "A's unsent content"); err != nil {
				t.Fatal(err)
			}
			c := nuevaCuentaBarra(traductorPrueba(t), suite)
			window := test.NewTempWindow(t, c.boton)
			window.Resize(fyne.NewSize(1280, 800))
			c.salir(1)
			overlay, _ := window.Canvas().Overlays().Top().(*capa)
			if overlay == nil {
				t.Fatal("exit dialog absent")
			}
			d := overlay.panel.(*Dialogo)
			var discard *Boton
			for _, object := range d.Pie.(*fyne.Container).Objects {
				if button, ok := object.(*Boton); ok && button.Texto == c.t.T("account.signOutPending.discard", nil) {
					discard = button
				}
			}
			if discard == nil {
				t.Fatal("discard action absent")
			}
			tokens := auth.Tokens{Issuer: suite.URL("id"), Cliente: auth.ClientID, Access: "tok", Refresh: "new-refresh", Subject: "ada", Expiry: time.Now().Add(time.Hour)}
			account := (&auth.OIDC{Issuer: suite.URL("id")}).Almacen()
			if scenario == "shared-refresh" {
				release, err := ajustes.CerrojoSesion(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				err = account.Guardar(tokens)
				release()
				if err != nil {
					t.Fatal(err)
				}
				suite.Sesion.Sincronizar(tokens)
			} else {
				tokens.Subject = "bea"
				if err := suite.Sesion.AdoptarCon(t.Context(), tokens); err != nil {
					t.Fatal(err)
				}
				if scenario == "aba" {
					tokens.Subject = "ada"
					if err := suite.Sesion.AdoptarCon(t.Context(), tokens); err != nil {
						t.Fatal(err)
					}
				}
			}
			test.Tap(discard)
			esperarCierreCuenta(t, c)
			remaining, err := journal.CountOwner("ada/", notas.EspacioBorradores)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "shared-refresh" {
				if suite.Estado().Dentro() || remaining != 0 {
					t.Fatal("same nonce refresh blocked authorized close", remaining)
				}
			} else {
				saved, err := account.Cargar()
				if err != nil || saved.Subject != tokens.Subject || !suite.Estado().Dentro() || remaining != 1 {
					t.Fatal("old dialog closed a newer login or discarded A", remaining, err)
				}
			}
		})
	}
}
