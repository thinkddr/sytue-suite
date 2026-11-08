// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// TestLasFuentesNoImportanFyne: la misma Fuente corre en un despertar sin ventana
// (piezas 4 y 6 del ADR 0078 enmendado). Si un día importa la interfaz, no podrá.
func TestLasFuentesNoImportanFyne(t *testing.T) {
	salida, err := exec.Command("go", "list", "-tags", "goolm", "-deps", ".").CombinedOutput()
	if err != nil {
		t.Skipf("sin `go list` aquí: %v %s", err, salida)
	}
	for _, dep := range strings.Fields(string(salida)) {
		if strings.HasPrefix(dep, "fyne.io/") || strings.Contains(dep, "/apps/fyne/") {
			t.Fatalf("widgets/fuentes depende de la interfaz: %s", dep)
		}
	}
}

// servidor contesta `rutas` (ruta exacta → JSON); el resto, 503.
func servidor(t *testing.T, rutas map[string]any) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Los constructores de `api` añaden el prefijo de versión a la base.
		cuerpo, ok := rutas[strings.TrimPrefix(r.URL.Path, "/api/v1")]
		if !ok {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cuerpo)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestTodasLasFuentesCorrenSinInterfazYFallanSinPublicarNada: contra un servicio caído,
// cada una devuelve un error (el Publicador conserva lo publicado) y ninguna entra en
// pánico ni necesita una app de Fyne.
func TestTodasLasFuentesCorrenSinInterfazYFallanSinPublicarNada(t *testing.T) {
	base := servidor(t, nil)
	c := Comunes{}
	url := func(string) string { return base }
	todas := map[string]widgets.Fuente{}
	for producto, fabrica := range Registradas() {
		todas[producto] = fabrica(c, url, nil)
	}
	if len(todas) != 15 {
		t.Fatalf("hay %d fuentes; las apps con widget son quince", len(todas))
	}
	for nombre, f := range todas {
		ctx, parar := context.WithTimeout(context.Background(), 5*time.Second)
		if _, err := f(ctx); err == nil {
			t.Errorf("%s: con el servicio caído no devuelve error", nombre)
		}
		parar()
	}
}

func instancia(id, titulo string, ini, fin time.Time, diaCompleto bool, estado string) api.Instance {
	return api.Instance{Event: api.Event{ID: id, Summary: titulo, AllDay: diaCompleto, Status: estado}, Start: ini, End: fin}
}

// TestCalendarEnsenaHoyAunqueLaAppMireOtroMes: el caso del dueño. La Fuente no sabe
// qué mes ni qué calendarios enseña la pantalla: lee el mes en curso de TODOS, propios
// y compartidos, y el evento de hoy recién creado sale.
func TestCalendarEnsenaHoyAunqueLaAppMireOtroMes(t *testing.T) {
	zona := time.FixedZone("CET", 3600)
	ahora := time.Date(2026, 11, 6, 9, 30, 0, 0, zona)
	base := servidor(t, map[string]any{
		"/calendars":                map[string]any{"calendars": []api.Calendar{{ID: "trabajo", Color: "#16734D"}}},
		"/calendars/shared-with-me": map[string]any{"calendars": []api.Calendar{{ID: "equipo"}, {ID: "trabajo"}}},
		"/calendars/trabajo/instances": map[string]any{"instances": []api.Instance{
			instancia("ayer", "Ya pasó", ahora.Add(-26*time.Hour), ahora.Add(-25*time.Hour), false, ""),
			instancia("hoy", "Revisión de hoy", ahora.Add(time.Hour), ahora.Add(2*time.Hour), false, ""),
			instancia("anulado", "Anulado", ahora.Add(3*time.Hour), ahora.Add(4*time.Hour), false, "CANCELLED"),
		}},
		"/calendars/equipo/instances": map[string]any{"instances": []api.Instance{
			instancia("todo", "", time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC), true, ""),
		}},
	})
	c := Comunes{T: func(clave string) string { return "«" + clave + "»" }, Ahora: func() time.Time { return ahora }, Zona: func() *time.Location { return zona }}
	d, err := Calendar(c, api.NewCalendars(base, nil, nil))(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Counters) != 2 || d.Counters[0] != (widgets.Counter{Kind: "calendars", Value: 2}) || d.Counters[1].Value != 2 {
		t.Fatalf("contadores: %+v", d.Counters)
	}
	titulos := make([]string, 0, len(d.Items))
	for _, it := range d.Items {
		titulos = append(titulos, it.Title)
	}
	if strings.Join(titulos, "|") != "«event.untitled»|Revisión de hoy" {
		t.Fatalf("filas: %q", titulos)
	}
	if d.Items[1].Color != "#16734d" || d.Items[1].Badge != "10:30" {
		t.Fatalf("color o insignia: %+v", d.Items[1])
	}
	if d.Calendar == nil || d.Calendar.Month != 11 || d.Calendar.Year != 2026 || len(d.Calendar.BusyDays) == 0 {
		t.Fatalf("rejilla del mes en curso: %+v", d.Calendar)
	}
}

