// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// bancoFondo es una suite con sesión lanzada como la lanza iOS para su widget: sin
// escena, con un Hub que cuenta lo que se le pide y una Fuente que cuenta sus lecturas.
type bancoFondo struct {
	s        *Suite
	escena   *EscenaDePrueba
	hub      atomic.Int32
	lecturas atomic.Int32
	fallar   atomic.Bool
}

func nuevoBancoFondo(t *testing.T) *bancoFondo {
	t.Helper()
	aparte(t)
	t.Cleanup(widgets.SimularLector(true))
	app := test.NewApp()
	t.Cleanup(app.Quit)
	b := &bancoFondo{}
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		b.hub.Add(1)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(hub.Close)
	b.s = suiteConSesion(t, widgetApp{app}, "ana")
	b.escena = b.s.ProbarSinEscena()
	b.s.Hub = api.NewHub(hub.URL, tokenFijo("tok"), nil)
	// Los temporizadores del Publicador, a mano: aquí solo publica el refresco.
	b.s.ProbarWidgetsCon(widgets.NuevoRelojDePrueba(time.Now()).Opciones(widgets.OpcionesPublicador{}))
	b.s.PublicadorWidget(func(context.Context) (widgets.Detail, error) {
		b.lecturas.Add(1)
		if b.fallar.Load() {
			return widgets.Detail{}, errors.New("sin red")
		}
		return conDocumentos(7), nil
	})
	return b
}

// TestLanzadaSinEscenaSoloRefrescaElWidget: con la app lanzada por iOS para su tarea de
// refresco no se late «En línea» ni se pide nada al Hub (ni presencia ni, desde la
// barra, el SSE), y las cargas de los `cmd` esperan; el refresco lee con la Fuente y
// publica. Al conectar la escena, todo eso arranca.
//
// VISTO EN ROJO: con `go s.latir(ctx)` fuera de CuandoHayaEscena, el Hub recibe la
// presencia antes de la escena.
func TestLanzadaSinEscenaSoloRefrescaElWidget(t *testing.T) {
	b := nuevoBancoFondo(t)
	ctx, parar := context.WithCancel(context.Background())
	t.Cleanup(parar)
	var cargada atomic.Bool
	b.s.CuandoHayaEscena(func() { cargada.Store(true) }) // el Arrancar de un `cmd`
	b.s.Arrancar(ctx)
	if !b.s.LanzadoEnFondo() {
		t.Fatal("sin escena, la suite no se sabe lanzada en segundo plano")
	}
	fyne.DoAndWait(func() {})
	time.Sleep(200 * time.Millisecond) // lo que latir tardaría en pedir
	if n := b.hub.Load(); n != 0 {
		t.Fatalf("sin escena se han hecho %d peticiones al Hub (presencia)", n)
	}
	if cargada.Load() {
		t.Fatal("sin escena ha corrido la carga del cmd")
	}

	ok, reintento, cada := b.escena.Refrescar(ctx)
	if !ok || reintento != 0 || cada != CadaRefrescoEnFondo {
		t.Fatalf("refresco = %v, reintento %v, cada %v", ok, reintento, cada)
	}
	if b.lecturas.Load() != 1 || publicado(t, "eu.sytue.docus") != 7 {
		t.Fatalf("el refresco no ha publicado: %d lecturas, %d en el widget", b.lecturas.Load(), publicado(t, "eu.sytue.docus"))
	}
	if n := b.hub.Load(); n != 0 {
		t.Fatalf("el refresco ha hecho %d peticiones al Hub", n)
	}

	b.escena.Conectar()
	limite := time.Now().Add(5 * time.Second)
	for !cargada.Load() || b.hub.Load() == 0 {
		if time.Now().After(limite) {
			t.Fatalf("con escena no arranca: cargada %v, %d peticiones al Hub", cargada.Load(), b.hub.Load())
		}
		fyne.DoAndWait(func() {})
		time.Sleep(10 * time.Millisecond)
	}
	if b.s.LanzadoEnFondo() {
		t.Fatal("con escena, la suite se sigue creyendo en segundo plano")
	}
}

