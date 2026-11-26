// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// servicioFiel es Sytue Calendar como contesta DE VERDAD (services/calendar): en
// /instances, el evento de cada ocurrencia de una serie lleva en `dtstart` y `dtend` los
// de ESA ocurrencia (`domain.ExpandEvents` copia el maestro y le pone su inicio), y el
// maestro entero, con su DTSTART, solo se lee en /events/{id}. Apunta las escrituras con
// su cuerpo.
type servicioFiel struct {
	mu         sync.Mutex
	maestros   map[string]api.Event
	instancias []api.Instance
	hechas     []string
	cuerpos    []map[string]any
}

func (s *servicioFiel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ruta := r.Method + " " + r.URL.Path
	if r.Method == http.MethodGet {
		switch {
		case ruta == "GET /api/v1/calendars":
			_, _ = io.WriteString(w, `{"calendars":[{"id":"mio","display_name":"Personal","color":"#1e50a8","is_default":true}]}`)
		case ruta == "GET /api/v1/calendars/shared-with-me":
			_, _ = io.WriteString(w, `{"calendars":[]}`)
		case ruta == "GET /api/v1/calendars/mio/instances":
			_ = json.NewEncoder(w).Encode(map[string]any{"instances": s.instancias})
		case strings.HasPrefix(r.URL.Path, "/api/v1/events/"):
			ev, ok := s.maestros[strings.TrimPrefix(r.URL.Path, "/api/v1/events/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(ev)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
		return
	}
	var c map[string]any
	if datos, _ := io.ReadAll(r.Body); len(datos) > 0 {
		_ = json.Unmarshal(datos, &c)
	}
	s.hechas, s.cuerpos = append(s.hechas, ruta), append(s.cuerpos, c)
	_, _ = io.WriteString(w, `{}`)
}

func (s *servicioFiel) apuntadas() ([]string, []map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.hechas...), append([]map[string]any(nil), s.cuerpos...)
}

// serieFiel es la serie semanal `ev` con sus ocurrencias `semanas` como las expande el
// servicio: el mismo evento con el inicio y el fin de cada una.
func serieFiel(ev api.Event, semanas ...int) []api.Instance {
	loc, err := time.LoadLocation(ev.TZID)
	if err != nil {
		loc = time.UTC
	}
	dur := ev.DTEnd.Sub(ev.DTStart)
	out := make([]api.Instance, 0, len(semanas))
	for _, n := range semanas {
		i := ev
		i.DTStart = ev.DTStart.In(loc).AddDate(0, 0, 7*n)
		i.DTEnd = i.DTStart.Add(dur)
		out = append(out, api.Instance{Event: i, Start: i.DTStart, End: i.DTEnd})
	}
	return out
}

// appEnZona monta la App sobre `s` pintando en `zona` (nil, la del sistema), en una
// ventana de 1280 × 744 y con las cargas en línea.
func appEnZona(t *testing.T, s http.Handler, zona *time.Location) (*App, fyne.Window) {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	p := &Pantalla{T: traductor(t), Cursor: time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC), Zona: zona}
	a := NuevaApp(p, api.NewCalendars(srv.URL, nil, nil))
	a.lanzar = func(f func(context.Context)) bool { f(a.vida); return true }
	a.enUI = func(f func()) { f() }
	w := test.NewWindow(a.Objeto())
	t.Cleanup(w.Close)
	t.Cleanup(func() {
		for _, aviso := range a.avisos.Avisos() {
			a.avisos.Descartar(aviso)
		}
	})
	w.SetPadded(false)
	w.Resize(fyne.NewSize(ventana, altoUtil))
	a.Arrancar()
	return a, w
}

// ocurrenciaQueEmpieza es la ocurrencia del evento `id` que empieza en `inicio`.
func ocurrenciaQueEmpieza(t *testing.T, a *App, id string, inicio time.Time) Ocurrencia {
	t.Helper()
	for _, o := range a.P.Eventos {
		if o.Event.ID == id && o.Start.Equal(inicio) {
			return o
		}
	}
	t.Fatalf("no está la ocurrencia de %s del %v entre %d", id, inicio, len(a.P.Eventos))
	return Ocurrencia{}
}

func cargarZona(t *testing.T, nombre string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(nombre)
	if err != nil {
		t.Skipf("sin tzdata: %v", err)
	}
	return loc
}

// instanteDe lee un instante RFC 3339 de un cuerpo.
func instanteDe(t *testing.T, c map[string]any, campo string) time.Time {
	t.Helper()
	s, _ := c[campo].(string)
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("%s no es un instante: %v (%v)", campo, c[campo], err)
	}
	return v
}

