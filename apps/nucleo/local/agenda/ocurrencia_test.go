// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/dav/davprueba"
	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
)

// serieICS es una reunión semanal de cinco que organiza Ana, empezando en inicio (en
// Madrid), sin la tercera (EXDATE).
func serieICS(uid string, inicio time.Time) string {
	h := func(t time.Time) string { return t.Format("20060102T150405") }
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//prueba//ES\r\nBEGIN:VEVENT\r\nUID:" + uid + "\r\nDTSTAMP:20260101T000000Z\r\nSEQUENCE:1\r\n" +
		"DTSTART;TZID=Europe/Madrid:" + h(inicio) + "\r\nDTEND;TZID=Europe/Madrid:" + h(inicio.Add(90*time.Minute)) + "\r\n" +
		"RRULE:FREQ=WEEKLY;COUNT=5\r\nEXDATE;TZID=Europe/Madrid:" + h(inicio.AddDate(0, 0, 14)) + "\r\n" +
		"SUMMARY:Seguimiento semanal\r\nLOCATION:Sala 2\r\n" +
		"ORGANIZER;CN=Ana:mailto:ana@example.org\r\nATTENDEE;CN=Ana;PARTSTAT=ACCEPTED:mailto:ana@example.org\r\n" +
		"ATTENDEE;CN=Luis;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:luis@example.org\r\nATTENDEE;CN=Marta;PARTSTAT=ACCEPTED:mailto:marta@example.org\r\n" +
		"BEGIN:VALARM\r\nACTION:DISPLAY\r\nDESCRIPTION:Aviso\r\nTRIGGER:-PT15M\r\nEND:VALARM\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
}