// TestElRefrescoConElDispositivoBloqueadoSeAplaza: con los datos protegidos fuera de
// alcance no se lee nada y se pide otro intento a los 5 min; desbloqueado, sí.
//
// VISTO EN ROJO: sin la comprobación de ProtectedDataAvailable, lee con el teléfono
// bloqueado.
func TestElRefrescoConElDispositivoBloqueadoSeAplaza(t *testing.T) {
	b := nuevoBancoFondo(t)
	b.s.Arrancar(t.Context())
	b.escena.Bloquear(true)
	if r := b.s.RefrescarWidgetEnFondo(t.Context()); r != Aplazado {
		t.Fatalf("bloqueado: %v", r)
	}
	if ok, reintento, _ := b.escena.Refrescar(t.Context()); !ok || reintento != ReintentoAplazado {
		t.Fatalf("bloqueado, el sistema recibe %v y reintento %v", ok, reintento)
	}
	if n := b.lecturas.Load(); n != 0 {
		t.Fatalf("con el dispositivo bloqueado se ha leído %d veces", n)
	}
	b.escena.Bloquear(false)
	if r := b.s.RefrescarWidgetEnFondo(t.Context()); r != Nuevos {
		t.Fatalf("desbloqueado: %v", r)
	}
	// Lo mismo otra vez: lo publicado no cambia.
	if r := b.s.RefrescarWidgetEnFondo(t.Context()); r != SinCambios {
		t.Fatalf("sin cambios: %v", r)
	}
}

// TestElRefrescoNoLeeConElBloqueoYDiceElFallo: un fallo de red se le dice al sistema
// sin borrar lo publicado, y con el bloqueo de la suite no se lee.
func TestElRefrescoNoLeeConElBloqueoYDiceElFallo(t *testing.T) {
	b := nuevoBancoFondo(t)
	b.s.Arrancar(t.Context())
	if r := b.s.RefrescarWidgetEnFondo(t.Context()); r != Nuevos {
		t.Fatalf("primer refresco: %v", r)
	}
	b.fallar.Store(true)
	if ok, _, _ := b.escena.Refrescar(t.Context()); ok {
		t.Fatal("un fallo de red se le dice al sistema como éxito")
	}
	if n := publicado(t, "eu.sytue.docus"); n != 7 {
		t.Fatalf("un fallo de red ha borrado lo publicado (%d)", n)
	}
	b.fallar.Store(false)
	antes := b.lecturas.Load()
	if err := ajustes.Cambiar(func(a *ajustes.Ajustes) { a.Bloqueo = true }); err != nil {
		t.Fatal(err)
	}
	if r := b.s.RefrescarWidgetEnFondo(t.Context()); r != SinCambios || b.lecturas.Load() != antes {
		t.Fatalf("con el bloqueo de la suite: %v, %d lecturas", r, b.lecturas.Load()-antes)
	}
}

// TestLosCambiosSinEscenaSeAvisanAlConectar: sin escena, los AlCambiar de las apps no
// se llaman (cargarían la pantalla); el del widget, sí. Al conectar, cada app se entera
// una vez, antes que lo de CuandoHayaEscena.
//
// VISTO EN ROJO: sin retener en avisarA, el AlCambiar de la app corre sin escena.
func TestLosCambiosSinEscenaSeAvisanAlConectar(t *testing.T) {
	aparte(t)
	app := test.NewApp()
	defer app.Quit()
	s := suiteConSesion(t, widgetApp{app}, "ana")
	e := s.ProbarSinEscena()
	var mu sync.Mutex
	var orden []string
	apuntar := func(que string) { mu.Lock(); orden = append(orden, que); mu.Unlock() }
	leer := func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), orden...) }
	s.AlCambiar(func(Estado) { apuntar("app") })
	s.alCambiarSinEscena(func(Estado) { apuntar("widget") })
	s.CuandoHayaEscena(func() { apuntar("escena") })
	s.avisar()
	s.avisar()
	fyne.DoAndWait(func() {})
	if got := leer(); len(got) != 2 || got[0] != "widget" || got[1] != "widget" {
		t.Fatalf("sin escena: %v", got)
	}
	e.Conectar()
	fyne.DoAndWait(func() {})
	if got := leer(); len(got) != 4 || got[2] != "app" || got[3] != "escena" {
		t.Fatalf("al conectar: %v (una vez la app, y luego la escena)", got)
	}
	s.avisar()
	fyne.DoAndWait(func() {})
	if got := leer(); len(got) != 6 {
		t.Fatalf("con escena, los avisos no llegan a todos: %v", got)
	}
}

