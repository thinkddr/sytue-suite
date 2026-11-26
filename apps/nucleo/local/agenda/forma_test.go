// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateCalendar "git.sytue.eu/sytue/one/apps/nucleo/calendar"
	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/dav/davprueba"
	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
)

// comoSytueOffice abre el calendario de este equipo como lo abre Sytue Office
// (`cmd-local/calendar.nuevoRepositorio`).
func comoSytueOffice(t *testing.T) *privateCalendar.Repository {
	t.Helper()
	repo := abrirCalendario(t, t.TempDir())
	repo.ConservarRespuestas, repo.ConservarForma, repo.SerieALasExcepciones = true, true, true
	return repo
}

// lineaDe es la primera línea de d que empieza por prefijo, o "".
func lineaDe(d, prefijo string) string {
	for _, l := range strings.Split(strings.ReplaceAll(d, "\r\n ", ""), "\r\n") {
		if strings.HasPrefix(l, prefijo) {
			return l
		}
	}
	return ""
}

// Un evento que vino de la cuenta escrito a su manera —la EXDATE en la hora de Madrid,
// sin STATUS, el recordatorio con su texto— y se guarda desde el formulario sin cambiar
// nada sube como estaba: ni sale la invitación, ni va a la cola, ni sube la SEQUENCE, y lo
// no tocado tiene el mismo texto aquí y en el servidor. Un cambio de verdad sí sale.
func TestGuardarSinCambiosUnEventoDeLaCuentaNoMandaNada(t *testing.T) {
	srv := davprueba.Nuevo()
	defer srv.Close()
	col := davprueba.CasaCal + "personal/"
	srv.Crear(col, "Personal", false)
	hoy := time.Now().In(madridDe(t)).AddDate(0, 0, 2)
	inicio := time.Date(hoy.Year(), hoy.Month(), hoy.Day(), 9, 30, 0, 0, madridDe(t))
	original := serieICS("serie-4", inicio)
	srv.Poner(col+"serie-4.ics", []byte(original))
	mail := &correoFalso{buzones: []agenda.Buzon{{ID: "m1", Direccion: "ana@example.org", Nombre: "Ana"}}}
	r := cuentaDAV(t, srv, mail)
	r.Cola = &agenda.ColaIMIP{Dir: filepath.Join(t.TempDir(), "imip")}
	repo := comoSytueOffice(t)
	lc := agenda.LocalCalendario{Repo: repo}
	dir := filepath.Join(t.TempDir(), "agenda")
	vuelta := func() agenda.Resultado {
		t.Helper()
		res, errs := r.SincronizarTodo(t.Context(), agenda.Calendario, lc, dir, "")
		if len(errs) > 0 {
			t.Fatal(errs)
		}
		return res[0]
	}
	vuelta()

	// El formulario: el evento entero con lo que leyó, y sus recordatorios.
	ev := eventoPorUID(t, repo, "serie-4")
	in := ev.Input()
	in.Reminders = ev.Reminders
	if _, err := repo.UpdateEvent(t.Context(), ev.ID, in); err != nil {
		t.Fatal(err)
	}
	aqui := objetoPorUID(t, repo, "serie-4").Datos
	for _, d := range []string{aqui} {
		for _, p := range []string{"EXDATE", "SEQUENCE", "DTSTART", "RRULE", "ORGANIZER", "ATTENDEE;CN=Luis", "DESCRIPTION", "TRIGGER"} {
			if lineaDe(d, p) != lineaDe(original, p) {
				t.Errorf("%s cambia: %q → %q", p, lineaDe(original, p), lineaDe(d, p))
			}
		}
		if strings.Contains(d, "STATUS") || strings.Contains(d, "CLASS") || strings.Contains(d, "TRANSP") {
			t.Errorf("sin STATUS, CLASS ni TRANSP, que no traía:\n%s", d)
		}
	}
	res := vuelta()
	if res.Subidos != 1 || len(res.Invitaciones) != 0 {
		t.Fatalf("la vuelta: %+v", res)
	}
	if e := mail.tomar(); len(e) != 0 || r.Cola.Cuantos() != 0 {
		t.Fatalf("guardar sin cambios no manda nada: %v", e)
	}
	alla := string(srv.Datos(col + "serie-4.ics"))
	for _, p := range []string{"EXDATE", "SEQUENCE", "DTSTART"} {
		if lineaDe(alla, p) != lineaDe(original, p) {
			t.Errorf("en el servidor %s cambia: %q → %q", p, lineaDe(original, p), lineaDe(alla, p))
		}
	}

	// Un cambio de verdad (el lugar) sí sale, y sube la SEQUENCE; lo demás, como estaba.
	ev = eventoPorUID(t, repo, "serie-4")
	in = ev.Input()
	in.Location = "Sala 4"
	if _, err := repo.UpdateEvent(t.Context(), ev.ID, in); err != nil {
		t.Fatal(err)
	}
	vuelta()
	if e := mail.tomar(); len(e) != 1 {
		t.Fatalf("el cambio de lugar sale a los invitados: %v", e)
	}
	alla = string(srv.Datos(col + "serie-4.ics"))
	if lineaDe(alla, "SEQUENCE") != "SEQUENCE:2" || lineaDe(alla, "LOCATION") != "LOCATION:Sala 4" || lineaDe(alla, "EXDATE") != lineaDe(original, "EXDATE") {
		t.Fatalf("en el servidor: %s", alla)
	}
}