// TestAbrirYGuardarNoMueveLaHora: abrir un evento en el formulario y guardarlo sin tocar
// nada manda la MISMA hora real, se pinte en la zona que se pinte —la del sistema o otra,
// igual o distinta de la del evento—, y el campo enseña la hora del evento en SU zona, que
// es la que dice «Zona horaria». Antes, un evento de las 10:00 de Madrid abierto en UTC
// se guardaba a las 09:00 de Madrid (la hora de pared de UTC leída en la zona del
// evento). El de día completo no tiene zona y sigue en su día.
func TestAbrirYGuardarNoMueveLaHora(t *testing.T) {
	madrid, nuevaYork := cargarZona(t, "Europe/Madrid"), cargarZona(t, "America/New_York")
	kolkata := cargarZona(t, "Asia/Kolkata")
	eventos := []struct {
		nombre string
		ev     api.Event
		campo  string // lo que dice el campo «Inicio»
	}{
		{"10:00 en Madrid", api.Event{TZID: "Europe/Madrid", DTStart: time.Date(2026, 11, 10, 10, 0, 0, 0, madrid)}, "10/11/2026, 10:00"},
		{"10:00 en Nueva York", api.Event{TZID: "America/New_York", DTStart: time.Date(2026, 11, 10, 10, 0, 0, 0, nuevaYork)}, "10/11/2026, 10:00"},
		{"23:30 en Kolkata", api.Event{TZID: "Asia/Kolkata", DTStart: time.Date(2026, 11, 10, 23, 30, 0, 0, kolkata)}, "10/11/2026, 23:30"},
		// Una hora que no existe en Nueva York (la que se salta al pasar al horario de
		// verano, que en Madrid aún no ha llegado): los campos no la pueden decir allí.
		{"02:30 en Madrid el día que Nueva York se la salta", api.Event{TZID: "Europe/Madrid", DTStart: time.Date(2027, 3, 14, 2, 30, 0, 0, madrid)}, ""},
		// La SEGUNDA 01:30 de Nueva York, la de la vuelta al horario de invierno: en los
		// campos es la misma hora de pared que la primera.
		{"la segunda 01:30 en Nueva York", api.Event{TZID: "America/New_York", DTStart: time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC)}, "01/11/2026, 01:30"},
		{"sin zona (UTC)", api.Event{DTStart: time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)}, ""},
		{"día completo", api.Event{AllDay: true, DTStart: time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)}, "10/11/2026"},
	}
	zonas := []struct {
		nombre string
		loc    *time.Location
	}{{"UTC", time.UTC}, {"Nueva York", nuevaYork}, {"Madrid", madrid}, {"Kolkata", kolkata}, {"la del sistema", nil}}
	for _, z := range zonas {
		for _, c := range eventos {
			ev := c.ev
			ev.ID, ev.UID, ev.Summary, ev.CalendarID = "e1", "u1", "Comité", "mio"
			if ev.AllDay {
				ev.DTEnd = ev.DTStart.AddDate(0, 0, 1)
			} else {
				ev.DTEnd = ev.DTStart.Add(90 * time.Minute)
			}
			s := &servicioFiel{
				maestros:   map[string]api.Event{"e1": ev},
				instancias: []api.Instance{{Event: ev, Start: ev.DTStart, End: ev.DTEnd}},
			}
			a, w := appEnZona(t, s, z.loc)
			e := a.P.EdicionDe(ocurrenciaDe(t, a, "e1"))
			f := a.P.nuevoFormulario(e, a.destinos(), a.accionesFormulario(e))
			f.d.Mostrar(w.Canvas())
			if c.campo != "" && f.inicio.Texto() != c.campo {
				t.Errorf("en %s, %s: el campo dice %q y la hora del evento es %q", z.nombre, c.nombre, f.inicio.Texto(), c.campo)
			}
			f.enviar()
			hechas, cuerpos := s.apuntadas()
			if len(hechas) != 1 || hechas[0] != "PUT /api/v1/events/e1" {
				t.Fatalf("en %s, %s: guardar hizo %v", z.nombre, c.nombre, hechas)
			}
			cu := cuerpos[0]
			ini, fin := instanteDe(t, cu, "start"), instanteDe(t, cu, "end")
			if !ini.Equal(ev.DTStart) || !fin.Equal(ev.DTEnd) {
				t.Errorf("en %s, %s: se guarda %v – %v y era %v – %v", z.nombre, c.nombre,
					ini.UTC(), fin.UTC(), ev.DTStart.UTC(), ev.DTEnd.UTC())
			}
			if tz, _ := cu["tzid"].(string); tz != ev.TZID {
				t.Errorf("en %s, %s: la zona pasa a %q", z.nombre, c.nombre, tz)
			}
			if cu["all_day"] != ev.AllDay {
				t.Errorf("en %s, %s: all_day %v", z.nombre, c.nombre, cu["all_day"])
			}
		}
	}
}