// TestEntrarSinEscenaCargaUnaVez: la sesión entra sin escena (lanzada con el teléfono
// bloqueado, el vigilante la adopta al desbloquear) y luego se toca el icono. El cambio
// retenido llega al AlCambiar justo antes que lo de CuandoHayaEscena, y un `cmd` que
// cargaba en los dos sitios (bases, erp, hub, contacts, drive) pedía su pantalla dos
// veces a la vez. Con el patrón de esos `cmd` —el AlCambiar no carga hasta que corre lo
// de la escena— se carga una, y lo que entra después con escena carga otra.
//
// VISTO EN ROJO: sin `conEscena` en el AlCambiar, 2 cargas al conectar.
func TestEntrarSinEscenaCargaUnaVez(t *testing.T) {
	aparte(t)
	app := test.NewApp()
	defer app.Quit()
	s := &Suite{app: widgetApp{app}, Sesion: auth.NewSession(auth.Tokens{}, nil, nil)}
	s.visto = ajustes.Leer()
	e := s.ProbarSinEscena()
	var cargas atomic.Int32
	// El `main` de bases, tal cual.
	dentro, conEscena := s.Estado().Dentro(), false
	s.AlCambiar(func(e Estado) {
		if e.Dentro() && !dentro && conEscena {
			cargas.Add(1)
		}
		dentro = e.Dentro()
	})
	s.CuandoHayaEscena(func() {
		conEscena = true
		if dentro {
			cargas.Add(1)
		}
	})
	guardar(t, "ana", "r1")
	s.aplicar(ajustes.Leer()) // el vigilante, al desbloquear
	if !s.Estado().Dentro() {
		t.Fatal("la prueba no adopta la sesión sin escena")
	}
	fyne.DoAndWait(func() {})
	if n := cargas.Load(); n != 0 {
		t.Fatalf("sin escena se ha cargado %d veces", n)
	}
	e.Conectar()
	fyne.DoAndWait(func() {})
	fyne.DoAndWait(func() {})
	if n := cargas.Load(); n != 1 {
		t.Fatalf("al conectar con la sesión entrada sin escena se ha cargado %d veces, y es una", n)
	}
	// Con escena, salir y volver a entrar carga otra vez, desde el AlCambiar.
	guardar(t, "", "")
	s.aplicar(ajustes.Leer())
	guardar(t, "ana", "r2")
	s.aplicar(ajustes.Leer())
	fyne.DoAndWait(func() {})
	if n := cargas.Load(); n != 2 {
		t.Fatalf("con escena, entrar otra vez carga %d veces en total, y son dos", n)
	}
}

// TestConEscenaTodoEsComoAntes: sin la API del fork (otro Fyne, escritorio, Android)
// CuandoHayaEscena corre en el acto y los AlCambiar no se retienen.
func TestConEscenaTodoEsComoAntes(t *testing.T) {
	aparte(t)
	app := test.NewApp()
	defer app.Quit()
	s := Abrir(widgetApp{app}, "sytue.eu")
	ya := false
	s.CuandoHayaEscena(func() { ya = true })
	if !ya || s.LanzadoEnFondo() {
		t.Fatal("sin la API de fondo del driver, CuandoHayaEscena espera")
	}
	avisado := make(chan struct{}, 1)
	s.AlCambiar(func(Estado) { avisado <- struct{}{} })
	s.avisar()
	fyne.DoAndWait(func() {})
	select {
	case <-avisado:
	default:
		t.Fatal("con escena, un AlCambiar no se avisa")
	}
	if r := s.RefrescarWidgetEnFondo(t.Context()); r != SinCambios {
		t.Fatalf("sin publicador, el refresco hace algo: %v", r)
	}
}

