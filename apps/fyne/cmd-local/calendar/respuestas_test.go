// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateCalendar "git.sytue.eu/sytue/one/apps/nucleo/calendar"
	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/correo"
)

func replyICS(uid, asistente, partstat string, sello time.Time) string {
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//prueba//ES\r\nMETHOD:REPLY\r\nBEGIN:VEVENT\r\nUID:" + uid +
		"\r\nSEQUENCE:0\r\nDTSTAMP:" + sello.UTC().Format("20060102T150405Z") + "\r\nORGANIZER:mailto:ana@example.org\r\n" +
		"ATTENDEE;PARTSTAT=" + partstat + ":mailto:" + asistente + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
}

// aplicadorFalso contesta lo que se le diga por UID y apunta lo que recibe.
type aplicadorFalso struct {
	por      map[string]agenda.EstadoRespuesta
	recibido []string
}

func (a *aplicadorFalso) AplicarRespuestas(_ context.Context, _ agenda.Local, _ string, replies [][]byte) []agenda.EstadoRespuesta {
	out := make([]agenda.EstadoRespuesta, len(replies))
	for i, r := range replies {
		uid := strings.SplitN(strings.SplitN(string(r), "UID:", 2)[1], "\r\n", 2)[0]
		a.recibido = append(a.recibido, uid)
		out[i] = a.por[uid]
	}
	return out
}

// Calendar recoge la cola de respuestas que deja Mail (`<Datos>/Correo/respuestas-imip/`),
// de la más vieja a la más nueva: lo aplicado y lo que no aplica a nada salen de la cola;
// lo que espera (su evento aún no está aquí) se queda, salvo que lleve más de un mes; un
// temporal a medio escribir no se toca. (La cadena entera —Mail por IMAP, la cola, el
// evento y la subida a CalDAV— está en `agenda.TestRespuestaDePuntaAPunta`.)
func TestCalendarRecogeLasRespuestasDeMail(t *testing.T) {
	raiz := t.TempDir()
	cola := filepath.Join(raiz, "respuestas-imip")
	if err := os.MkdirAll(cola, 0o700); err != nil {
		t.Fatal(err)
	}
	poner := func(nombre, uid string, edad time.Duration) {
		t.Helper()
		ruta := filepath.Join(cola, nombre)
		if err := os.WriteFile(ruta, []byte(replyICS(uid, "luis@example.org", "ACCEPTED", time.Now())), 0o600); err != nil {
			t.Fatal(err)
		}
		cuando := time.Now().Add(-edad)
		_ = os.Chtimes(ruta, cuando, cuando)
	}
	poner("1.ics", "aplicada", 3*time.Minute)
	poner("2.ics", "espera", 2*time.Minute)
	poner("3.ics", "vieja", 40*24*time.Hour)
	poner("4.ics", "descartada", time.Minute)
	if err := os.WriteFile(filepath.Join(cola, ".5.ics.123.tmp"), []byte("a medias"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := &aplicadorFalso{por: map[string]agenda.EstadoRespuesta{
		"aplicada": agenda.RespuestaAplicada, "espera": agenda.RespuestaEspera,
		"vieja": agenda.RespuestaEspera, "descartada": agenda.RespuestaDescartada,
	}}
	if n := aplicarCola(t.Context(), a, nil, "", raiz); n != 1 {
		t.Fatalf("aplicadas: %d", n)
	}
	if strings.Join(a.recibido, ",") != "vieja,aplicada,espera,descartada" {
		t.Fatalf("por orden de llegada: %v", a.recibido)
	}
	if quedan := correo.RespuestasEnCola(raiz); len(quedan) != 1 || quedan[0] != "2.ics" {
		t.Fatalf("en la cola queda lo que espera: %v", quedan)
	}
	if _, err := os.Stat(filepath.Join(cola, ".5.ics.123.tmp")); err != nil {
		t.Fatal("un temporal a medias no se toca")
	}
	// Sin cola (Mail no está), nada.
	if n := aplicarCola(t.Context(), a, nil, "", t.TempDir()); n != 0 {
		t.Fatal(n)
	}
}

// «Añadir Google Meet» o «Reunión de Teams»: lo pedido en el formulario marca el evento al
// guardarlo, solo en el calendario del proveedor que lo crea y si se pidió hace poco.
func TestLaReunionPedidaMarcaElEvento(t *testing.T) {
	repo := abrir(t, t.TempDir(), "calendar", func(r *almacen.Recursos) (*privateCalendar.Repository, error) { return privateCalendar.New(r.Config) })
	t.Cleanup(repo.Close)
	ctx := t.Context()
	google, err := repo.CreateCalendar(ctx, api.CalendarInput{Name: "g", DisplayName: "Ana (Google)", Timezone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	propio, err := repo.CreateCalendar(ctx, api.CalendarInput{Name: "p", DisplayName: "Personal", Timezone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	s := &sincronia{}
	s.cals.poner([]agenda.Grupo{{Cuenta: agenda.Cuenta{ID: "c1", Proveedor: agenda.ProveedorGoogle}, Colecciones: []agenda.ColeccionLocal{{ID: google.ID}}}})
	r := nuevasReuniones(s, repo)
	s.reuniones = r
	inicio := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Hour)
	crear := func(cal, uid string) api.Event {
		ev, err := repo.CreateEvent(ctx, cal, api.EventInput{UID: uid, Summary: "Revisión", Start: inicio, End: inicio.Add(time.Hour), TZID: "UTC"})
		if err != nil {
			t.Fatal(err)
		}
		return ev
	}
	datos := func(cal string) string {
		lista, _ := repo.ObjetosCrudos(ctx, cal)
		var b strings.Builder
		for _, o := range lista {
			b.WriteString(o.Datos)
		}
		return b.String()
	}
	if r.tipoDe(google.ID) != agenda.ReunionMeet || r.tipoDe(propio.ID) != "" {
		t.Fatal("Meet en el de Google; en el de este equipo, nada del proveedor")
	}
	pantalla := r.pantalla(nil)
	pantalla.Pedir(google.ID, "uid-1@prueba")
	pantalla.Pedir(propio.ID, "uid-2@prueba")
	s.marcarReunion(ctx, crear(google.ID, "uid-1@prueba"))
	s.marcarReunion(ctx, crear(propio.ID, "uid-2@prueba"))
	if !strings.Contains(datos(google.ID), agenda.PropReunion+":"+agenda.ReunionMeet) {
		t.Fatalf("el de Google lleva la marca: %s", datos(google.ID))
	}
	if strings.Contains(datos(propio.ID), agenda.PropReunion) {
		t.Fatal("en un calendario que no es del proveedor no se marca nada")
	}
	// Una pedida vieja no se usa.
	pantalla.Pedir(google.ID, "uid-3@prueba")
	r.mu.Lock()
	p := r.pedidas["uid-3@prueba"]
	p.cuando = time.Now().Add(-time.Hour)
	r.pedidas["uid-3@prueba"] = p
	r.mu.Unlock()
	ev := crear(google.ID, "uid-3@prueba")
	s.marcarReunion(ctx, ev)
	if strings.Count(datos(google.ID), agenda.PropReunion) != 1 {
		t.Fatalf("una pedida de hace una hora no vale: %s", datos(google.ID))
	}
	// Sin sincronía (la app sin cuentas), nada.
	var nadie *sincronia
	nadie.marcarReunion(ctx, ev)
}