// TestGuardarLaSerieDesdeOtraOcurrenciaNoLaAncla: contra el servicio como contesta de
// verdad (el `dtstart` de cada ocurrencia es el suyo), guardar «todos» desde la quinta
// ocurrencia —pasado el cambio de hora— conserva el DTSTART del maestro y le aplica lo
// que se movió la ocurrencia; «este y los siguientes» deja a la serie nueva las que le
// quedan del COUNT. Antes la serie se anclaba en la quinta (se perdían las cuatro
// primeras) y la serie nueva salía con el COUNT entero. Pintando en Madrid y en UTC.
func TestGuardarLaSerieDesdeOtraOcurrenciaNoLaAncla(t *testing.T) {
	madrid := cargarZona(t, "Europe/Madrid")
	maestro := api.Event{
		ID: "s1", UID: "us", CalendarID: "mio", Summary: "Seguimiento", TZID: "Europe/Madrid",
		RRule: "FREQ=WEEKLY;COUNT=10", DTStart: time.Date(2026, 10, 6, 10, 0, 0, 0, madrid),
		DTEnd: time.Date(2026, 10, 6, 11, 0, 0, 0, madrid),
	}
	quinta := time.Date(2026, 11, 3, 10, 0, 0, 0, madrid) // ya en horario de invierno
	for _, zona := range []*time.Location{madrid, time.UTC} {
		abrir := func() (*servicioFiel, *formulario, fyne.Window) {
			s := &servicioFiel{maestros: map[string]api.Event{"s1": maestro}, instancias: serieFiel(maestro, 3, 4, 5)}
			a, w := appEnZona(t, s, zona)
			e := a.P.EdicionDe(ocurrenciaQueEmpieza(t, a, "s1", quinta))
			f := a.P.nuevoFormulario(e, a.destinos(), a.accionesFormulario(e))
			f.d.Mostrar(w.Canvas())
			return s, f, w
		}

		// «Todos» sin tocar nada: el maestro, como estaba.
		s, f, w := abrir()
		f.enviar()
		confirmarAlcance(t, w, "Todos los eventos")
		hechas, cuerpos := s.apuntadas()
		if len(hechas) != 1 || hechas[0] != "PUT /api/v1/events/s1" {
			t.Fatalf("en %v, «todos» hizo %v", zona, hechas)
		}
		if ini, fin := instanteDe(t, cuerpos[0], "start"), instanteDe(t, cuerpos[0], "end"); !ini.Equal(maestro.DTStart) || !fin.Equal(maestro.DTEnd) ||
			cuerpos[0]["rrule"] != "FREQ=WEEKLY;COUNT=10" || cuerpos[0]["tzid"] != "Europe/Madrid" {
			t.Errorf("en %v, «todos» sin cambios manda %v – %v (%v) y el maestro es %v – %v", zona, ini, fin, cuerpos[0], maestro.DTStart, maestro.DTEnd)
		}

		// «Todos» con la quinta una hora más tarde: la serie entera a las 11:00 de Madrid,
		// desde su primera.
		s, f, w = abrir()
		f.inicio.PonerTexto("03/11/2026, 11:00")
		f.fin.PonerTexto("03/11/2026, 12:00")
		f.enviar()
		confirmarAlcance(t, w, "Todos los eventos")
		_, cuerpos = s.apuntadas()
		if len(cuerpos) != 1 {
			t.Fatalf("en %v, «todos» movida hizo %d escrituras", zona, len(cuerpos))
		}
		if ini, fin := instanteDe(t, cuerpos[0], "start"), instanteDe(t, cuerpos[0], "end"); !ini.Equal(maestro.DTStart.Add(time.Hour)) || !fin.Equal(maestro.DTEnd.Add(time.Hour)) {
			t.Errorf("en %v, «todos» una hora más tarde manda %v – %v", zona, ini.In(madrid), fin.In(madrid))
		}

		// «Este y los siguientes»: la serie nueva desde la quinta, con las 6 que quedan.
		s, f, w = abrir()
		f.titulo.PonerTexto("Seguimiento (segunda etapa)")
		f.enviar()
		confirmarAlcance(t, w, "Este y los siguientes")
		hechas, cuerpos = s.apuntadas()
		if len(hechas) != 2 || hechas[0] != "POST /api/v1/calendars/mio/events" || hechas[1] != "POST /api/v1/calendars/mio/amend-recurrence" {
			t.Fatalf("en %v, «siguientes» hizo %v", zona, hechas)
		}
		if ini := instanteDe(t, cuerpos[0], "start"); !ini.Equal(quinta) || cuerpos[0]["rrule"] != "FREQ=WEEKLY;COUNT=6" {
			t.Errorf("en %v, la serie nueva empieza %v con %v (y son la quinta y COUNT=6)", zona, ini.In(madrid), cuerpos[0]["rrule"])
		}
		if rid := instanteDe(t, cuerpos[1], "recurrence_id"); !rid.Equal(quinta) || cuerpos[1]["mode"] != "until" {
			t.Errorf("en %v, el recorte: %v", zona, cuerpos[1])
		}
	}
}

