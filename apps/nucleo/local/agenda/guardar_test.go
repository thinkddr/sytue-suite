// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda_test

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateCalendar "git.sytue.eu/sytue/one/apps/nucleo/calendar"
	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/dav/davprueba"
	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
)

// eventoPorUID es el maestro de un evento como lo lee el formulario al abrirse.
func eventoPorUID(t *testing.T, repo *privateCalendar.Repository, uid string) api.Event {
	t.Helper()
	cal := calDe(t, repo, uid)
	is, err := repo.Instances(t.Context(), cal, time.Now().AddDate(0, 0, -1), time.Now().AddDate(0, 2, 0))
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range is {
		if i.Event.UID == uid && i.Event.RecurrenceID == nil {
			ev, err := repo.GetEvent(t.Context(), i.Event.ID)
			if err != nil {
				t.Fatal(err)
			}
			return ev
		}
	}
	t.Fatalf("no está %s", uid)
	return api.Event{}
}

// estadoDe es el PARTSTAT de quien en un VEVENT (un trozo tras «BEGIN:VEVENT»); "" si no
// está invitado.
func estadoDe(vevent, quien string) string {
	for _, l := range strings.Split(strings.ReplaceAll(vevent, "\r\n ", ""), "\r\n") {
		if !strings.HasPrefix(l, "ATTENDEE") || !strings.HasSuffix(strings.ToLower(l), "mailto:"+quien) {
			continue
		}
		for _, p := range strings.Split(strings.SplitN(l, ":", 2)[0], ";") {
			if v, ok := strings.CutPrefix(p, "PARTSTAT="); ok {
				return v
			}
		}
		return "NEEDS-ACTION"
	}
	return ""
}

// sinFormulario quita de lo que se manda a quien la persona quita en el formulario.
func sinInvitado(in api.EventInput, quien string) api.EventInput {
	var quedan []api.Attendee
	for _, a := range in.Attendees {
		if !strings.EqualFold(a.Email, quien) {
			quedan = append(quedan, a)
		}
	}
	in.Attendees = quedan
	return in
}

