// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// servidorEnBoveda es el servidor de una persona con el Drive ya en la bóveda: el estado
// de su almacenamiento dice `private` y cualquier subida al Drive del servidor contesta
// 409, como el de verdad. Cuenta las dos cosas.
func servidorEnBoveda(t *testing.T, scope storage.Scope) (url string, estados, subidas *atomic.Int32) {
	t.Helper()
	estados, subidas = &atomic.Int32{}, &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/private-storage/migration") {
			estados.Add(1)
			ahora := time.Now().UTC()
			_ = json.NewEncoder(w).Encode(storage.LegacyMigrationState{
				Scope: scope, Mode: "private", Epoch: "6f1c2b9e-1d2a-4c55-9a0e-3f9a1b2c3d4e", StartedAt: &ahora,
			})
			return
		}
		subidas.Add(1)
		w.WriteHeader(http.StatusConflict)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, estados, subidas
}

// copiaEnFondoDePrueba deja la copia con la app cerrada sobre una galería con una foto
// por subir y un registro propio, y cuenta los avisos de que espera a la bóveda.
func copiaEnFondoDePrueba(t *testing.T) (*galeriaDeMentira, *Copia, *atomic.Int32) {
	t.Helper()
	g := nuevaGaleria(t)
	g.poner("1", "IMG_0001.JPG", "uno", tomaCopia)
	ruta := filepath.Join(t.TempDir(), "copia.json")
	antes, antesRuta, antesAviso := galeriaFondo, rutaFondo, avisoEsperaBoveda
	avisos := &atomic.Int32{}
	galeriaFondo, rutaFondo = g, func() (string, error) { return ruta, nil }
	avisoEsperaBoveda = func() { avisos.Add(1) }
	t.Cleanup(func() { galeriaFondo, rutaFondo, avisoEsperaBoveda = antes, antesRuta, antesAviso })
	c := CopiaDe(g, ruta)
	if err := c.CambiarAjustes(func(a *AjustesCopia) { a.Activa = true }); err != nil {
		t.Fatal(err)
	}
	return g, c, avisos
}

// TestLaCopiaEnFondoNoInsisteConElDriveEnLaBoveda: con el Drive de la persona en la
// bóveda (ADR 0086), la ronda con la app cerrada NO sube nada al Drive del servidor
// (que contestaría 409 una y otra vez): pregunta el estado una vez, se para sin contar
// fallos y deja las fotos para la copia que sí sube cifrado. Sin la App viva no abre
// nada, pero tampoco espera en silencio: avisa, una vez al día, de que hay que abrir la
// app; y vuelve a avisar si pasa un día más.
func TestLaCopiaEnFondoNoInsisteConElDriveEnLaBoveda(t *testing.T) {
	cuenta := sesion.IdentidadDeCuenta("https://id.example", "ana", "acme")
	scope, ok := scopeDeCuenta(cuenta)
	if !ok || scope != (storage.Scope{Tenant: "acme", Subject: "ana", Product: "drive"}) {
		t.Fatalf("scope = %+v, %v", scope, ok)
	}
	if _, ok := scopeDeCuenta("ana"); ok {
		t.Fatal("una identidad sin sus tres partes no da ámbito")
	}
	url, estados, subidas := servidorEnBoveda(t, scope)
	_, c, avisos := copiaEnFondoDePrueba(t)
	f := sesion.EnFondo{URL: func(string) string { return url }, Cuenta: cuenta}
	for range 3 {
		if r := copiaEnFondo(context.Background(), f); r != sesion.SinCambios {
			t.Fatalf("resultado = %v", r)
		}
	}
	if subidas.Load() != 0 || estados.Load() != 3 {
		t.Fatalf("subidas al servidor %d, estados %d", subidas.Load(), estados.Load())
	}
	if avisos.Load() != 1 || !c.EsperandoBoveda(cuenta) {
		t.Fatalf("avisos = %d: se avisa una vez, no en cada despertar", avisos.Load())
	}
	ahora := time.Now()
	c.ahora = func() time.Time { return ahora.Add(CadaAvisoBoveda + time.Minute) }
	t.Cleanup(func() { c.ahora = time.Now })
	if r := copiaEnFondo(context.Background(), f); r != sesion.SinCambios || avisos.Load() != 2 {
		t.Fatalf("pasado un día: %v, avisos = %d", r, avisos.Load())
	}

	// Con la App de esa cuenta viva y SIN la clave en el dispositivo, no se pide
	// desbloquear nada (no hay nadie para volver a entrar en Sytue ID): sigue esperando.
	a := &App{}
	a.cuenta.Store(cuenta)
	pedido := false
	a.GaleriaCifrada = func(_ context.Context, abrir bool) (*api.Drive, error) {
		pedido = true
		if abrir {
			t.Error("sin la clave en el dispositivo no se pide desbloquear nada")
		}
		return nil, ErrDriveCifradoCerrado
	}
	a.ClaveEnDispositivo = func() bool { return false }
	appEnFondo.Store(a)
	t.Cleanup(func() { appEnFondo.Store(nil) })
	if _, err := destinoEnFondo(context.Background(), f); !errors.Is(err, ErrCopiaEsperaBoveda) || !pedido {
		t.Fatalf("con el Drive cifrado cerrado: %v, pedido %v", err, pedido)
	}
}

