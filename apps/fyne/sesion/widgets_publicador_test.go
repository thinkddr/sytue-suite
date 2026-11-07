// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets/fuentes"
)

// appDeWidget es una app con identificador y esquema, que es lo que el widget necesita.
type appDeWidget struct {
	fyne.App
	id, esquema string
}

func (a appDeWidget) Metadata() fyne.AppMetadata {
	return fyne.AppMetadata{ID: a.id, Custom: map[string]string{"esquema": a.esquema}}
}

// guardar es lo que hace auth al guardar la sesión de `sujeto` (vacío: cerrarla): la
// marca y la época, bajo el cerrojo de la sesión.
func guardar(t *testing.T, sujeto, refresh string) {
	t.Helper()
	soltar, err := ajustes.CerrojoSesion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()
	if err := auth.Save(auth.Tokens{Access: "a-" + refresh, Refresh: refresh, Subject: sujeto, Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
}

// suiteConSesion es una suite que ya ha adoptado la sesión de `sujeto`, guardada como
// la guarda auth.
func suiteConSesion(t *testing.T, a fyne.App, sujeto string) *Suite {
	t.Helper()
	guardar(t, sujeto, "r1")
	s := &Suite{app: a, Sesion: auth.NewSession(auth.Tokens{}, nil, nil)}
	guardados, marcas, err := s.leerSesion(context.Background())
	if err != nil || guardados.Subject != sujeto {
		t.Fatalf("la sesión guardada no se deja leer: %+v %v", guardados, err)
	}
	s.Sesion.Sincronizar(guardados)
	s.generacionSesion, s.epocaSesion = marcas.generacion, marcas.epoca
	s.visto = ajustes.Leer()
	s.visto.Sesion, s.visto.Epoca = marcas.generacion, marcas.epoca
	return s
}

func conDocumentos(n int64) widgets.Detail {
	return widgets.Detail{Counters: []widgets.Counter{{Kind: "documents", Value: n}}}
}

func publicado(t *testing.T, bundle string) int64 {
	t.Helper()
	s, err := widgets.Read(bundle)
	if err != nil || len(s.Counters) == 0 {
		return -1
	}
	return s.Counters[0].Value
}

// TestRenovarDuranteLaRondaNoPierdeLaPublicacion es el caso del dueño (§0.2 del
// diseño): con la app abierta más de 15 min, la propia petición que lee renueva el
// token y cambia la marca. Antes, la captura quedaba vieja y PublishDetail devolvía
// ErrPrivate, que nadie miraba: el evento nuevo no llegaba al widget. Ahora la captura
// es de la época, que no cambia al renovar, y lo publicado lleva la marca NUEVA, que
// es la que miran los lectores de hoy.
func TestRenovarDuranteLaRondaNoPierdeLaPublicacion(t *testing.T) {
	t.Cleanup(widgets.SimularLector(true))
	aparte(t)
	app := test.NewApp()
	defer app.Quit()
	s := suiteConSesion(t, widgetApp{app}, "ana")
	reloj := widgets.NuevoRelojDePrueba(time.Now())
	s.ProbarWidgetsCon(reloj.Opciones(widgets.OpcionesPublicador{}))
	pub := s.PublicadorWidget(func(context.Context) (widgets.Detail, error) {
		guardar(t, "ana", "r2") // la renovación que hace la propia lectura
		return conDocumentos(5), nil
	})
	pub.Avisar()
	reloj.Avanzar(2 * time.Second)
	if n := publicado(t, "eu.sytue.docus"); n != 5 {
		t.Fatalf("tras renovar a mitad de la ronda, el lector no ve lo publicado (%d)", n)
	}
}

// banco es una suite con su publicador, un reloj de prueba y una Fuente que cuenta.
type bancoSuite struct {
	t        *testing.T
	s        *Suite
	reloj    *widgets.RelojDePrueba
	lecturas int
	mu       sync.Mutex
}

func nuevoBancoSuite(t *testing.T, a fyne.App, f widgets.Fuente) *bancoSuite {
	t.Helper()
	t.Cleanup(widgets.SimularLector(true))
	b := &bancoSuite{t: t, s: suiteConSesion(t, a, "ana"), reloj: widgets.NuevoRelojDePrueba(time.Now())}
	b.s.ProbarWidgetsCon(b.reloj.Opciones(widgets.OpcionesPublicador{}))
	if f == nil {
		f = func(context.Context) (widgets.Detail, error) { return conDocumentos(1), nil }
	}
	b.s.PublicadorWidget(func(ctx context.Context) (widgets.Detail, error) {
		b.mu.Lock()
		b.lecturas++
		b.mu.Unlock()
		return f(ctx)
	}, "calendar")
	b.s.iniciarWidgets(t.Context())
	b.avanzar()
	return b
}

// avanzar deja correr lo programado y lo que la suite mandó al hilo de la interfaz.
func (b *bancoSuite) avanzar() {
	fyne.DoAndWait(func() {})
	b.reloj.Avanzar(10 * time.Second)
}

func (b *bancoSuite) leidas() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := b.lecturas
	b.lecturas = 0
	return n
}

// TestLosDisparadoresComunesPublican: entrar, volver a primer plano, un aviso del Hub
// del producto (y de los que el Hub sigue), quitar el bloqueo y entrar con otra persona
// vuelven a publicar; poner el bloqueo y salir pausan y borran. Pausar no es terminal.
func TestLosDisparadoresComunesPublican(t *testing.T) {
	aparte(t)
	app := test.NewApp()
	defer app.Quit()
	b := nuevoBancoSuite(t, widgetApp{app}, nil)
	if n := b.leidas(); n != 1 {
		t.Fatalf("al arrancar con sesión no se publica: %d lecturas", n)
	}
	b.s.repartir(true) // vuelve a primer plano
	b.avanzar()
	if n := b.leidas(); n != 1 {
		t.Fatalf("volver a primer plano: %d lecturas", n)
	}
	b.s.AvisoDelHub("docs")
	b.s.AvisoDelHub("calendar")
	b.avanzar()
	if n := b.leidas(); n != 1 {
		t.Fatalf("avisos del Hub de su producto: %d lecturas; quiero 1", n)
	}
	b.s.AvisoDelHub("rooms")
	b.avanzar()
	if n := b.leidas(); n != 0 {
		t.Fatalf("un aviso de otro producto ha publicado: %d lecturas", n)
	}
	if err := ajustes.Cambiar(func(a *ajustes.Ajustes) { a.Bloqueo = true }); err != nil {
		t.Fatal(err)
	}
	b.s.aplicar(ajustes.Leer())
	b.avanzar()
	b.s.AvisarWidget()
	b.avanzar()
	if n := b.leidas(); n != 0 {
		t.Fatalf("con el bloqueo puesto se ha leído %d veces", n)
	}
	esperarBorrado(t, "eu.sytue.docus")
	if err := ajustes.Cambiar(func(a *ajustes.Ajustes) { a.Bloqueo = false }); err != nil {
		t.Fatal(err)
	}
	b.s.aplicar(ajustes.Leer())
	b.avanzar()
	if n := b.leidas(); n != 1 || publicado(t, "eu.sytue.docus") != 1 {
		t.Fatalf("al quitar el bloqueo no se vuelve a publicar: %d lecturas", n)
	}
	guardar(t, "", "")
	b.s.aplicar(ajustes.Leer())
	b.avanzar()
	esperarBorrado(t, "eu.sytue.docus")
	guardar(t, "bea", "r9")
	b.s.aplicar(ajustes.Leer())
	b.avanzar()
	if n := b.leidas(); n != 1 || publicado(t, "eu.sytue.docus") != 1 {
		t.Fatalf("entrar con otra persona tras salir no publica: %d lecturas", n)
	}
}

// TestElEcoDelProductoPropioRespetaElMinimo: la señal `datos` (o un aviso del Hub) del
// PROPIO producto que llega justo tras una escritura propia es su eco y va por
// AvisarDelServidor, con el mínimo de las escrituras: el autoguardado (una escritura
// cada 3 s, con su eco un segundo después) sigue haciendo pocas rondas por minuto.
// VISTO EN ROJO con AvisoDelHub llamando a Avisar para el producto propio: 20 rondas.
func TestElEcoDelProductoPropioRespetaElMinimo(t *testing.T) {
	aparte(t)
	app := test.NewApp()
	defer app.Quit()
	b := nuevoBancoSuite(t, widgetApp{app}, nil)
	b.leidas()
	for range 20 {
		b.s.avisarEscrituraWidget()
		b.reloj.Avanzar(time.Second)
		b.s.AvisoDelHub("docs")
		b.reloj.Avanzar(2 * time.Second)
	}
	if n := b.leidas(); n == 0 || n > 3 {
		t.Fatalf("20 escrituras con su eco en 60 s hicieron %d rondas; quiero entre 1 y 3", n)
	}
}

// TestUnaRenovacionRepublicaSinLeer: otra app renueva (otra marca, la misma persona);
// esta reescribe lo último con la marca nueva sin volver a pedirlo.
func TestUnaRenovacionRepublicaSinLeer(t *testing.T) {
	aparte(t)
	app := test.NewApp()
	defer app.Quit()
	b := nuevoBancoSuite(t, widgetApp{app}, nil)
	b.leidas()
	guardar(t, "ana", "r2")
	b.s.aplicar(ajustes.Leer())
	b.avanzar()
	if n := b.leidas(); n != 0 || publicado(t, "eu.sytue.docus") != 1 {
		t.Fatalf("una renovación ha leído %d veces o el lector no ve lo publicado", n)
	}
}

func esperarBorrado(t *testing.T, bundle string) {
	t.Helper()
	for range 200 {
		if _, err := widgets.Read(bundle); errors.Is(err, os.ErrNotExist) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("el snapshot sigue ahí")
}

// calendarioFalso es el servicio de Calendar con un calendario y lo que se cree en él.
type calendarioFalso struct {
	mu      sync.Mutex
	eventos []api.Instance
}

func (c *calendarioFalso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	ruta := strings.TrimPrefix(r.URL.Path, "/api/v1")
	switch {
	case ruta == "/calendars":
		_ = json.NewEncoder(w).Encode(map[string]any{"calendars": []api.Calendar{{ID: "cal", Color: "#16734d"}}})
	case ruta == "/calendars/shared-with-me":
		_ = json.NewEncoder(w).Encode(map[string]any{"calendars": []api.Calendar{}})
	case ruta == "/calendars/cal/instances":
		_ = json.NewEncoder(w).Encode(map[string]any{"instances": c.eventos})
	case ruta == "/calendars/cal/events" && r.Method == http.MethodPost:
		var in api.EventInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		ev := api.Event{ID: "nuevo", Summary: in.Summary, Status: "CONFIRMED"}
		c.eventos = append(c.eventos, api.Instance{Event: ev, Start: in.Start, End: in.End})
		_ = json.NewEncoder(w).Encode(ev)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// TestCrearUnEventoDeHoyLlegaAlWidget es el caso literal del dueño: con Calendar
// abierta (mirando lo que sea), crear un evento para hoy lo publica en el widget, sin
// que la pantalla tenga que recargar nada. Y veinte escrituras seguidas no dan veinte
// rondas: como mucho tres por minuto.
func TestCrearUnEventoDeHoyLlegaAlWidget(t *testing.T) {
	aparte(t)
	app := test.NewApp()
	defer app.Quit()
	servicio := &calendarioFalso{}
	srv := httptest.NewServer(servicio)
	defer srv.Close()
	a := appDeWidget{App: app, id: "eu.sytue.calendar", esquema: "sytue-calendar"}
	b := nuevoBancoSuite(t, a, fuentes.Calendar(fuentes.Comunes{}, api.NewCalendars(srv.URL, nil, nil)))
	b.leidas()
	// El cliente de la pantalla, envuelto como lo envuelve cmd/calendar.
	pantalla := api.NewCalendars(srv.URL, nil, b.s.ClienteConAviso(nil))
	inicio := time.Now().Add(time.Hour)
	if _, err := pantalla.CreateEvent(context.Background(), "cal", api.EventInput{Summary: "Reunión de hoy", Start: inicio, End: inicio.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	b.avanzar()
	snap, err := widgets.Read("eu.sytue.calendar")
	if err != nil || len(snap.Items) != 1 || snap.Items[0].Title != "Reunión de hoy" {
		t.Fatalf("el evento recién creado no está en el widget: %+v %v", snap.Items, err)
	}
	b.leidas()
	for i := range 20 {
		if _, err := pantalla.CreateEvent(context.Background(), "cal", api.EventInput{Summary: "Otra " + string(rune('a'+i)), Start: inicio, End: inicio.Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
		b.reloj.Avanzar(3 * time.Second)
	}
	if n := b.leidas(); n > 3 {
		t.Fatalf("20 escrituras en 60 s hicieron %d rondas", n)
	}
}

// TestSalirAlFondoPublicaLaSegundaEscritura es el caso del revisor: creo un evento de
// hoy, a los 10 s creo otro (dentro de la ventana de 30 s entre rondas por escrituras)
// y salgo a la pantalla de inicio. iOS suspende la app segundos después, así que lo
// aplazado tiene que publicarse al pasar al fondo, sin esperar a que venza la ventana:
// aquí el reloj del Publicador no se mueve.
func TestSalirAlFondoPublicaLaSegundaEscritura(t *testing.T) {
	aparte(t)
	app := test.NewApp()
	defer app.Quit()
	servicio := &calendarioFalso{}
	srv := httptest.NewServer(servicio)
	defer srv.Close()
	a := appDeWidget{App: app, id: "eu.sytue.calendar", esquema: "sytue-calendar"}
	b := nuevoBancoSuite(t, a, fuentes.Calendar(fuentes.Comunes{}, api.NewCalendars(srv.URL, nil, nil)))
	pantalla := api.NewCalendars(srv.URL, nil, b.s.ClienteConAviso(nil))
	inicio := time.Now().Add(time.Hour)
	crear := func(titulo string) {
		t.Helper()
		if _, err := pantalla.CreateEvent(context.Background(), "cal", api.EventInput{Summary: titulo, Start: inicio, End: inicio.Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	filas := func() int {
		snap, err := widgets.Read("eu.sytue.calendar")
		if err != nil {
			return -1
		}
		return len(snap.Items)
	}
	crear("Reunión")
	b.avanzar() // 10 s: la primera escritura ya está publicada
	if n := filas(); n != 1 {
		t.Fatalf("la primera escritura no llegó: %d filas", n)
	}
	crear("Reunión, otra sala")
	b.s.repartir(false) // a la pantalla de inicio
	for range 400 {
		if filas() == 2 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("al pasar al fondo, la segunda escritura sigue sin publicar (%d filas)", filas())
}
