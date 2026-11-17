// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"io"
	"net/http"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// conSuelto es Sytue Calendar con un evento suelto y sin invitados (e2), con dos
// recordatorios que solo trae su lectura, además de lo de `escrituras`.
type conSuelto struct{ escrituras }

func (s *conSuelto) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method + " " + r.URL.Path {
	case "GET /api/v1/calendars/mio/instances":
		_, _ = io.WriteString(w, `{"instances":[
			{"event":{"id":"e1","uid":"u1","summary":"Comité","dtstart":"2026-09-10T08:00:00Z","dtend":"2026-09-10T09:00:00Z","attendees":[{"email":"ana@sytue.eu","partstat":"ACCEPTED"}]},"start":"2026-09-10T08:00:00Z","end":"2026-09-10T09:00:00Z"},
			{"event":{"id":"e2","calendar_id":"mio","uid":"u2","summary":"Dentista","dtstart":"2026-09-22T09:00:00Z","dtend":"2026-09-22T10:00:00Z"},"start":"2026-09-22T09:00:00Z","end":"2026-09-22T10:00:00Z"},
			{"event":{"id":"s1","uid":"us","summary":"Diario","rrule":"FREQ=WEEKLY;COUNT=10","dtstart":"2026-09-01T07:00:00Z","dtend":"2026-09-01T07:30:00Z"},"start":"2026-09-15T07:00:00Z","end":"2026-09-15T07:30:00Z"}]}`)
		return
	case "GET /api/v1/events/e2":
		_, _ = io.WriteString(w, `{"id":"e2","calendar_id":"mio","uid":"u2","summary":"Dentista","dtstart":"2026-09-22T09:00:00Z","dtend":"2026-09-22T10:00:00Z","reminders":[15,60]}`)
		return
	}
	s.escrituras.ServeHTTP(w, r)
}

// ultimoAviso es el aviso más reciente de la pila.
func ultimoAviso(t *testing.T, a *App) *ui.Aviso {
	t.Helper()
	as := a.avisos.Avisos()
	if len(as) == 0 {
		t.Fatal("no ha salido ningún aviso")
	}
	return as[len(as)-1]
}

// TestDeshacerUnMovimientoDevuelveElBloqueASuHora: mover un evento suelto avisa
// «Evento movido» con «Deshacer», que vuelve a guardar el inicio y el fin de antes.
// VISTO EN ROJO sin el aviso con acción de `mover`.
func TestDeshacerUnMovimientoDevuelveElBloqueASuHora(t *testing.T) {
	s := &conSuelto{}
	a, _ := appConVentana(t, s)
	o := ocurrenciaDe(t, a, "e2")
	a.mover(o, time.Date(2026, 9, 22, 14, 0, 0, 0, time.UTC), time.Date(2026, 9, 22, 15, 30, 0, 0, time.UTC))
	av := ultimoAviso(t, a)
	if av.Titulo != "Evento movido" || av.RotuloAccion != "Deshacer" || av.AlPulsarAccion == nil {
		t.Fatalf("el aviso es %q con acción %q", av.Titulo, av.RotuloAccion)
	}
	if av.Duracion != ui.DuracionDeshacer {
		t.Errorf("el aviso dura %v y deshacer pide %v", av.Duracion, ui.DuracionDeshacer)
	}
	av.AlPulsarAccion()
	hechas, cuerpos := s.apuntadas()
	if len(hechas) != 2 || hechas[0] != "PUT /api/v1/events/e2" || hechas[1] != "PUT /api/v1/events/e2" {
		t.Fatalf("mover y deshacer hicieron %v", hechas)
	}
	if cuerpos[0]["start"] != "2026-09-22T14:00:00Z" {
		t.Errorf("el movimiento guardó %v", cuerpos[0]["start"])
	}
	if cuerpos[1]["start"] != "2026-09-22T09:00:00Z" || cuerpos[1]["end"] != "2026-09-22T10:00:00Z" {
		t.Errorf("deshacer guardó %v–%v y no la hora de antes", cuerpos[1]["start"], cuerpos[1]["end"])
	}
	if _, ok := cuerpos[1]["reminders"]; ok {
		t.Error("deshacer manda recordatorios y el servicio dejaría de conservar los que hay")
	}
}

// TestDeshacerUnBorradoLoRecuperaConSusRecordatorios: borrar un evento suelto y sin
// invitados avisa con «Deshacer», que lo vuelve a crear en su calendario con su UID y
// sus recordatorios. Con invitados, no se ofrece.
func TestDeshacerUnBorradoLoRecuperaConSusRecordatorios(t *testing.T) {
	s := &conSuelto{}
	a, _ := appConVentana(t, s)
	a.eliminar(ocurrenciaDe(t, a, "e2"))
	av := ultimoAviso(t, a)
	if av.Titulo != "Evento eliminado" || av.RotuloAccion != "Deshacer" || av.AlPulsarAccion == nil {
		t.Fatalf("el aviso es %q con acción %q", av.Titulo, av.RotuloAccion)
	}
	av.AlPulsarAccion()
	hechas, cuerpos := s.apuntadas()
	if len(hechas) != 2 || hechas[0] != "DELETE /api/v1/events/e2" || hechas[1] != "POST /api/v1/calendars/mio/events" {
		t.Fatalf("borrar y deshacer hicieron %v", hechas)
	}
	c := cuerpos[1]
	if c["uid"] != "u2" || c["summary"] != "Dentista" || c["start"] != "2026-09-22T09:00:00Z" {
		t.Errorf("deshacer recreó %v", c)
	}
	if r, _ := c["reminders"].([]any); len(r) != 2 || r[0] != float64(15) || r[1] != float64(60) {
		t.Errorf("deshacer recreó los recordatorios %v", c["reminders"])
	}
	if got := ultimoAviso(t, a).Titulo; got != "Evento recuperado" {
		t.Errorf("tras deshacer avisa %q", got)
	}

	// Con invitados: borrar avisa sin «Deshacer» (no se manda otra invitación).
	a.eliminar(ocurrenciaDe(t, a, "e1"))
	if av := ultimoAviso(t, a); av.Titulo != "Evento eliminado" || av.RotuloAccion != "" {
		t.Errorf("con invitados el aviso es %q con acción %q", av.Titulo, av.RotuloAccion)
	}
}

// TestDeshacerNoSeOfreceEnSeries: una serie se borra por alcance y su vuelta atrás no
// es un único evento.
func TestDeshacerNoSeOfreceEnSeries(t *testing.T) {
	s := &conSuelto{}
	a, _ := appConVentana(t, s)
	if puedeDeshacerBorrado(ocurrenciaDe(t, a, "s1")) {
		t.Error("se ofrece deshacer el borrado de una serie")
	}
	o := ocurrenciaDe(t, a, "e2")
	o.SoloLectura = true
	if puedeDeshacerBorrado(o) {
		t.Error("se ofrece deshacer el borrado de un evento de solo lectura")
	}
	if !puedeDeshacerBorrado(ocurrenciaDe(t, a, "e2")) {
		t.Error("no se ofrece deshacer el borrado de un evento suelto sin invitados")
	}
}