// TestLaCopiaEnFondoAbreLaBovedaConLaClaveDelDispositivo: con la App de la cuenta viva
// y la clave de desbloqueo en el dispositivo, la ronda con la app cerrada pide a SU
// puerta que abra el Drive cifrado, espera a que esté y sube cifrado por él. Ni una
// subida al Drive del servidor, ni un aviso.
func TestLaCopiaEnFondoAbreLaBovedaConLaClaveDelDispositivo(t *testing.T) {
	cuenta := sesion.IdentidadDeCuenta("https://id.example", "ana", "acme")
	scope, _ := scopeDeCuenta(cuenta)
	url, _, subidas := servidorEnBoveda(t, scope)
	_, c, avisos := copiaEnFondoDePrueba(t)
	antesEspera, antesPaso := esperaBovedaFondo, pasoBovedaFondo
	esperaBovedaFondo, pasoBovedaFondo = 5*time.Second, 5*time.Millisecond
	t.Cleanup(func() { esperaBovedaFondo, pasoBovedaFondo = antesEspera, antesPaso })

	privado := &drivePrivadoFalso{t: t, fotos: map[string]string{}, carpeta: true}
	var abrir, consultas atomic.Int32
	a := &App{}
	a.cuenta.Store(cuenta)
	a.GaleriaCifrada = func(_ context.Context, pedirAbrir bool) (*api.Drive, error) {
		if pedirAbrir {
			abrir.Add(1)
			return nil, ErrDriveCifradoCerrado // la puerta abre fuera de esta llamada
		}
		// La puerta tarda: abierto, a la tercera consulta después de pedirlo.
		if abrir.Load() == 0 || consultas.Add(1) < 3 {
			return nil, ErrDriveCifradoCerrado
		}
		return api.NewDrive(url, nil, nil).WithPrivateData(privado)
	}
	var preguntas atomic.Int32
	a.ClaveEnDispositivo = func() bool { preguntas.Add(1); return true }
	appEnFondo.Store(a)
	t.Cleanup(func() { appEnFondo.Store(nil) })

	// Un despertar anterior ya había avisado: la ronda completa lo deja de esperar.
	c.con(cuenta, func(r *registroCopia) { r.AvisoBoveda = time.Now() })
	f := sesion.EnFondo{URL: func(string) string { return url }, Cuenta: cuenta}
	if r := copiaEnFondo(context.Background(), f); r != sesion.Nuevos {
		t.Fatalf("resultado = %v", r)
	}
	privado.mu.Lock()
	subidasCifradas := len(privado.fotos)
	privado.mu.Unlock()
	if abrir.Load() != 1 || preguntas.Load() != 1 || subidasCifradas != 1 {
		t.Fatalf("abrir %d, preguntas %d, subidas cifradas %d", abrir.Load(), preguntas.Load(), subidasCifradas)
	}
	if subidas.Load() != 0 || avisos.Load() != 0 || c.EsperandoBoveda(cuenta) {
		t.Fatalf("al servidor %d, avisos %d, sigue esperando %v", subidas.Load(), avisos.Load(), c.EsperandoBoveda(cuenta))
	}

	// Si la puerta no llega a abrir en el plazo, espera como sin clave (y avisa).
	g2 := c.galeria.(*galeriaDeMentira)
	g2.poner("2", "IMG_0002.JPG", "dos", tomaCopia.Add(time.Minute))
	esperaBovedaFondo = 30 * time.Millisecond
	abrir.Store(0)
	a.GaleriaCifrada = func(_ context.Context, pedirAbrir bool) (*api.Drive, error) {
		if pedirAbrir {
			abrir.Add(1)
		}
		return nil, ErrDriveCifradoCerrado
	}
	if r := copiaEnFondo(context.Background(), f); r != sesion.SinCambios || abrir.Load() != 1 || avisos.Load() != 1 {
		t.Fatalf("sin abrir a tiempo: %v, abrir %d, avisos %d", r, abrir.Load(), avisos.Load())
	}
}