// TestCalendarElDiaCompletoEsFlotante: un evento de día completo del 6 lo guarda el
// servicio a medianoche UTC. En Madrid ocupa el 6 y nada más, y el 7 ya no sale; al
// oeste de Greenwich, tampoco la víspera. Es la regla con que el lector recorta las
// filas (widgets.Vigentes): publicador y lector dicen lo mismo.
func TestCalendarElDiaCompletoEsFlotante(t *testing.T) {
	seis := instancia("todo", "Fiesta", time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC), true, "")
	por := map[string][]api.Instance{"c": {seis}}
	cals := []api.Calendar{{ID: "c"}}
	c := Comunes{T: func(clave string) string { return clave }}
	for _, zona := range []*time.Location{time.FixedZone("CET", 3600), time.FixedZone("EST", -5*3600)} {
		d := detalleCalendar(c, cals, por, time.Date(2026, 11, 6, 23, 30, 0, 0, zona))
		if len(d.Items) != 1 || d.Calendar == nil || !slices.Equal(d.Calendar.BusyDays, []int{6}) {
			t.Fatalf("%s, el 6 a las 23:30: filas %+v, días ocupados %+v", zona, d.Items, d.Calendar)
		}
		if got := widgets.Vigentes(d.Items, "calendar", time.Date(2026, 11, 6, 23, 59, 59, 0, zona)); len(got) != 1 {
			t.Fatalf("%s: el lector quita el día completo antes de acabar el día", zona)
		}
		if got := widgets.Vigentes(d.Items, "calendar", time.Date(2026, 11, 7, 0, 0, 0, 0, zona)); len(got) != 0 {
			t.Fatalf("%s: el lector sigue enseñando el día completo del 6 el día 7", zona)
		}
		if d := detalleCalendar(c, cals, por, time.Date(2026, 11, 7, 0, 30, 0, 0, zona)); len(d.Items) != 0 {
			t.Fatalf("%s, el 7: el día completo del 6 sigue vigente: %+v", zona, d.Items)
		}
	}
}

// TestCalendarSinLosCompartidosNoPublica: un contador de calendarios sin los
// compartidos mentiría; se conserva lo publicado.
func TestCalendarSinLosCompartidosNoPublica(t *testing.T) {
	base := servidor(t, map[string]any{
		"/calendars":             map[string]any{"calendars": []api.Calendar{{ID: "a"}}},
		"/calendars/a/instances": map[string]any{"instances": []api.Instance{}},
	})
	if _, err := Calendar(Comunes{}, api.NewCalendars(base, nil, nil))(context.Background()); err == nil {
		t.Fatal("sin «compartidos conmigo» se publica igual")
	}
}

