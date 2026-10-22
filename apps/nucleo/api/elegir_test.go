// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// servidorSuite contesta como los cinco servicios a la vez. Cada cliente cuelga de
// `/api/v1`, así que las rutas son las del servicio de verdad.
func servidorSuite(t *testing.T, pedidas *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pedidas.Add(1)
		q := r.URL.Query()
		if r.Method == "QUERY" { // Consulta manda los filtros en el cuerpo
			cuerpo, _ := io.ReadAll(r.Body)
			q, _ = url.ParseQuery(string(cuerpo))
		}
		switch r.URL.Path {
		case "/api/v1/folders/root":
			_, _ = io.WriteString(w, `{"id":"R","name":"Mi unidad","is_root":true}`)
		case "/api/v1/folders/R/children":
			if q.Get("cursor") != "c2" {
				_, _ = io.WriteString(w, `{"folders":[{"id":"F1","name":"Facturas"}],
					"files":[{"id":"a b","name":"acta.pdf","size_bytes":2048}],"next_cursor":"c2"}`)
				return
			}
			_, _ = io.WriteString(w, `{"folders":[],"files":[{"id":"z","name":"z.txt","size_bytes":1}]}`)
		case "/api/v1/search":
			if q.Get("q") != "acta" {
				t.Errorf("Drive busca %q", q.Get("q"))
			}
			_, _ = io.WriteString(w, `{"files":[{"id":"a b","name":"acta.pdf","is_directory":false,"size_bytes":2048},
				{"id":"F9","name":"Actas","is_directory":true}]}`)
		case "/api/v1/documents":
			if q.Get("q") != "plan" || q.Get("limit") != "30" {
				t.Errorf("Docs recibe q=%q limit=%q", q.Get("q"), q.Get("limit"))
			}
			_, _ = io.WriteString(w, `{"documents":[{"id":"d1","kind":"spreadsheet","title":"Plan"}]}`)
		case "/api/v1/events/search":
			if q.Get("q") != "comité" || q.Get("limit") != "25" {
				t.Errorf("Calendar recibe q=%q limit=%q", q.Get("q"), q.Get("limit"))
			}
			_, _ = io.WriteString(w, `{"events":[{"id":"e1","uid":"uid-1@sytue","summary":"Comité",
				"dtstart":"2026-10-01T09:00:00Z"}]}`)
		case "/api/v1/teams":
			_, _ = io.WriteString(w, `{"teams":[{"id":"T1"},{"id":"T2"}]}`)
		case "/api/v1/teams/T1/channels":
			_, _ = io.WriteString(w, `{"channels":[{"id":"c1","name":"General","topic":"Todo"},
				{"id":"c2","name":"Ventas"},{"id":"c3","name":"","kind":"direct","peers":["Ana"]}]}`)
		case "/api/v1/teams/T2/channels":
			http.Error(w, "caído", http.StatusInternalServerError)
		case "/api/v1/notes/search":
			_, _ = io.WriteString(w, `{"notes":[{"id":"n1","notebook_id":"k1","notebook":"Casa","title":""}]}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
}

func elegirContra(srv *httptest.Server) Elegir {
	return Elegir{
		Apex:  ApexProd,
		Org:   func() string { return "acme" },
		Drive: NewDrive(srv.URL, nil, nil), Docs: NewDocs(srv.URL, nil, nil),
		Calendar: NewCalendars(srv.URL, nil, nil), Rooms: NewRooms(srv.URL, nil, nil),
		Notes: NewNotes(srv.URL, nil, nil),
	}
}

// Cada origen devuelve lo que devuelve el BFF de la web —carpetas que se navegan sin
// enlace, ficheros con el suyo, el UID del evento como identidad, el cuaderno como meta
// de la nota— y los enlaces llevan el PREFIJO de la app y la ORGANIZACIÓN de la sesión
// detrás de él (ADR 0056): sin el prefijo, producción da 404.
func TestElegirCadaOrigenComoElBFF(t *testing.T) {
	var pedidas atomic.Int32
	srv := servidorSuite(t, &pedidas)
	defer srv.Close()
	e := elegirContra(srv)
	ctx := context.Background()

	casos := []struct {
		origen             OrigenSuite
		carpeta, q, cursor string
		quiero             []Elegible
		cursorQuiero       string
	}{
		{ElegirDrive, "", "", "", []Elegible{
			{ID: "F1", Nombre: "Facturas", Carpeta: true},
			{ID: "a b", Nombre: "acta.pdf", URL: "https://drive.sytue.eu/drive/acme/files/a%20b", Bytes: 2048},
		}, "c2"},
		{ElegirDrive, "R", "", "c2", []Elegible{
			{ID: "z", Nombre: "z.txt", URL: "https://drive.sytue.eu/drive/acme/files/z", Bytes: 1},
		}, ""},
		// Con término, Drive busca en toda la unidad: lo que pide la herramienta de enlaces.
		{ElegirDrive, "", "acta", "", []Elegible{
			{ID: "F9", Nombre: "Actas", Carpeta: true},
			{ID: "a b", Nombre: "acta.pdf", URL: "https://drive.sytue.eu/drive/acme/files/a%20b", Bytes: 2048},
		}, ""},
		{ElegirDocs, "", " plan ", "", []Elegible{
			{ID: "d1", Nombre: "Plan", Meta: "spreadsheet", URL: "https://calcus.sytue.eu/calcus/acme/edit?id=d1"},
		}, ""},
		{ElegirCalendar, "", "comité", "", []Elegible{
			{ID: "uid-1@sytue", Nombre: "Comité", Meta: "2026-10-01T09:00:00Z", URL: "https://calendar.sytue.eu/calendar/acme/events/e1"},
		}, ""},
		// El equipo T2 contesta 500 y no se lleva por delante los canales de T1.
		{ElegirRooms, "", "", "", []Elegible{
			{ID: "c1", Nombre: "General", Meta: "Todo", URL: "https://rooms.sytue.eu/rooms/acme/channels/c1"},
			{ID: "c2", Nombre: "Ventas", URL: "https://rooms.sytue.eu/rooms/acme/channels/c2"},
			{ID: "c3", Nombre: "Ana", URL: "https://rooms.sytue.eu/rooms/acme/channels/c3"},
		}, ""},
		{ElegirRooms, "", "VEN", "", []Elegible{
			{ID: "c2", Nombre: "Ventas", URL: "https://rooms.sytue.eu/rooms/acme/channels/c2"},
		}, ""},
		{ElegirNotes, "", "casa", "", []Elegible{
			{ID: "n1", Nombre: "Casa", Meta: "Casa", URL: "https://notes.sytue.eu/notes/acme/?notebook=k1&note=n1"},
		}, ""},
	}
	for _, c := range casos {
		p, err := e.Listar(ctx, c.origen, c.carpeta, c.q, c.cursor)
		if err != nil {
			t.Fatalf("%s: %v", c.origen, err)
		}
		if !reflect.DeepEqual(p.Elegibles, c.quiero) || p.Cursor != c.cursorQuiero {
			t.Errorf("%s %q:\n tengo %+v (cursor %q)\nquiero %+v (cursor %q)", c.origen, c.q, p.Elegibles, p.Cursor, c.quiero, c.cursorQuiero)
		}
	}

	// Sin término, Calendar y Notes no preguntan: no hay «últimos eventos».
	antes := pedidas.Load()
	for _, o := range []OrigenSuite{ElegirCalendar, ElegirNotes} {
		if p, err := e.Listar(ctx, o, "", "  ", ""); err != nil || len(p.Elegibles) != 0 {
			t.Errorf("%s sin término = %+v, %v", o, p, err)
		}
	}
	if pedidas.Load() != antes {
		t.Error("sin término se ha preguntado al servicio")
	}
	if _, err := (Elegir{}).Listar(ctx, ElegirDocs, "", "x", ""); !errors.Is(err, ErrOrigenNoDisponible) {
		t.Errorf("un origen sin cliente = %v", err)
	}
}

// El enlace de una nota es el MISMO que compone la web para la búsqueda y el selector:
// una referencia guardada desde el escritorio tiene que abrir igual desde el navegador.
func TestElEnlaceDeNotaEsElDeLaWeb(t *testing.T) {
	// La web lo compone con `urlDeNotaPublica` sobre `appUrl('notes', '', org)` (lib/elegibles.ts), que
	// pone `notebook` y `note` en la consulta de la RAÍZ de la app (client/enlaces.ts), con la
	// organización detrás del prefijo cuando la sesión la tiene (ADR 0056).
	elegibles := webref.Read(t, "apps/frontend/apps/notes/src/lib/elegibles.ts")
	enlaces := webref.Read(t, "apps/frontend/apps/notes/src/client/enlaces.ts")
	if !strings.Contains(elegibles, "urlDeNotaPublica(notebookId, noteId, org)") ||
		!strings.Contains(enlaces, "u.searchParams.set('notebook', notebookId);") ||
		!strings.Contains(enlaces, "u.searchParams.set('note', noteId);") {
		t.Fatal("la web ya no compone el enlace de la nota como `/?notebook=…&note=…`: revisar EnlaceNota")
	}
	if got := EnlaceNota(ApexProd, "", "k 1", "n&1"); got != "https://notes.sytue.eu/notes/?notebook=k+1&note=n%261" {
		t.Errorf("EnlaceNota sin org = %s", got)
	}
	// Y con la organización detrás del prefijo (ADR 0056).
	if got := EnlaceNota(ApexProd, "acme", "k 1", "n&1"); got != "https://notes.sytue.eu/notes/acme/?notebook=k+1&note=n%261" {
		t.Errorf("EnlaceNota con org = %s", got)
	}
}

// Lo que se emite es lo que se lee, y con los NOMBRES que usan el Hub y la web: si uno
// cambia `attach_drive` o `share_source`, la otra app deja de entender el enlace sin que
// nada falle a la vista.
func TestLosEnlacesEntreAppsSeLeenComoSeEmiten(t *testing.T) {
	r := Redactar{Para: "a@b.eu", Asunto: "Acta & firma", Cuerpo: "hola\nadiós", Drive: []string{"f1", " ", "f 2"}}
	u, err := url.Parse(Esquema("mail") + ":" + RutaRedactar(r))
	if err != nil {
		t.Fatal(err)
	}
	leido, ok := LeerRedactar(u)
	r.Drive = []string{"f1", "f 2"} // el vacío no viaja
	if !ok || !reflect.DeepEqual(leido, r) {
		t.Errorf("LeerRedactar(RutaRedactar) = %+v, %v", leido, ok)
	}
	if _, ok := LeerRedactar(&url.URL{Scheme: "sytue-mail", Path: "/", RawQuery: "subject=x"}); ok {
		t.Error("un enlace de Mail sin action=compose no es redactar")
	}
	if _, ok := LeerRedactar(&url.URL{Scheme: "sytue-rooms", RawQuery: "action=compose"}); ok {
		t.Error("un enlace de otra app no es de Mail")
	}
	// El respaldo web cae en la RAÍZ de Mail, que existe; `/compose` da 404.
	if web := AppURL(ApexProd, "mail", "", RutaRedactar(Redactar{Drive: []string{"f1"}})); web != "https://mail.sytue.eu/mail/?action=compose&attach_drive=f1" {
		t.Errorf("respaldo web del redactor = %s", web)
	}

	c := Compartir{Origen: "drive", ID: "f1", Texto: "acta.pdf\nhttps://drive.sytue.eu/drive/files/f1"}
	u, _ = url.Parse(Esquema("rooms") + ":" + RutaCompartir(c))
	if leido, ok := LeerCompartir(u); !ok || leido != c {
		t.Errorf("LeerCompartir(RutaCompartir) = %+v, %v", leido, ok)
	}
	largo := Compartir{Origen: "drive", ID: "f", Texto: strings.Repeat("ñ", 1500)}
	u, _ = url.Parse(Esquema("rooms") + ":" + RutaCompartir(largo))
	if leido, _ := LeerCompartir(u); len([]rune(leido.Texto)) != 1000 {
		t.Errorf("el texto se recorta a 1000 runas como en el Hub: %d", len([]rune(leido.Texto)))
	}

	hub := webref.Read(t, "services/hub/internal/intents/intents.go")
	mail := webref.Read(t, "apps/frontend/apps/mail/src/components/MailApp.tsx")
	rooms := webref.Read(t, "apps/frontend/apps/rooms/src/components/ShareLanding.tsx")
	for _, x := range []struct{ fuente, texto, dice string }{
		{hub, `q.Add("attach_drive", id)`, "el Hub"},
		{hub, `q.Set("share_source", string(src))`, "el Hub"},
		{hub, `q.Set("share_id", resourceID)`, "el Hub"},
		{mail, `p.getAll('attach_drive')`, "la bandeja de la web"},
		{mail, `p.get('action') !== 'compose'`, "la bandeja de la web"},
		{rooms, `params.get('share_source')`, "Compartir de la web"},
		{rooms, `params.get('text')`, "Compartir de la web"},
	} {
		if !strings.Contains(x.fuente, x.texto) {
			t.Errorf("%s ya no usa %s: revisar RutaRedactar/RutaCompartir", x.dice, x.texto)
		}
	}
}

// El mensaje de «Compartir en Rooms» de una nota: su enlace, nunca su contenido, con el
// título recortado a 200 como el BFF.
func TestMensajeNota(t *testing.T) {
	if got := MensajeNota(ApexProd, "acme", "k1", "n1", "  Acta  ", "Nota"); got != "📝 Acta\nhttps://notes.sytue.eu/notes/acme/?notebook=k1&note=n1" {
		t.Errorf("MensajeNota = %q", got)
	}
	if got := MensajeNota(ApexProd, "", "k1", "n1", " ", "Sin título"); !strings.HasPrefix(got, "📝 Sin título\n") {
		t.Errorf("sin título = %q", got)
	}
	if got := MensajeNota(ApexProd, "", "k", "n", strings.Repeat("a", 300), ""); strings.Count(got, "a") != 200 {
		t.Errorf("el título no se recorta a 200: %q", got)
	}
}
