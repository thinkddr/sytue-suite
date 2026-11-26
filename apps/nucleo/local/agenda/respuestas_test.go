// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda_test

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	privateCalendar "git.sytue.eu/sytue/one/apps/nucleo/calendar"
	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/dav/davprueba"
	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
	"git.sytue.eu/sytue/one/apps/nucleo/local/correo"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
	"git.sytue.eu/sytue/one/apps/nucleo/local/secretos"
)

// reply es un VCALENDAR METHOD:REPLY de asistente con partstat a la reunión uid que
// organiza organizador, con su secuencia, su sello y, si no es "", la ocurrencia rid.
func reply(uid, organizador, asistente, partstat string, secuencia int, sello time.Time, rid string) []byte {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//prueba//ES\r\nMETHOD:REPLY\r\nBEGIN:VEVENT\r\nUID:" + uid + "\r\n")
	if rid != "" {
		b.WriteString("RECURRENCE-ID:" + rid + "\r\n")
	}
	b.WriteString("SEQUENCE:" + string(rune('0'+secuencia)) + "\r\nDTSTAMP:" + sello.UTC().Format("20060102T150405Z") + "\r\n")
	b.WriteString("ORGANIZER:mailto:" + organizador + "\r\nATTENDEE;PARTSTAT=" + partstat + ":mailto:" + asistente + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
	return []byte(b.String())
}

// cuentaDAV da de alta una cuenta CalDAV en srv y devuelve su registro, con unas cuentas
// de Mail de mentira con la dirección de la persona.
func cuentaDAV(t *testing.T, srv *davprueba.Servidor, mail agenda.Correo) *agenda.Registro {
	t.Helper()
	ajustes := t.TempDir()
	r := agenda.AbrirEn(filepath.Join(ajustes, "agenda.json"), secretos.SoloFichero(filepath.Join(ajustes, "secretos")), nil)
	r.HTTP = func(red.Proposito) *http.Client { return srv.Client() }
	r.Correo = mail
	if _, err := r.AnadirDAV(t.Context(), agenda.Alta{Rotulo: "Nextcloud", Usuario: srv.Usuario, Clave: srv.Clave, Calendario: srv.URL + "/cal/"}); err != nil {
		t.Fatal(err)
	}
	return r
}

func objetoPorUID(t *testing.T, repo *privateCalendar.Repository, uid string) privateCalendar.ObjetoCrudo {
	t.Helper()
	cals, err := repo.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cals {
		lista, _ := repo.ObjetosCrudos(t.Context(), c.ID)
		for _, o := range lista {
			if o.UID == uid {
				return o
			}
		}
	}
	t.Fatalf("no está %s", uid)
	return privateCalendar.ObjetoCrudo{}
}

// A-11: las respuestas de los invitados que llegan por correo a lo que organiza la persona
// en un CalDAV que no planifica se ponen en el evento (el PARTSTAT del invitado) y suben
// con la sincronización, sin mandar nada a nadie. Se aplican por su DTSTAMP; una vieja,
// repetida, de otra versión, de quien no está invitado o a lo que organiza otro no se
// aplica; la de una ocurrencia va a su excepción, y sin ella la crea (ocurrencia_test.go);
// la de un evento que aún no está aquí espera; con un servidor que planifica, se descarta
// (la aplica él).
func TestLasRespuestasPorCorreoSeAplicanAlEvento(t *testing.T) {
	srv := davprueba.Nuevo()
	defer srv.Close()
	col := davprueba.CasaCal + "personal/"
	srv.Crear(col, "Personal", false)
	inicio := time.Now().AddDate(0, 0, 7).Truncate(time.Hour)
	reunion := strings.NewReplacer("%INICIO%", inicio.Format("20060102T150405"), "%FIN%", inicio.Add(time.Hour).Format("20060102T150405")).Replace(reunionICS)
	srv.Poner(col+"reunion-1.ics", []byte(reunion))
	ajena := strings.NewReplacer("UID:reunion-1", "UID:reunion-2", "ORGANIZER;CN=Ana:mailto:ana@example.org", "ORGANIZER;CN=Pedro:mailto:pedro@example.org").Replace(reunion)
	srv.Poner(col+"reunion-2.ics", []byte(ajena))
	// Una serie semanal con la ocurrencia de la semana siguiente cambiada.
	semana := inicio.AddDate(0, 0, 7)
	serie := strings.NewReplacer("UID:reunion-1", "UID:serie-1", "LOCATION:Sala 2\r\n", "LOCATION:Sala 2\r\nRRULE:FREQ=WEEKLY;COUNT=5\r\n").Replace(reunion)
	excepcion := strings.NewReplacer("UID:reunion-1", "UID:serie-1",
		"DTSTART;TZID=Europe/Madrid:"+inicio.Format("20060102T150405"), "RECURRENCE-ID;TZID=Europe/Madrid:"+semana.Format("20060102T150405")+"\r\nDTSTART;TZID=Europe/Madrid:"+semana.Add(time.Hour).Format("20060102T150405"),
		"DTEND;TZID=Europe/Madrid:"+inicio.Add(time.Hour).Format("20060102T150405"), "DTEND;TZID=Europe/Madrid:"+semana.Add(2*time.Hour).Format("20060102T150405")).Replace(reunion)
	i := strings.Index(excepcion, "BEGIN:VEVENT")
	j := strings.Index(excepcion, "END:VCALENDAR")
	serie = strings.Replace(serie, "END:VCALENDAR", excepcion[i:j]+"END:VCALENDAR", 1)
	srv.Poner(col+"serie-1.ics", []byte(serie))

	mail := &correoFalso{buzones: []agenda.Buzon{{ID: "m1", Direccion: "ana@example.org", Nombre: "Ana"}}}
	r := cuentaDAV(t, srv, mail)
	repo := abrirCalendario(t, t.TempDir())
	lc := agenda.LocalCalendario{Repo: repo}
	dir := filepath.Join(t.TempDir(), "agenda")
	if _, errs := r.SincronizarTodo(t.Context(), agenda.Calendario, lc, dir, ""); len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(mail.tomar()) != 0 {
		t.Fatal("bajar no manda nada")
	}

	t0 := time.Now().Add(-2 * time.Hour)
	t1 := t0.Add(time.Hour)
	rid := "TZID=Europe/Madrid:" + semana.Format("20060102T150405")
	ridSin := "TZID=Europe/Madrid:" + semana.AddDate(0, 0, 7).Format("20060102T150405")
	conRID := func(b []byte, rid string) []byte {
		return []byte(strings.Replace(string(b), "RECURRENCE-ID:"+rid, "RECURRENCE-ID;"+rid, 1))
	}
	lote := [][]byte{
		reply("reunion-1", "ana@example.org", "luis@example.org", "ACCEPTED", 0, t1, ""),                    // 0: se aplica (tras la 1)
		reply("reunion-1", "ana@example.org", "luis@example.org", "TENTATIVE", 0, t0, ""),                   // 1: más vieja: va primero
		reply("reunion-1", "ana@example.org", "marta@example.org", "DECLINED", 0, t1, ""),                   // 2: se aplica
		reply("reunion-1", "ana@example.org", "pedro@example.org", "ACCEPTED", 0, t1, ""),                   // 3: no está invitado
		reply("reunion-2", "pedro@example.org", "luis@example.org", "ACCEPTED", 0, t1, ""),                  // 4: organiza otro
		reply("no-esta", "ana@example.org", "luis@example.org", "ACCEPTED", 0, t1, ""),                      // 5: espera
		[]byte("esto no es un calendario"),                                                                  // 6
		conRID(reply("serie-1", "ana@example.org", "luis@example.org", "DECLINED", 0, t1, rid), rid),        // 7: su excepción
		conRID(reply("serie-1", "ana@example.org", "marta@example.org", "DECLINED", 0, t1, ridSin), ridSin), // 8: crea su excepción
	}
	quiero := []agenda.EstadoRespuesta{
		agenda.RespuestaAplicada, agenda.RespuestaAplicada, agenda.RespuestaAplicada, agenda.RespuestaDescartada,
		agenda.RespuestaDescartada, agenda.RespuestaEspera, agenda.RespuestaDescartada, agenda.RespuestaAplicada, agenda.RespuestaAplicada,
	}
	estados := r.AplicarRespuestas(t.Context(), lc, dir, lote)
	for k := range quiero {
		if estados[k] != quiero[k] {
			t.Errorf("respuesta %d: %v, quiero %v", k, estados[k], quiero[k])
		}
	}
	d := objetoPorUID(t, repo, "reunion-1").Datos
	if !strings.Contains(d, "PARTSTAT=ACCEPTED;RSVP=TRUE:mailto:luis@example.org") && !strings.Contains(d, "RSVP=TRUE;PARTSTAT=ACCEPTED") ||
		!strings.Contains(d, "PARTSTAT=DECLINED:mailto:marta@example.org") || strings.Contains(d, "pedro@") {
		t.Fatalf("la reunión con las respuestas: %s", d)
	}
	if d := objetoPorUID(t, repo, "reunion-2").Datos; strings.Contains(d, "PARTSTAT=ACCEPTED;RSVP=TRUE:mailto:luis") {
		t.Fatalf("lo que organiza otro no cambia: %s", d)
	}
	s := objetoPorUID(t, repo, "serie-1").Datos
	partes := strings.Split(s, "BEGIN:VEVENT")
	if len(partes) != 4 || strings.Contains(partes[1], "DECLINED") || !strings.Contains(partes[2], "PARTSTAT=DECLINED;RSVP=TRUE:mailto:luis@") ||
		!strings.Contains(partes[3], "RECURRENCE-ID;"+ridSin) || !strings.Contains(partes[3], "PARTSTAT=DECLINED:mailto:marta@") {
		t.Fatalf("la respuesta a una ocurrencia va a su excepción, y si no la hay, a una nueva: %s", s)
	}

	// La sincronización la sube sin mandar ningún correo.
	if res, errs := r.SincronizarTodo(t.Context(), agenda.Calendario, lc, dir, ""); len(errs) > 0 || res[0].Subidos != 2 {
		t.Fatalf("subida: %+v %v", res, errs)
	}
	if alla := string(srv.Datos(col + "reunion-1.ics")); !strings.Contains(alla, "DECLINED:mailto:marta@example.org") {
		t.Fatalf("en el servidor: %s", alla)
	}
	if e := mail.tomar(); len(e) != 0 {
		t.Fatalf("aplicar una respuesta no manda nada a nadie: %v", e)
	}

	// La misma otra vez, o una más vieja que la aplicada: nada.
	if e := r.AplicarRespuestas(t.Context(), lc, dir, [][]byte{lote[0], reply("reunion-1", "ana@example.org", "luis@example.org", "DECLINED", 0, t0.Add(time.Minute), "")}); e[0] != agenda.RespuestaDescartada || e[1] != agenda.RespuestaDescartada {
		t.Fatalf("repetida o vieja: %v", e)
	}
	// De una versión anterior del evento: no.
	o := objetoPorUID(t, repo, "reunion-1")
	if _, err := repo.PonerObjetoCrudo(t.Context(), calDe(t, repo, "reunion-1"), o.ID, "", strings.Replace(o.Datos, "SUMMARY:", "SEQUENCE:3\r\nSUMMARY:", 1)); err != nil {
		t.Fatal(err)
	}
	if e := r.AplicarRespuestas(t.Context(), lc, dir, [][]byte{reply("reunion-1", "ana@example.org", "marta@example.org", "ACCEPTED", 2, time.Now(), "")}); e[0] != agenda.RespuestaDescartada {
		t.Fatalf("de otra versión: %v", e)
	}

	// Con un servidor que planifica, la respuesta la aplica él.
	srv.Planifica = true
	if _, errs := r.SincronizarTodo(t.Context(), agenda.Calendario, lc, dir, ""); len(errs) > 0 {
		t.Fatal(errs)
	}
	if e := r.AplicarRespuestas(t.Context(), lc, dir, [][]byte{reply("reunion-1", "ana@example.org", "marta@example.org", "ACCEPTED", 3, time.Now(), "")}); e[0] != agenda.RespuestaDescartada {
		t.Fatalf("con planificación: %v", e)
	}
}

// calDe es el calendario de aquí de un evento.
func calDe(t *testing.T, repo *privateCalendar.Repository, uid string) string {
	t.Helper()
	cals, _ := repo.List(t.Context())
	for _, c := range cals {
		lista, _ := repo.ObjetosCrudos(t.Context(), c.ID)
		for _, o := range lista {
			if o.UID == uid {
				return c.ID
			}
		}
	}
	return ""
}

// Con -race: aplicar las respuestas mientras la persona edita el mismo evento no pisa lo
// que ella escribe (si cambió entre medias, la respuesta espera a la vez siguiente), y lo
// que ella escribe con lo que leyó tampoco pisa una respuesta ya puesta.
func TestAplicarRespuestasMientrasSeEdita(t *testing.T) {
	srv := davprueba.Nuevo()
	defer srv.Close()
	col := davprueba.CasaCal + "personal/"
	srv.Crear(col, "Personal", false)
	inicio := time.Now().AddDate(0, 0, 3).Truncate(time.Hour)
	srv.Poner(col+"reunion-1.ics", []byte(strings.NewReplacer("%INICIO%", inicio.Format("20060102T150405"), "%FIN%", inicio.Add(time.Hour).Format("20060102T150405")).Replace(reunionICS)))
	r := cuentaDAV(t, srv, &correoFalso{buzones: []agenda.Buzon{{ID: "m1", Direccion: "ana@example.org"}}})
	repo := abrirCalendario(t, t.TempDir())
	lc := agenda.LocalCalendario{Repo: repo}
	dir := filepath.Join(t.TempDir(), "agenda")
	if _, errs := r.SincronizarTodo(t.Context(), agenda.Calendario, lc, dir, ""); len(errs) > 0 {
		t.Fatal(errs)
	}
	cal := calDe(t, repo, "reunion-1")
	const ediciones = 10
	var wg sync.WaitGroup
	wg.Add(1)
	listo := make(chan struct{})
	go func() {
		defer wg.Done()
		defer close(listo)
		for hechas := 0; hechas < ediciones; {
			o := objetoPorUID(t, repo, "reunion-1")
			if _, err := repo.PonerObjetoCrudo(context.Background(), cal, o.ID, o.Datos, strings.Replace(o.Datos, "LOCATION:", "LOCATION:x", 1)); err == nil {
				hechas++
			}
		}
	}()
	sello := time.Now()
	terminado := func() bool {
		select {
		case <-listo:
			return true
		default:
			return false
		}
	}
	for n := 0; n < 60 && (n < 3 || !terminado()); n++ {
		ps := []string{"ACCEPTED", "DECLINED"}[n%2]
		sello = sello.Add(time.Second)
		if e := r.AplicarRespuestas(t.Context(), lc, dir, [][]byte{reply("reunion-1", "ana@example.org", "luis@example.org", ps, 0, sello, "")}); e[0] == agenda.RespuestaDescartada {
			t.Fatalf("una respuesta nueva mientras se edita espera o se aplica, no se descarta: %v", e)
		}
	}
	wg.Wait()
	sello = sello.Add(time.Second)
	if e := r.AplicarRespuestas(t.Context(), lc, dir, [][]byte{reply("reunion-1", "ana@example.org", "luis@example.org", "TENTATIVE", 0, sello, "")}); e[0] != agenda.RespuestaAplicada {
		t.Fatalf("la última: %v", e)
	}
	d := objetoPorUID(t, repo, "reunion-1").Datos
	if !strings.Contains(d, "LOCATION:"+strings.Repeat("x", ediciones)+"Sala 2") || !strings.Contains(d, "PARTSTAT=TENTATIVE") {
		t.Fatalf("las ediciones y la respuesta quedan: %s", d)
	}
}

// ---- De punta a punta: el REPLY llega a Mail por IMAP y Calendar lo aplica ----

const usuarioIMAP = "ana@example.org"

func imapEnMemoria(t *testing.T) (*imapmemserver.User, int) {
	t.Helper()
	mem := imapmemserver.New()
	u := imapmemserver.NewUser(usuarioIMAP, "clave-de-app")
	for _, n := range []string{"INBOX", "Sent"} {
		if err := u.Create(n, nil); err != nil {
			t.Fatal(err)
		}
	}
	mem.AddUser(u)
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapIMAP4rev2: {}},
		InsecureAuth: true,
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return u, ln.Addr().(*net.TCPAddr).Port
}