// TestMailLeeSiempreLaEntradaSinExtracto: el contador del servicio, una fila por hilo
// y nunca el extracto del mensaje.
func TestMailLeeSiempreLaEntradaSinExtracto(t *testing.T) {
	ahora := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	base := servidor(t, map[string]any{
		"/mailboxes": map[string]any{"mailboxes": []api.Mailbox{{ID: "env", Role: "sent", UnreadEmails: 9}, {ID: "in", Role: "inbox", UnreadEmails: 3}}},
		"/mailboxes/in/messages": map[string]any{"messages": []api.MailMessage{
			{ID: "1", ThreadID: "t1", FromName: "Ana", Subject: "Hola", Preview: "secreto del cuerpo", ReceivedAt: ahora},
			{ID: "2", ThreadID: "t1", FromName: "Ana", Subject: "Re: Hola", Keywords: []string{api.MailFlagSeen}, ReceivedAt: ahora.Add(-time.Hour)},
			{ID: "3", ThreadID: "t2", From: "bea@example.com", Subject: "Factura", Keywords: []string{api.MailFlagSeen}, ReceivedAt: ahora.Add(-2 * time.Hour)},
		}},
	})
	d, err := Mail(Comunes{Zona: func() *time.Location { return time.UTC }}, api.NewMail(base, nil, nil))(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d.Counters[0] != (widgets.Counter{Kind: "unread", Value: 3}) {
		t.Fatalf("el contador no es el de Recibidos: %+v", d.Counters)
	}
	if len(d.Items) != 2 || d.Items[0].Title != "Ana" || !d.Items[0].Unread || d.Items[1].Title != "bea@example.com" || d.Items[1].Unread {
		t.Fatalf("filas por hilo: %+v", d.Items)
	}
	for _, it := range d.Items {
		if strings.Contains(it.Title+it.Subtitle+it.Badge, "secreto") {
			t.Fatal("el extracto del correo ha llegado al widget")
		}
	}
}

// TestRoomsCuentaComoLaListaYNoEnsenaDirectos: un canal silenciado no suma, uno con
// «menciones» solo suma menciones, y un directo cuenta pero no sale con su nombre.
func TestRoomsCuentaComoLaListaYNoEnsenaDirectos(t *testing.T) {
	base := servidor(t, map[string]any{
		"/teams": map[string]any{"teams": []api.RoomsTeam{{ID: "eq"}}},
		"/teams/eq/channels": map[string]any{"channels": []api.RoomsChannel{
			{ID: "g", Kind: "public", Name: "general"},
			{ID: "s", Kind: "public", Name: "ruido", Muted: true},
			{ID: "m", Kind: "public", Name: "anuncios", NotifyLevel: "mentions"},
			{ID: "d", Kind: "direct", Name: "ana"},
		}},
		"/teams/eq/unread": map[string]any{"channels": []api.RoomsUnread{
			{ChannelID: "g", Unread: 2}, {ChannelID: "s", Unread: 50}, {ChannelID: "m", Unread: 9, Mentions: 1}, {ChannelID: "d", Unread: 4},
		}},
		"/me/external-channels": map[string]any{"channels": []api.RoomsChannel{}, "unread": []api.RoomsUnread{}},
	})
	d, err := Rooms(Comunes{}, api.NewRooms(base, nil, nil))(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d.Counters[0] != (widgets.Counter{Kind: "unread", Value: 7}) || d.Counters[1] != (widgets.Counter{Kind: "channels", Value: 4}) {
		t.Fatalf("contadores: %+v", d.Counters)
	}
	for _, it := range d.Items {
		if strings.Contains(it.Title, "ana") {
			t.Fatal("un directo sale en el widget con el nombre de la otra persona")
		}
	}
}

// TestNotesNoDescifraNiEnsenaUnCuadernoCifrado: un cuaderno cifrado cuenta sus notas,
// pero ninguna sale como fila del widget.
func TestNotesNoDescifraNiEnsenaUnCuadernoCifrado(t *testing.T) {
	base := servidor(t, map[string]any{
		"/notebooks": map[string]any{"notebooks": []api.Notebook{{ID: "c", Title: "Diario", Encrypted: true, NoteCount: 5}}},
	})
	d, err := Notes(Comunes{}, api.NewNotes(base, nil, nil))(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Items) != 0 || d.Counters[0].Value != 5 {
		t.Fatalf("cuaderno cifrado: %+v", d)
	}
}

// TestHubEnsenaLaMismaAgendaQueLaTarjetaHoy: el widget del Hub lee api.Calendars.Hoy,
// la agenda de la tarjeta «Hoy»: seis como mucho y un calendario caído no la tumba. El
// contador «events» es el de la tarjeta, no el total sin cortar.
func TestHubEnsenaLaMismaAgendaQueLaTarjetaHoy(t *testing.T) {
	zona := time.UTC
	ahora := time.Date(2026, 11, 6, 12, 0, 0, 0, zona)
	inicios := []int{23, 9, 10, 11, 13, 14, 15, 8}
	horas := make([]api.Instance, 0, len(inicios))
	for _, h := range inicios {
		ini := time.Date(2026, 11, 6, h, 0, 0, 0, zona)
		horas = append(horas, instancia("e", "Evento", ini, ini.Add(time.Hour), false, ""))
	}
	base := servidor(t, map[string]any{
		"/calendars":             map[string]any{"calendars": []api.Calendar{{ID: "a"}, {ID: "roto"}}},
		"/calendars/a/instances": map[string]any{"instances": horas},
		"/mailboxes":             map[string]any{"mailboxes": []api.Mailbox{{ID: "in", Role: "inbox", UnreadEmails: 4}}},
	})
	c := Comunes{Ahora: func() time.Time { return ahora }, Zona: func() *time.Location { return zona }}
	d, err := Hub(c, api.NewHub(base, nil, nil), api.NewCalendars(base, nil, nil), api.NewMail(base, nil, nil))(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Counters) != 2 || d.Counters[1] != (widgets.Counter{Kind: "events", Value: api.MaxEventosHoy}) {
		t.Fatalf("contadores: %+v", d.Counters)
	}
	if len(d.Items) != api.MaxEventosHoy || d.Items[0].Badge != "08:00" || d.Items[0].Unread || !d.Items[5].Unread || d.Items[5].Badge != "14:00" {
		t.Fatalf("agenda: %+v", d.Items)
	}
}

// servidorConEstados es `servidor` con códigos a elegir por ruta. La ruta de /videos
// lleva además su `scope` y su `status`, que es lo que distingue las lecturas de Cast.
func servidorConEstados(t *testing.T, rutas map[string]any, estados map[string]int) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ruta := strings.TrimPrefix(r.URL.Path, "/api/v1")
		if ruta == "/videos" {
			ruta += " " + r.URL.Query().Get("scope") + " " + r.URL.Query().Get("status")
		}
		if codigo, hay := estados[ruta]; hay {
			w.WriteHeader(codigo)
			return
		}
		cuerpo, ok := rutas[ruta]
		if !ok {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cuerpo)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestUnaLecturaSecundariaQueFallaConservaLoPublicado: si falla la red en la lectura de
// los directos de Cast, de los álbumes de Photos o de la cuota de Drive, la Fuente
// devuelve error y el Publicador conserva lo publicado, en vez de publicar con éxito un
// parcial que borra el directo en curso. Un 403/404 (no es para ti, o el servicio no lo
// tiene) sí publica sin ese dato.
func TestUnaLecturaSecundariaQueFallaConservaLoPublicado(t *testing.T) {
	no := false
	cast := map[string]any{
		"/videos " + api.CastMine + " ":                           api.CastPage{Videos: []api.CastVideo{{ID: "v", Title: "Mío"}}},
		"/videos " + api.CastOrgScope + " " + api.CastStatusReady: api.CastPage{},
		"/videos " + api.CastOrgScope + " " + api.CastStatusLive:  api.CastPage{Videos: []api.CastVideo{{ID: "l", Title: "Directo"}}},
		"/quota": api.CastQuota{LimitBytes: 100, UsedBytes: 50, ExternalStorage: &no},
	}
	fotos := map[string]any{
		"/timeline": map[string]any{"items": []api.Photo{{ID: "f", Name: "playa.jpg"}}},
		"/albums":   map[string]any{"items": []api.Album{{ID: "a", Title: "Verano", Count: 3}}},
	}
	drive := map[string]any{
		"/folders/root":       api.Folder{ID: "r"},
		"/folders/r/children": api.Children{Files: []api.File{{ID: "f", Name: "informe.pdf"}}},
		"/quota":              api.DriveQuota{LimitBytes: 100, UsedBytes: 10},
	}
	casos := []struct {
		nombre string
		fuente func(base string) widgets.Fuente
		rutas  map[string]any
		ruta   string
	}{
		{"los directos de Cast", func(b string) widgets.Fuente { return Cast(Comunes{}, api.NewCast(b, nil, nil)) }, cast, "/videos " + api.CastOrgScope + " " + api.CastStatusLive},
		{"lo publicado en la organización", func(b string) widgets.Fuente { return Cast(Comunes{}, api.NewCast(b, nil, nil)) }, cast, "/videos " + api.CastOrgScope + " " + api.CastStatusReady},
		{"la cuota de Cast", func(b string) widgets.Fuente { return Cast(Comunes{}, api.NewCast(b, nil, nil)) }, cast, "/quota"},
		{"los álbumes", func(b string) widgets.Fuente { return Photos(Comunes{}, api.NewPhotos(b, nil, nil)) }, fotos, "/albums"},
		{"la cuota de Drive", func(b string) widgets.Fuente { return Drive(Comunes{}, api.NewDrive(b, nil, nil)) }, drive, "/quota"},
	}
	for _, c := range casos {
		if _, err := c.fuente(servidorConEstados(t, c.rutas, nil))(context.Background()); err != nil {
			t.Fatalf("%s: con todo bien no publica: %v", c.nombre, err)
		}
		if _, err := c.fuente(servidorConEstados(t, c.rutas, map[string]int{c.ruta: http.StatusBadGateway}))(context.Background()); err == nil {
			t.Errorf("%s: con un fallo de red se publica un parcial", c.nombre)
		}
		if _, err := c.fuente(servidorConEstados(t, c.rutas, map[string]int{c.ruta: http.StatusNotFound}))(context.Background()); err != nil {
			t.Errorf("%s: un 404 no publica sin ese dato: %v", c.nombre, err)
		}
	}
}