// TestTodosDesplazaElMaestroEnHoraDePared: lo que se movió la ocurrencia se aplica al
// DTSTART del maestro en días de calendario y hora de pared, no en horas absolutas: un
// maestro del sábado de antes del cambio de hora, movido un día desde una ocurrencia de
// noviembre, pasa al domingo a la MISMA hora (en horas absolutas caía una antes); cambiar
// solo la zona conserva la hora de pared; y pasar a día completo, o volver a tener hora,
// se queda en la fecha del maestro.
func TestTodosDesplazaElMaestroEnHoraDePared(t *testing.T) {
	madrid, nuevaYork := cargarZona(t, "Europe/Madrid"), cargarZona(t, "America/New_York")
	sabado := time.Date(2026, 10, 24, 10, 0, 0, 0, madrid) // al día siguiente acaba el horario de verano
	maestro := api.Event{TZID: "Europe/Madrid", DTStart: sabado, DTEnd: sabado.Add(time.Hour), RRule: "FREQ=WEEKLY"}
	oc := Ocurrencia{Instance: api.Instance{Event: maestro, Start: time.Date(2026, 11, 7, 10, 0, 0, 0, madrid)}}
	dia := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	for _, c := range []struct {
		nombre      string
		maestro     api.Event
		o           Ocurrencia
		editado     api.EventInput
		inicio, fin time.Time
	}{
		{
			"sin cambios", maestro, oc,
			api.EventInput{TZID: "Europe/Madrid", Start: oc.Start, End: oc.Start.Add(time.Hour)},
			sabado, sabado.Add(time.Hour),
		},
		{
			"un día más tarde, al otro lado del cambio de hora", maestro, oc,
			api.EventInput{TZID: "Europe/Madrid", Start: oc.Start.AddDate(0, 0, 1), End: oc.Start.AddDate(0, 0, 1).Add(time.Hour)},
			time.Date(2026, 10, 25, 10, 0, 0, 0, madrid), time.Date(2026, 10, 25, 11, 0, 0, 0, madrid),
		},
		{
			"a las 10:00 de Nueva York", maestro, oc,
			api.EventInput{TZID: "America/New_York", Start: time.Date(2026, 11, 7, 10, 0, 0, 0, nuevaYork), End: time.Date(2026, 11, 7, 11, 30, 0, 0, nuevaYork)},
			time.Date(2026, 10, 24, 10, 0, 0, 0, nuevaYork), time.Date(2026, 10, 24, 11, 30, 0, 0, nuevaYork),
		},
		{
			"a día completo", maestro, oc,
			api.EventInput{AllDay: true, Start: dia(2026, 11, 7), End: dia(2026, 11, 8)},
			dia(2026, 10, 24), dia(2026, 10, 25),
		},
		{
			"de día completo a las 09:30",
			api.Event{AllDay: true, DTStart: dia(2026, 10, 24), DTEnd: dia(2026, 10, 25), RRule: "FREQ=WEEKLY"},
			Ocurrencia{Instance: api.Instance{Event: api.Event{AllDay: true}, Start: dia(2026, 11, 7)}},
			api.EventInput{TZID: "Europe/Madrid", Start: time.Date(2026, 11, 7, 9, 30, 0, 0, madrid), End: time.Date(2026, 11, 7, 10, 0, 0, 0, madrid)},
			time.Date(2026, 10, 24, 9, 30, 0, 0, madrid), time.Date(2026, 10, 24, 10, 0, 0, 0, madrid),
		},
	} {
		ini, fin := desplazarMaestro(c.maestro, c.o, c.editado)
		if !ini.Equal(c.inicio) || !fin.Equal(c.fin) {
			t.Errorf("%s: el maestro queda %v – %v y es %v – %v", c.nombre, ini, fin, c.inicio, c.fin)
		}
	}
}