// A-11: la respuesta de un invitado a UNA ocurrencia de una serie que aquí no tiene
// excepción CREA la excepción de esa ocurrencia (en su hora, con los invitados, la
// SEQUENCE y los avisos del maestro, sin la repetición) con su PARTSTAT, y la vuelta la
// sube al servidor sin mandar ninguna invitación a nadie: ni por correo ni a la cola de
// las que no salen, aunque crearla cambie lo que ven los invitados. Una respuesta a lo que
// no es una ocurrencia (quitada, de otra hora) o de una versión anterior no crea nada, y
// la que no cambia nada tampoco. Si después la persona cambia la serie, la invitación sí
// sale, por su cambio, y lleva las excepciones.
func TestLaRespuestaAUnaOcurrenciaCreaSuExcepcionSinMandarNada(t *testing.T) {
	srv := davprueba.Nuevo()
	defer srv.Close()
	col := davprueba.CasaCal + "personal/"
	srv.Crear(col, "Personal", false)
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Fatal(err)
	}
	hoy := time.Now().In(madrid).AddDate(0, 0, 3)
	inicio := time.Date(hoy.Year(), hoy.Month(), hoy.Day(), 9, 30, 0, 0, madrid)
	srv.Poner(col+"serie-2.ics", []byte(serieICS("serie-2", inicio)))

	mail := &correoFalso{buzones: []agenda.Buzon{{ID: "m1", Direccion: "ana@example.org", Nombre: "Ana"}}}
	r := cuentaDAV(t, srv, mail)
	r.Cola = &agenda.ColaIMIP{Dir: filepath.Join(t.TempDir(), "imip")}
	repo := abrirCalendario(t, t.TempDir())
	lc := agenda.LocalCalendario{Repo: repo}
	dir := filepath.Join(t.TempDir(), "agenda")
	if _, errs := r.SincronizarTodo(t.Context(), agenda.Calendario, lc, dir, ""); len(errs) > 0 {
		t.Fatal(errs)
	}
	hora := func(t time.Time) string { return t.In(madrid).Format("20060102T150405") }
	sello := time.Now().Add(-time.Hour)
	ocurrencia := func(asistente, partstat string, sec int, cuando time.Time) []byte {
		rid := "TZID=Europe/Madrid:" + hora(cuando)
		sello = sello.Add(time.Second) // cada una contestada después de la anterior
		b := reply("serie-2", "ana@example.org", asistente, partstat, sec, sello, rid)
		return []byte(strings.Replace(string(b), "RECURRENCE-ID:"+rid, "RECURRENCE-ID;"+rid, 1))
	}
	semana := func(n int) time.Time { return inicio.AddDate(0, 0, 7*n) }
	estados := r.AplicarRespuestas(t.Context(), lc, dir, [][]byte{
		ocurrencia("luis@example.org", "DECLINED", 1, semana(1)),                // 0: crea la excepción
		ocurrencia("luis@example.org", "DECLINED", 1, semana(2)),                // 1: quitada con EXDATE
		ocurrencia("luis@example.org", "DECLINED", 1, semana(3).Add(time.Hour)), // 2: no es una ocurrencia
		ocurrencia("luis@example.org", "DECLINED", 0, semana(3)),                // 3: de una versión anterior
		ocurrencia("marta@example.org", "ACCEPTED", 1, semana(3)),               // 4: ya estaba así
		ocurrencia("pedro@example.org", "ACCEPTED", 1, semana(3)),               // 5: no está invitado
		ocurrencia("marta@example.org", "TENTATIVE", 1, semana(5)),              // 6: después de la última
	})
	quiero := []agenda.EstadoRespuesta{
		agenda.RespuestaAplicada, agenda.RespuestaDescartada, agenda.RespuestaDescartada,
		agenda.RespuestaDescartada, agenda.RespuestaAplicada, agenda.RespuestaDescartada, agenda.RespuestaDescartada,
	}
	for k := range quiero {
		if estados[k] != quiero[k] {
			t.Errorf("respuesta %d: %v, quiero %v", k, estados[k], quiero[k])
		}
	}
	d := objetoPorUID(t, repo, "serie-2").Datos
	eventos := strings.Split(d, "BEGIN:VEVENT")
	if len(eventos) != 3 {
		t.Fatalf("el maestro y UNA excepción: %s", d)
	}
	maestro, ex := eventos[1], eventos[2]
	if strings.Contains(maestro, "DECLINED") || !strings.Contains(maestro, "RRULE:FREQ=WEEKLY;COUNT=5") {
		t.Fatalf("el maestro no cambia: %s", maestro)
	}
	for _, quiero := range []string{
		"RECURRENCE-ID;TZID=Europe/Madrid:" + hora(semana(1)),
		"DTSTART;TZID=Europe/Madrid:" + hora(semana(1)),
		"DTEND;TZID=Europe/Madrid:" + hora(semana(1).Add(90*time.Minute)),
		"SEQUENCE:1", "SUMMARY:Seguimiento semanal", "ORGANIZER;CN=Ana:mailto:ana@example.org",
		"mailto:marta@example.org", "TRIGGER:-PT15M",
	} {
		if !strings.Contains(ex, quiero) {
			t.Errorf("a la excepción le falta %q: %s", quiero, ex)
		}
	}
	if !strings.Contains(ex, "PARTSTAT=DECLINED;RSVP=TRUE:mailto:luis@") && !strings.Contains(ex, "RSVP=TRUE;PARTSTAT=DECLINED") {
		t.Errorf("la respuesta de Luis en su excepción: %s", ex)
	}
	if strings.Contains(ex, "RRULE") || strings.Contains(ex, "EXDATE") {
		t.Errorf("la excepción no se repite: %s", ex)
	}

	// La vuelta la sube y no manda nada: ni correo ni cola.
	res, errs := r.SincronizarTodo(t.Context(), agenda.Calendario, lc, dir, "")
	if len(errs) > 0 || res[0].Subidos != 1 || len(res[0].Invitaciones) != 0 {
		t.Fatalf("subida: %+v %v", res, errs)
	}
	if alla := string(srv.Datos(col + "serie-2.ics")); !strings.Contains(alla, "RECURRENCE-ID;TZID=Europe/Madrid:"+hora(semana(1))) ||
		!strings.Contains(alla, "DECLINED") {
		t.Fatalf("la excepción en el servidor: %s", alla)
	}
	if e := mail.tomar(); len(e) != 0 {
		t.Fatalf("crear la excepción de una respuesta no manda nada a nadie: %v", e)
	}
	if n := r.Cola.Cuantos(); n != 0 {
		t.Fatalf("ni deja nada en la cola: %d", n)
	}

	// Dos respuestas más antes de la vuelta siguiente tampoco mandan nada.
	if e := r.AplicarRespuestas(t.Context(), lc, dir, [][]byte{ocurrencia("marta@example.org", "DECLINED", 1, semana(3))}); e[0] != agenda.RespuestaAplicada {
		t.Fatalf("la cuarta: %v", e)
	}
	if e := r.AplicarRespuestas(t.Context(), lc, dir, [][]byte{ocurrencia("luis@example.org", "ACCEPTED", 1, semana(4))}); e[0] != agenda.RespuestaAplicada {
		t.Fatalf("la quinta: %v", e)
	}
	if res, errs := r.SincronizarTodo(t.Context(), agenda.Calendario, lc, dir, ""); len(errs) > 0 || res[0].Subidos != 1 {
		t.Fatalf("subida de las dos: %+v %v", res, errs)
	}
	if e := mail.tomar(); len(e) != 0 || r.Cola.Cuantos() != 0 {
		t.Fatalf("dos respuestas seguidas no mandan nada: %v", e)
	}

	// Ana cambia la sala: eso sí sale a los invitados, con las excepciones dentro.
	o := objetoPorUID(t, repo, "serie-2")
	if _, err := repo.PonerObjetoCrudo(t.Context(), calDe(t, repo, "serie-2"), o.ID, o.Datos, strings.ReplaceAll(o.Datos, "Sala 2", "Sala 4")); err != nil {
		t.Fatal(err)
	}
	if _, errs := r.SincronizarTodo(t.Context(), agenda.Calendario, lc, dir, ""); len(errs) > 0 {
		t.Fatal(errs)
	}
	e := mail.tomar()
	if len(e) != 1 || !strings.HasPrefix(e[0], "m1→luis@example.org,marta@example.org|") {
		t.Fatalf("el cambio de Ana sale a los invitados: %v", e)
	}
	_, crudo, _ := strings.Cut(e[0], "|")
	_, metodo, cal, _ := partes(t, crudo)
	if metodo != "REQUEST" || !strings.Contains(cal, "Sala 4") || strings.Count(cal, "RECURRENCE-ID") != 3 {
		t.Fatalf("el cambio de Ana sale con las excepciones: %s %s", metodo, cal)
	}
}