// La invitación sale por correo desde Calendar (iMIP, con el SMTP de la cuenta de Mail),
// Luis contesta desde su programa de correo, el REPLY llega a la entrada de Mail por IMAP,
// Mail lo deja en su cola, Calendar lo recoge y lo aplica, y la sincronización lo sube
// a la cuenta CalDAV.
func TestRespuestaDePuntaAPunta(t *testing.T) {
	buzonIMAP, puertoIMAP := imapEnMemoria(t)
	ss := nuevoSMTP(t, usuarioIMAP, "clave-de-app")
	raizMail := t.TempDir()
	sec := &secretosEnMemoria{m: map[string]string{}}
	b, err := correo.Abrir(raizMail, sec, correo.Red{Marcar: marcarDirecto})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Cerrar()
	if _, err := b.Anadir(correo.Cuenta{
		Proveedor: correo.ProveedorIMAP, Direccion: usuarioIMAP, Nombre: "Ana",
		IMAP: correo.Servidor{Host: "127.0.0.1", Puerto: puertoIMAP, Seguridad: correo.SeguridadNinguna},
		SMTP: correo.Servidor{Host: "127.0.0.1", Puerto: ss.ln.Addr().(*net.TCPAddr).Port, Seguridad: correo.SeguridadNinguna},
	}, "clave-de-app"); err != nil {
		t.Fatal(err)
	}
	// La entrada está VACÍA en la primera vuelta de la cuenta: lo que llegue después es
	// nuevo igual (correo.carpeta.estrenada), también la respuesta de Luis.
	if err := b.Sincronizar(t.Context()); err != nil {
		t.Fatal(err)
	}

	srv := davprueba.Nuevo()
	defer srv.Close()
	col := davprueba.CasaCal + "personal/"
	srv.Crear(col, "Personal", false)
	r := cuentaDAV(t, srv, correoDeMail{correo.NuevoRemitente(raizMail, sec, correo.Red{Marcar: marcarDirecto})})
	repo := abrirCalendario(t, t.TempDir())
	lc := agenda.LocalCalendario{Repo: repo}
	dir := filepath.Join(t.TempDir(), "agenda")
	if _, errs := r.SincronizarTodo(t.Context(), agenda.Calendario, lc, dir, ""); len(errs) > 0 {
		t.Fatal(errs)
	}
	// Ana crea la reunión en su calendario de la cuenta: la invitación sale a Luis.
	cal := r.Grupos(agenda.Calendario, lc, dir)[0].Colecciones[0].ID
	inicio := time.Now().AddDate(0, 0, 5).Truncate(time.Hour)
	reunion := strings.NewReplacer("%INICIO%", inicio.Format("20060102T150405"), "%FIN%", inicio.Add(time.Hour).Format("20060102T150405"),
		"ATTENDEE;CN=Marta;PARTSTAT=ACCEPTED:mailto:marta@example.org\r\n", "").Replace(reunionICS)
	if _, err := repo.PonerObjetoCrudo(t.Context(), cal, "", "", reunion); err != nil {
		t.Fatal(err)
	}
	if _, errs := r.SincronizarTodo(t.Context(), agenda.Calendario, lc, dir, ""); len(errs) > 0 {
		t.Fatal(errs)
	}
	if env := ss.tomar(); len(env) != 1 || env[0].para[0] != "luis@example.org" {
		t.Fatalf("la invitación: %+v", env)
	}

	// Luis acepta: su REPLY llega a la entrada de Ana.
	ics := string(reply("reunion-1", usuarioIMAP, "luis@example.org", "ACCEPTED", 0, time.Now(), ""))
	crudo := "From: Luis <luis@example.org>\r\nTo: ana@example.org\r\nSubject: Accepted: Revisión del presupuesto\r\nMessage-ID: <r1@example.org>\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: text/calendar; charset=utf-8; method=REPLY\r\n\r\n" + ics
	if _, err := buzonIMAP.Append("INBOX", strings.NewReader(crudo), &imap.AppendOptions{Time: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := b.Sincronizar(t.Context()); err != nil {
		t.Fatal(err)
	}
	pend, err := correo.RespuestasPorAplicar(raizMail)
	if err != nil || len(pend) != 1 {
		t.Fatalf("en la cola de Mail: %v %v", pend, err)
	}
	// Calendar la recoge (con la app abierta, en su vuelta).
	estados := r.AplicarRespuestas(t.Context(), lc, dir, [][]byte{pend[0].ICS})
	if estados[0] != agenda.RespuestaAplicada {
		t.Fatalf("aplicada: %v", estados)
	}
	if err := pend[0].Quitar(); err != nil {
		t.Fatal(err)
	}
	if _, errs := r.SincronizarTodo(t.Context(), agenda.Calendario, lc, dir, ""); len(errs) > 0 {
		t.Fatal(errs)
	}
	var enServidor string
	for _, ruta := range srv.Rutas(col) {
		enServidor += string(srv.Datos(ruta))
	}
	if !strings.Contains(enServidor, "PARTSTAT=ACCEPTED;RSVP=TRUE:mailto:luis@example.org") && !strings.Contains(enServidor, "RSVP=TRUE;PARTSTAT=ACCEPTED") {
		t.Fatalf("en el servidor CalDAV: %s", enServidor)
	}
	if env := ss.tomar(); len(env) != 0 {
		t.Fatalf("la respuesta no manda más correo: %d", len(env))
	}
}