// TestElHuecoYLaReunionVanALaHoraDelEvento: con un evento de Nueva York abierto en
// Madrid, los campos dicen la hora de Nueva York; elegir un hueco —cuyo botón también la
// dice (gruposHuecos)— pone esa hora en el campo y guarda ESE instante, y la reunión que
// se crea desde el formulario va a la hora real del evento, no a la de pared de Madrid.
func TestElHuecoYLaReunionVanALaHoraDelEvento(t *testing.T) {
	madrid, nuevaYork := cargarZona(t, "Europe/Madrid"), cargarZona(t, "America/New_York")
	ev := api.Event{
		ID: "e1", UID: "u1", CalendarID: "mio", Summary: "Comité", TZID: "America/New_York",
		DTStart: time.Date(2026, 11, 10, 10, 0, 0, 0, nuevaYork), DTEnd: time.Date(2026, 11, 10, 11, 0, 0, 0, nuevaYork),
	}
	s := &servicioFiel{maestros: map[string]api.Event{"e1": ev}, instancias: []api.Instance{{Event: ev, Start: ev.DTStart, End: ev.DTEnd}}}
	a, w := appEnZona(t, s, madrid)
	a.P.SinPersonas, a.P.PistaEnlace = true, "Pega el enlace"
	var reunion []time.Time
	a.P.Reunion = &Reunion{
		Boton: func(string) string { return "Crear reunión de Microsoft Teams" },
		Crear: func(_ context.Context, _, _ string, inicio, fin time.Time) (string, error) {
			reunion = []time.Time{inicio, fin}
			return "https://teams.microsoft.com/l/meetup-join/abc", nil
		},
	}
	e := a.P.EdicionDe(ocurrenciaDe(t, a, "e1"))
	f := a.P.nuevoFormulario(e, a.destinos(), a.accionesFormulario(e))
	f.d.Mostrar(w.Canvas())

	f.crearReunion()
	if len(reunion) != 2 || !reunion[0].Equal(ev.DTStart) || !reunion[1].Equal(ev.DTEnd) {
		t.Fatalf("la reunión se crea para %v y el evento es %v – %v", reunion, ev.DTStart, ev.DTEnd)
	}

	hueco := time.Date(2026, 11, 12, 14, 0, 0, 0, nuevaYork)
	f.elegirHueco(hueco.In(nuevaYork), 60)
	if got := f.inicio.Texto(); got != "12/11/2026, 14:00" {
		t.Errorf("el hueco de las 14:00 de Nueva York deja en el campo %q", got)
	}
	f.enviar()
	_, cuerpos := s.apuntadas()
	if len(cuerpos) != 1 {
		t.Fatalf("guardar hizo %d escrituras", len(cuerpos))
	}
	if ini, fin := instanteDe(t, cuerpos[0], "start"), instanteDe(t, cuerpos[0], "end"); !ini.Equal(hueco) || !fin.Equal(hueco.Add(time.Hour)) {
		t.Errorf("el hueco elegido se guarda %v – %v y es %v", ini.In(nuevaYork), fin.In(nuevaYork), hueco)
	}
}