func madridDe(t *testing.T) *time.Location {
	t.Helper()
	l, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// «Solo este» con una respuesta que llega por correo mientras el formulario está
// abierto: lo que sube es el evento nuevo con la respuesta de Luis a esa ocurrencia y el
// organizador, y la serie sin esa ocurrencia ni la excepción que creó la respuesta.
func TestPartirLaSerieSubeLoContestadoEntreMedias(t *testing.T) {
	srv := davprueba.Nuevo()
	defer srv.Close()
	col := davprueba.CasaCal + "personal/"
	srv.Crear(col, "Personal", false)
	madrid := madridDe(t)
	hoy := time.Now().In(madrid).AddDate(0, 0, 2)
	inicio := time.Date(hoy.Year(), hoy.Month(), hoy.Day(), 9, 30, 0, 0, madrid)
	srv.Poner(col+"serie-5.ics", []byte(serieICS("serie-5", inicio)))
	mail := &correoFalso{buzones: []agenda.Buzon{{ID: "m1", Direccion: "ana@example.org", Nombre: "Ana"}}}
	r := cuentaDAV(t, srv, mail)
	r.Cola = &agenda.ColaIMIP{Dir: filepath.Join(t.TempDir(), "imip")}
	repo := comoSytueOffice(t)
	lc := agenda.LocalCalendario{Repo: repo}
	dir := filepath.Join(t.TempDir(), "agenda")
	if _, errs := r.SincronizarTodo(t.Context(), agenda.Calendario, lc, dir, ""); len(errs) > 0 {
		t.Fatal(errs)
	}

	// Ana abre la segunda: lo que lee el formulario («solo este» manda el contenido).
	semana := inicio.AddDate(0, 0, 7)
	cal := calDe(t, repo, "serie-5")
	is, err := repo.Instances(t.Context(), cal, semana.Add(-time.Hour), semana.Add(time.Hour))
	if err != nil || len(is) != 1 {
		t.Fatal(is, err)
	}
	o := is[0]
	in := api.EventInput{Summary: "Seguimiento (en la sala grande)", Location: o.Event.Location, Start: o.Start, End: o.End, TZID: o.Event.TZID, Attendees: o.Event.Attendees}

	// Luis declina esa ocurrencia por correo.
	rid := "TZID=Europe/Madrid:" + semana.Format("20060102T150405")
	ocurrencia := []byte(strings.Replace(string(reply("serie-5", "ana@example.org", "luis@example.org", "DECLINED", 1, time.Now().Add(-time.Minute), rid)), "RECURRENCE-ID:"+rid, "RECURRENCE-ID;"+rid, 1))
	if e := r.AplicarRespuestas(t.Context(), lc, dir, [][]byte{ocurrencia}); e[0] != agenda.RespuestaAplicada {
		t.Fatalf("la respuesta de Luis: %v", e)
	}

	// Ana guarda «solo este».
	if _, err := repo.DividirSerie(t.Context(), cal, "serie-5", o.Start, api.RecorteExcluir, in); err != nil {
		t.Fatal(err)
	}
	res, errs := r.SincronizarTodo(t.Context(), agenda.Calendario, lc, dir, "")
	if len(errs) > 0 || res[0].Subidos != 2 {
		t.Fatalf("la vuelta sube la serie y el evento nuevo: %+v %v", res, errs)
	}
	rutas := srv.Rutas(col)
	if len(rutas) != 2 {
		t.Fatalf("en el servidor: %v", rutas)
	}
	for _, ruta := range rutas {
		d := string(srv.Datos(ruta))
		if strings.Contains(d, "UID:serie-5") {
			if strings.Contains(d, "RECURRENCE-ID") || lineaDe(d, "EXDATE;TZID=Europe/Madrid:"+semana.Format("20060102T150405")) == "" {
				t.Fatalf("la serie, sin la segunda ni su excepción: %s", d)
			}
			continue
		}
		if estadoDe(d, "luis@example.org") != "DECLINED" || estadoDe(d, "marta@example.org") != "ACCEPTED" ||
			lineaDe(d, "ORGANIZER") != "ORGANIZER;CN=Ana:mailto:ana@example.org" || !strings.Contains(d, "Seguimiento (en la sala grande)") {
			t.Fatalf("el evento nuevo, con la respuesta de Luis y el organizador: %s", d)
		}
	}
}