// TestLaCopiaSigueEnCuantoSeAbreElDriveCifrado: con la app abierta, la ronda que se
// paró porque el Drive cifrado estaba cerrado sigue en cuanto se abre, sin esperar a la
// ronda siguiente; y la que dejó esperando la copia con la app cerrada, también.
func TestLaCopiaSigueEnCuantoSeAbreElDriveCifrado(t *testing.T) {
	g, servidor := nuevaGaleria(t), &driveFalso{}
	g.poner("1", "IMG_0001.JPG", "uno", tomaCopia)
	a := appConCopia(t, g, servidor)
	_ = a.Copia.CambiarAjustes(func(aj *AjustesCopia) { aj.Activa = true })
	privado := &drivePrivadoFalso{t: t, fotos: map[string]string{}, carpeta: true}
	var abierto atomic.Bool
	a.DriveDeSubida = func(context.Context) (*api.Drive, error) {
		if !abierto.Load() {
			return nil, ErrDriveCifradoCerrado
		}
		return a.Drive.WithPrivateData(privado)
	}
	subidasCifradas := func() int {
		privado.mu.Lock()
		defer privado.mu.Unlock()
		return len(privado.fotos)
	}
	a.Arrancar()
	esperarCopia(t, a)
	esperar(t, "la ronda no se paró esperando al Drive cifrado", func() bool { return errors.Is(a.cop.fallo, ErrDriveCifradoCerrado) })
	if subidasCifradas() != 0 {
		t.Fatal("subió con el Drive cifrado cerrado")
	}
	abierto.Store(true)
	fyne.DoAndWait(a.DriveCifradoAbierto)
	esperarCopia(t, a)
	esperar(t, "no siguió al abrirse el Drive cifrado", func() bool { return a.cop.fallo == nil && subidasCifradas() == 1 })

	// Lo que dejó esperando la copia con la app cerrada sigue igual al abrirse.
	g.poner("2", "IMG_0002.JPG", "dos", tomaCopia.Add(time.Minute))
	a.Copia.con(a.identidad, func(r *registroCopia) { r.AvisoBoveda = time.Now() })
	fyne.DoAndWait(a.DriveCifradoAbierto)
	esperarCopia(t, a)
	esperar(t, "no siguió lo que esperaba la copia en fondo", func() bool {
		return subidasCifradas() == 2 && !a.Copia.EsperandoBoveda(a.identidad)
	})
	if len(servidor.subidos) != 0 {
		t.Fatalf("subió al Drive del servidor: %v", servidor.subidos)
	}
}