// TestUnaSerieSinZonaSeQuedaSinZona: una serie con las horas en UTC y sin TZID (la de un
// .ics importado), abierta en una ocurrencia de invierno con la pantalla en Madrid y
// guardada «todos» sin tocar nada, se queda como estaba: sin zona y con su inicio de
// verano. Ponerle la zona de visualización, que es lo que enseña el formulario, la
// anclaba a las 10:00 de Madrid y movía una hora las repeticiones del otro lado del
// cambio de hora.
func TestUnaSerieSinZonaSeQuedaSinZona(t *testing.T) {
	madrid := cargarZona(t, "Europe/Madrid")
	maestro := api.Event{
		ID: "s1", UID: "us", CalendarID: "mio", Summary: "Guardia", RRule: "FREQ=WEEKLY",
		DTStart: time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC), DTEnd: time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC),
	}
	s := &servicioFiel{maestros: map[string]api.Event{"s1": maestro}, instancias: serieFiel(maestro, 4)}
	a, w := appEnZona(t, s, madrid)
	e := a.P.EdicionDe(ocurrenciaQueEmpieza(t, a, "s1", time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)))
	f := a.P.nuevoFormulario(e, a.destinos(), a.accionesFormulario(e))
	f.d.Mostrar(w.Canvas())
	if got := f.inicio.Texto(); got != "03/11/2026, 10:00" {
		t.Errorf("el campo dice %q: es la hora de Madrid, la que se ve en la rejilla", got)
	}
	f.enviar()
	confirmarAlcance(t, w, "Todos los eventos")
	_, cuerpos := s.apuntadas()
	if len(cuerpos) != 1 {
		t.Fatalf("«todos» hizo %d escrituras", len(cuerpos))
	}
	if ini, fin := instanteDe(t, cuerpos[0], "start"), instanteDe(t, cuerpos[0], "end"); !ini.Equal(maestro.DTStart) || !fin.Equal(maestro.DTEnd) || cuerpos[0]["tzid"] != nil {
		t.Errorf("la serie se guarda %v – %v en %v y era %v – %v sin zona", ini, fin, cuerpos[0]["tzid"], maestro.DTStart, maestro.DTEnd)
	}
}