// TestElDespertarEnCalienteNoLate: la app abierta una vez y mandada al fondo, sin
// deslizarla, sigue en memoria; iOS la reanuda para su tarea de refresco y todo lo que
// esperaba vuelve a correr. Ni ese refresco ni el latido que venció mientras estaba
// suspendida publican «En línea»: la presencia sigue al primer plano, no a la escena. Al
// volver, se late en el acto.
//
// VISTO EN ROJO: sin la comprobación de latidoEnPausa en latir, el Hub recibe PUT
// /presence con la app al fondo.
func TestElDespertarEnCalienteNoLate(t *testing.T) {
	aparte(t)
	t.Cleanup(widgets.SimularLector(true))
	app := test.NewApp()
	t.Cleanup(app.Quit)
	var latidos atomic.Int32
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/presence") {
			latidos.Add(1)
			_, _ = w.Write([]byte(`{"heartbeat_seconds":1}`)) // que el siguiente venza pronto
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(hub.Close)
	s := suiteConSesion(t, widgetApp{app}, "ana")
	escena := s.ProbarSinEscena()
	s.Hub = api.NewHub(hub.URL, tokenFijo("tok"), nil)
	s.ProbarWidgetsCon(widgets.NuevoRelojDePrueba(time.Now()).Opciones(widgets.OpcionesPublicador{}))
	s.PublicadorWidget(func(context.Context) (widgets.Detail, error) { return conDocumentos(7), nil })
	ctx, parar := context.WithCancel(context.Background())
	t.Cleanup(parar)
	s.Arrancar(ctx)
	escena.Conectar()
	s.ProbarPrimerPlano(true)
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
	esperar("con la app en primer plano no se late", func() bool { return latidos.Load() > 0 })

	s.ProbarPrimerPlano(false)         // al fondo, sin deslizarla
	time.Sleep(100 * time.Millisecond) // el latido que estuviera en vuelo
	antes := latidos.Load()
	if r := s.RefrescarWidgetEnFondo(ctx); r != Nuevos && r != SinCambios {
		t.Fatalf("el refresco en caliente: %v", r)
	}
	time.Sleep(1500 * time.Millisecond) // vence el latido de 1 s
	fyne.DoAndWait(func() {})
	if n := latidos.Load() - antes; n != 0 {
		t.Fatalf("con la app al fondo se han publicado %d latidos «En línea»", n)
	}
	if publicado(t, "eu.sytue.docus") != 7 {
		t.Fatal("el refresco en caliente no ha publicado el widget")
	}

	s.ProbarPrimerPlano(true)
	limite := time.Now().Add(500 * time.Millisecond) // antes que el latido de 1 s
	for latidos.Load() == antes {
		if time.Now().After(limite) {
			t.Fatal("al volver al primer plano no se late en el acto")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestFueraDeIOSElFondoNoParaLaPresencia: en el escritorio «fondo» es perder el foco, y
// se sigue en línea; sin PausaEnFondo, latir no se para.
func TestFueraDeIOSElFondoNoParaLaPresencia(t *testing.T) {
	aparte(t)
	app := test.NewApp()
	defer app.Quit()
	s := Abrir(widgetApp{app}, "sytue.eu")
	if s.PausaEnFondo() {
		t.Fatal("fuera de iOS la presencia se para al perder el foco")
	}
	s.ProbarPrimerPlano(false)
	if s.latidoEnPausa() {
		t.Fatal("fuera de iOS, perder el foco pausa el latido")
	}
}