// La respuesta de un invitado que llega por correo mientras la persona tiene abierto el
// formulario de ese evento no se pierde al guardar: lo que guarda se hizo con lo que leyó
// al abrirlo, pero lo que contestaron entre medias se queda (aquí y, tras la vuelta, en el
// servidor), y conservarlo no manda ninguna invitación ni deja nada en la cola. Lo que
// cambia la persona sí sale, y lleva las respuestas. Si quita a ese invitado, no vuelve
// (ni porque llegue otra vez su respuesta). Y guardar algo que los invitados no ven (un
// recordatorio) no sube la SEQUENCE: la respuesta que se aplica DESPUÉS de guardar sigue
// siendo de la versión de ahora.
func TestGuardarElFormularioNoPisaLaRespuestaQueLlegaMientrasTanto(t *testing.T) {
	srv := davprueba.Nuevo()
	defer srv.Close()
	col := davprueba.CasaCal + "personal/"
	srv.Crear(col, "Personal", false)
	inicio := time.Now().AddDate(0, 0, 7).Truncate(time.Hour)
	// Con STATUS: guardar desde el formulario lo escribe siempre, y aquí no tiene que
	// cambiar nada de lo que ven los invitados.
	reunion := strings.NewReplacer("%INICIO%", inicio.Format("20060102T150405"), "%FIN%", inicio.Add(time.Hour).Format("20060102T150405"),
		"LOCATION:Sala 2\r\n", "LOCATION:Sala 2\r\nSTATUS:CONFIRMED\r\n").Replace(reunionICS)
	srv.Poner(col+"reunion-1.ics", []byte(reunion))
	mail := &correoFalso{buzones: []agenda.Buzon{{ID: "m1", Direccion: "ana@example.org", Nombre: "Ana"}}}
	r := cuentaDAV(t, srv, mail)
	r.Cola = &agenda.ColaIMIP{Dir: filepath.Join(t.TempDir(), "imip")}
	repo := abrirCalendario(t, t.TempDir())
	repo.ConservarRespuestas = true // como lo abre Sytue Office (cmd-local/calendar)
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
	sello := time.Now().Add(-time.Hour)
	// secuencia es la SEQUENCE de lo que hay aquí: la de la última invitación que salió.
	secuencia := func() int {
		t.Helper()
		_, v, _ := strings.Cut(objetoPorUID(t, repo, "reunion-1").Datos, "\r\nSEQUENCE")
		v, _, _ = strings.Cut(v, "\r\n")
		_, v, _ = strings.Cut(v, ":")
		n, _ := strconv.Atoi(v)
		return n
	}
	// contesta aplica la respuesta de quien a la invitación de SEQUENCE sec.
	contesta := func(quien, partstat string, sec int) agenda.EstadoRespuesta {
		t.Helper()
		sello = sello.Add(time.Minute)
		return r.AplicarRespuestas(t.Context(), lc, dir, [][]byte{reply("reunion-1", "ana@example.org", quien, partstat, sec, sello, "")})[0]
	}
	guardar := func(in api.EventInput) {
		t.Helper()
		if _, err := repo.UpdateEvent(t.Context(), eventoPorUID(t, repo, "reunion-1").ID, in); err != nil {
			t.Fatal(err)
		}
	}
	recordatorio := func(in api.EventInput, minutos int) api.EventInput {
		in.Reminders = &[]int{minutos}
		return in
	}

	// 1. Ana abre el formulario; Luis acepta; Ana cambia el recordatorio y guarda.
	in := eventoPorUID(t, repo, "reunion-1").Input()
	if e := contesta("luis@example.org", "ACCEPTED", secuencia()); e != agenda.RespuestaAplicada {
		t.Fatalf("la respuesta de Luis: %v", e)
	}
	guardar(recordatorio(in, 30))
	aqui := objetoPorUID(t, repo, "reunion-1").Datos
	if estadoDe(aqui, "luis@example.org") != "ACCEPTED" || !strings.Contains(aqui, "TRIGGER:-PT30M") {
		t.Fatalf("guardar conserva la respuesta de Luis y pone el recordatorio: %s", aqui)
	}
	if res := vuelta(); res.Subidos != 1 || len(res.Invitaciones) != 0 {
		t.Fatalf("la vuelta: %+v", res)
	}
	if alla := string(srv.Datos(col + "reunion-1.ics")); estadoDe(alla, "luis@example.org") != "ACCEPTED" {
		t.Fatalf("en el servidor: %s", alla)
	}
	if e := mail.tomar(); len(e) != 0 || r.Cola.Cuantos() != 0 {
		t.Fatalf("conservar una respuesta no manda nada a nadie: %v", e)
	}

	// 2. Otra vez: Marta declina mientras Ana cambia el título. Su cambio sale a los
	//    invitados, con las respuestas de los dos.
	in = eventoPorUID(t, repo, "reunion-1").Input()
	if e := contesta("marta@example.org", "DECLINED", secuencia()); e != agenda.RespuestaAplicada {
		t.Fatalf("la respuesta de Marta: %v", e)
	}
	in.Summary = "Revisión del presupuesto (2.ª)"
	guardar(in)
	vuelta()
	e := mail.tomar()
	if len(e) != 1 {
		t.Fatalf("el cambio de Ana sale: %v", e)
	}
	para, crudo, _ := strings.Cut(e[0], "|")
	if quienes := strings.Split(strings.TrimPrefix(para, "m1→"), ","); len(quienes) != 2 || !slices.Contains(quienes, "luis@example.org") || !slices.Contains(quienes, "marta@example.org") {
		t.Fatalf("a Luis y a Marta: %s", para)
	}
	if _, metodo, cal, _ := partes(t, crudo); metodo != "REQUEST" || !strings.Contains(cal, "(2.ª)") ||
		estadoDe(cal, "marta@example.org") != "DECLINED" || estadoDe(cal, "luis@example.org") != "ACCEPTED" {
		t.Fatalf("con las respuestas que llegaron: %s %s", metodo, cal)
	}

	// 3. Luis cambia de idea mientras Ana lo quita de la lista: gana Ana, y su respuesta
	//    otra vez no lo devuelve.
	in = eventoPorUID(t, repo, "reunion-1").Input()
	if e := contesta("luis@example.org", "TENTATIVE", secuencia()); e != agenda.RespuestaAplicada {
		t.Fatalf("Luis otra vez: %v", e)
	}
	guardar(sinInvitado(in, "luis@example.org"))
	if d := objetoPorUID(t, repo, "reunion-1").Datos; strings.Contains(d, "luis@") || estadoDe(d, "marta@example.org") != "DECLINED" {
		t.Fatalf("Luis quitado, Marta igual: %s", d)
	}
	vuelta()
	cancelado := false
	for _, x := range mail.tomar() {
		if para, crudo, _ := strings.Cut(x, "|"); para == "m1→luis@example.org" {
			_, metodo, _, _ := partes(t, crudo)
			cancelado = metodo == "CANCEL"
		}
	}
	if !cancelado {
		t.Fatal("a Luis le llega la cancelación")
	}
	if e := contesta("luis@example.org", "ACCEPTED", secuencia()); e != agenda.RespuestaDescartada {
		t.Fatalf("la respuesta de quien ya no está: %v", e)
	}
	if d := objetoPorUID(t, repo, "reunion-1").Datos + string(srv.Datos(col+"reunion-1.ics")); strings.Contains(d, "luis@") {
		t.Fatalf("Luis no vuelve: %s", d)
	}

	// 4. Ana cambia solo el recordatorio y la respuesta de Marta a la invitación de ahora
	//    (que estaba de camino) se aplica DESPUÉS: sigue valiendo.
	enviada := secuencia()
	guardar(recordatorio(eventoPorUID(t, repo, "reunion-1").Input(), 10))
	if despues := objetoPorUID(t, repo, "reunion-1").Datos; enviada == 0 || secuencia() != enviada || !strings.Contains(despues, "TRIGGER:-PT10M") {
		t.Fatalf("un recordatorio no sube la SEQUENCE (%d → %d): %s", enviada, secuencia(), despues)
	}
	if e := contesta("marta@example.org", "ACCEPTED", enviada); e != agenda.RespuestaAplicada {
		t.Fatalf("la respuesta que llega tras guardar un recordatorio: %v", e)
	}
	vuelta()
	if e := mail.tomar(); len(e) != 0 || r.Cola.Cuantos() != 0 {
		t.Fatalf("ni el recordatorio ni la respuesta mandan nada: %v", e)
	}
	if alla := string(srv.Datos(col + "reunion-1.ics")); estadoDe(alla, "marta@example.org") != "ACCEPTED" || !strings.Contains(alla, "(2.ª)") {
		t.Fatalf("en el servidor: %s", alla)
	}
}

// Lo mismo con una serie: mientras la persona tiene abierta la serie («todos»), Marta
// contesta a la serie entera y Luis a UNA ocurrencia (se crea su excepción). Guardar
// conserva las dos y no manda nada; quitar después a Luis de la serie lo quita también de
// esa ocurrencia.
func TestGuardarLaSerieNoPisaLasRespuestasNiSusExcepciones(t *testing.T) {
	srv := davprueba.Nuevo()
	defer srv.Close()
	col := davprueba.CasaCal + "personal/"
	srv.Crear(col, "Personal", false)
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Fatal(err)
	}
	hoy := time.Now().In(madrid).AddDate(0, 0, 2)
	inicio := time.Date(hoy.Year(), hoy.Month(), hoy.Day(), 9, 30, 0, 0, madrid)
	// Sin EXDATE y con STATUS: el formulario escribe la EXDATE en UTC y siempre pone el
	// STATUS, y aquí guardar no tiene que cambiar nada de lo que ven los invitados.
	serie := strings.Replace(serieICS("serie-3", inicio), "LOCATION:Sala 2\r\n", "LOCATION:Sala 2\r\nSTATUS:CONFIRMED\r\n", 1)
	serie = strings.Replace(serie, "EXDATE;TZID=Europe/Madrid:"+inicio.AddDate(0, 0, 14).Format("20060102T150405")+"\r\n", "", 1)
	srv.Poner(col+"serie-3.ics", []byte(serie))
	mail := &correoFalso{buzones: []agenda.Buzon{{ID: "m1", Direccion: "ana@example.org", Nombre: "Ana"}}}
	r := cuentaDAV(t, srv, mail)
	r.Cola = &agenda.ColaIMIP{Dir: filepath.Join(t.TempDir(), "imip")}
	repo := abrirCalendario(t, t.TempDir())
	repo.ConservarRespuestas = true
	lc := agenda.LocalCalendario{Repo: repo}
	dir := filepath.Join(t.TempDir(), "agenda")
	if _, errs := r.SincronizarTodo(t.Context(), agenda.Calendario, lc, dir, ""); len(errs) > 0 {
		t.Fatal(errs)
	}
	semana := inicio.AddDate(0, 0, 7)
	rid := "TZID=Europe/Madrid:" + semana.Format("20060102T150405")
	sello := time.Now().Add(-time.Hour)
	ocurrencia := []byte(strings.Replace(string(reply("serie-3", "ana@example.org", "luis@example.org", "DECLINED", 1, sello, rid)), "RECURRENCE-ID:"+rid, "RECURRENCE-ID;"+rid, 1))

	maestro := eventoPorUID(t, repo, "serie-3")
	in := maestro.Input()
	estados := r.AplicarRespuestas(t.Context(), lc, dir, [][]byte{ocurrencia, reply("serie-3", "ana@example.org", "marta@example.org", "TENTATIVE", 1, sello.Add(time.Second), "")})
	if estados[0] != agenda.RespuestaAplicada || estados[1] != agenda.RespuestaAplicada {
		t.Fatalf("las respuestas: %v", estados)
	}
	in.Reminders = &[]int{5}
	if _, err := repo.UpdateEvent(t.Context(), maestro.ID, in); err != nil {
		t.Fatal(err)
	}
	d := objetoPorUID(t, repo, "serie-3").Datos
	eventos := strings.Split(d, "BEGIN:VEVENT")
	if len(eventos) != 3 {
		t.Fatalf("el maestro y la excepción de la respuesta de Luis: %s", d)
	}
	m, ex := eventos[1], eventos[2]
	if strings.Contains(m, "RECURRENCE-ID") {
		m, ex = ex, m
	}
	if estadoDe(m, "marta@example.org") != "TENTATIVE" || estadoDe(m, "luis@example.org") != "NEEDS-ACTION" || !strings.Contains(m, "TRIGGER:-PT5M") {
		t.Fatalf("el maestro con la respuesta de Marta y el recordatorio: %s", m)
	}
	if !strings.Contains(ex, "RECURRENCE-ID;"+rid) || estadoDe(ex, "luis@example.org") != "DECLINED" || estadoDe(ex, "marta@example.org") != "TENTATIVE" {
		t.Fatalf("la excepción con las dos respuestas: %s", ex)
	}
	res, errs := r.SincronizarTodo(t.Context(), agenda.Calendario, lc, dir, "")
	if len(errs) > 0 || res[0].Subidos != 1 {
		t.Fatalf("la vuelta: %+v %v", res, errs)
	}
	if e := mail.tomar(); len(e) != 0 || r.Cola.Cuantos() != 0 {
		t.Fatalf("guardar no manda nada por conservar las respuestas: %v", e)
	}

	// Ana quita a Luis de la serie: tampoco está en la ocurrencia que contestó.
	if _, err := repo.UpdateEvent(t.Context(), maestro.ID, sinInvitado(eventoPorUID(t, repo, "serie-3").Input(), "luis@example.org")); err != nil {
		t.Fatal(err)
	}
	if d := objetoPorUID(t, repo, "serie-3").Datos; strings.Contains(d, "luis@") || strings.Count(d, "RECURRENCE-ID") != 1 || strings.Count(d, "TENTATIVE") != 2 {
		t.Fatalf("Luis fuera de toda la serie: %s", d)
	}
}
