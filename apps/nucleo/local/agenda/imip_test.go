// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda_test

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
	"git.sytue.eu/sytue/one/apps/nucleo/local/correo"
)

// ---- Un SMTP de prueba ----

// marcarDirecto marca sin portero: el SMTP de estas pruebas escucha en este equipo. Sin
// Red.Marcar el correo no sale (R-RED).
func marcarDirecto(ctx context.Context, red, direccion string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, red, direccion)
}

type recibido struct {
	de    string
	para  []string
	datos string
}

type smtpFalso struct {
	mu        sync.Mutex
	recibidos []recibido
	ln        net.Listener
	usuario   string
	clave     string
}

type sesionFalsa struct {
	s  *smtpFalso
	r  recibido
	ok bool
}

func (x *sesionFalsa) AuthMechanisms() []string { return []string{sasl.Plain} }
func (x *sesionFalsa) Auth(string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(_, u, p string) error {
		if u != x.s.usuario || p != x.s.clave {
			return errors.New("credenciales")
		}
		x.ok = true
		return nil
	}), nil
}

func (x *sesionFalsa) Mail(de string, _ *smtp.MailOptions) error {
	if !x.ok {
		return smtp.ErrAuthRequired
	}
	x.r.de = de
	return nil
}

func (x *sesionFalsa) Rcpt(para string, _ *smtp.RcptOptions) error {
	x.r.para = append(x.r.para, para)
	return nil
}

func (x *sesionFalsa) Data(r io.Reader) error {
	d, err := io.ReadAll(r)
	x.r.datos = string(d)
	x.s.mu.Lock()
	x.s.recibidos = append(x.s.recibidos, x.r)
	x.s.mu.Unlock()
	return err
}
func (x *sesionFalsa) Reset()        { x.r = recibido{} }
func (x *sesionFalsa) Logout() error { return nil }

func nuevoSMTP(t *testing.T, usuario, clave string) *smtpFalso {
	t.Helper()
	s := &smtpFalso{usuario: usuario, clave: clave}
	srv := smtp.NewServer(smtp.BackendFunc(func(*smtp.Conn) (smtp.Session, error) { return &sesionFalsa{s: s}, nil }))
	srv.Domain, srv.AllowInsecureAuth = "localhost", true
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.ln = ln
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return s
}

// tomar devuelve lo recibido desde la última vez y lo vacía.
func (s *smtpFalso) tomar() []recibido {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.recibidos
	s.recibidos = nil
	return out
}

type secretosEnMemoria struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *secretosEnMemoria) Leer(n string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[n], nil
}

func (s *secretosEnMemoria) Guardar(n, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[n] = v
	return nil
}

func (s *secretosEnMemoria) Borrar(n string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, n)
	return nil
}

// correoDeMail es el Correo de Sytue Office: las cuentas de Mail y su SMTP por
// correo.Remitente, como lo monta Calendar (cmd-local/calendar).
type correoDeMail struct{ r *correo.Remitente }

func (c correoDeMail) Buzones() ([]agenda.Buzon, error) {
	cs, err := c.r.Cuentas()
	out := make([]agenda.Buzon, 0, len(cs))
	for _, x := range cs {
		out = append(out, agenda.Buzon{ID: x.ID, Direccion: x.Direccion, Nombre: x.Nombre})
	}
	return out, err
}

func (c correoDeMail) Enviar(ctx context.Context, id string, para []string, crudo []byte) error {
	return c.r.Enviar(ctx, id, para, crudo)
}

// partes lee un correo iMIP: las cabeceras, la parte text/calendar (con su method) y
// el adjunto .ics.
func partes(t *testing.T, crudo string) (mail.Header, string, string, string) {
	t.Helper()
	m, err := mail.ReadMessage(strings.NewReader(crudo))
	if err != nil {
		t.Fatal(err)
	}
	tipo, ps, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil || tipo != "multipart/mixed" {
		t.Fatalf("tipo: %q %v", tipo, err)
	}
	var metodo, cal, adjunto string
	mr := multipart.NewReader(m.Body, ps["boundary"])
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		pt, pps, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		switch pt {
		case "multipart/alternative":
			ar := multipart.NewReader(p, pps["boundary"])
			for {
				q, err := ar.NextPart()
				if err != nil {
					break
				}
				qt, qps, _ := mime.ParseMediaType(q.Header.Get("Content-Type"))
				if qt == "text/calendar" {
					metodo = qps["method"]
					cal = base64De(t, q)
				}
			}
		case "application/ics":
			adjunto = base64De(t, p)
		}
	}
	return m.Header, metodo, cal, adjunto
}

func base64De(t *testing.T, r io.Reader) string {
	t.Helper()
	d, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, r))
	if err != nil {
		t.Fatal(err)
	}
	return string(d)
}

const reunionICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//prueba//ES\r\nBEGIN:VEVENT\r\nUID:reunion-1\r\nDTSTAMP:20260101T000000Z\r\n" +
	"DTSTART;TZID=Europe/Madrid:%INICIO%\r\nDTEND;TZID=Europe/Madrid:%FIN%\r\nSUMMARY:Revisión del presupuesto\r\nLOCATION:Sala 2\r\n" +
	"ORGANIZER;CN=Ana:mailto:ana@example.org\r\nATTENDEE;CN=Ana;PARTSTAT=ACCEPTED:mailto:ana@example.org\r\n" +
	"ATTENDEE;CN=Luis;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:luis@example.org\r\nATTENDEE;CN=Marta;PARTSTAT=ACCEPTED:mailto:marta@example.org\r\n" +
	"BEGIN:VALARM\r\nACTION:DISPLAY\r\nDESCRIPTION:Aviso\r\nTRIGGER:-PT15M\r\nEND:VALARM\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

// A-11: con un CalDAV que no planifica, lo que la persona organiza sale por correo a
// los invitados desde su cuenta de Mail (REQUEST y CANCEL), y su respuesta a lo que
// organiza otro, al organizador (REPLY). Lo que no cambia para los invitados no manda
// nada; sin cuenta de Mail con la dirección del organizador, se avisa.
func TestInvitacionesPorCorreoSinPlanificacion(t *testing.T) {
	ss := nuevoSMTP(t, "ana@example.org", "clave-de-app")
	raiz := t.TempDir()
	sec := &secretosEnMemoria{m: map[string]string{}}
	buzon, err := correo.Abrir(raiz, sec, correo.Red{Marcar: marcarDirecto})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buzon.Anadir(correo.Cuenta{
		Proveedor: correo.ProveedorIMAP, Direccion: "ana@example.org", Nombre: "Ana",
		IMAP: correo.Servidor{Host: "127.0.0.1", Puerto: 1, Seguridad: correo.SeguridadNinguna},
		SMTP: correo.Servidor{Host: "127.0.0.1", Puerto: ss.ln.Addr().(*net.TCPAddr).Port, Seguridad: correo.SeguridadNinguna},
	}, "clave-de-app"); err != nil {
		t.Fatal(err)
	}
	buzon.Cerrar()
	rem := correo.NuevoRemitente(raiz, sec, correo.Red{Marcar: marcarDirecto})

	m := montarCalendario(t, false)
	m.s.Invitar = (&agenda.Cartero{
		Correo: correoDeMail{rem}, Propia: "ana@example.org",
		Textos: agenda.TextosIMIP{
			Invitacion: "Invitación: %s", Actualizada: "Invitación actualizada: %s", Cancelada: "Cancelada: %s",
			Cuando: "Cuándo", Donde: "Dónde", Organiza: "Organiza",
		},
	}).Invitar
	inicio := time.Now().AddDate(0, 0, 7).Truncate(time.Hour)
	datos := func(ini time.Time) string {
		r := strings.NewReplacer("%INICIO%", ini.Format("20060102T150405"), "%FIN%", ini.Add(time.Hour).Format("20060102T150405"))
		return r.Replace(reunionICS)
	}
	m.srv.Poner(m.col+"reunion-1.ics", []byte(datos(inicio)))
	m.sincronizar()
	if r := ss.tomar(); len(r) != 0 {
		t.Fatalf("bajar no manda nada: %+v", r)
	}
	local := func() (string, string) {
		o := m.locales()["reunion-1"]
		return o.ID, o.Datos
	}
	cambiar := func(f func(string) string) agenda.Resultado {
		t.Helper()
		id, d := local()
		if _, err := m.repo.PonerObjetoCrudo(t.Context(), m.cal, id, "", f(d)); err != nil {
			t.Fatal(err)
		}
		return m.sincronizar()
	}

	// Se mueve la reunión: REQUEST a Luis y a Marta, desde Ana, con el .ics adjunto.
	res := cambiar(func(d string) string { return strings.ReplaceAll(d, "Sala 2", "Sala 3") })
	r := ss.tomar()
	if res.Subidos != 1 || len(r) != 1 || len(res.Invitaciones) != 0 {
		t.Fatalf("cambio: %+v %+v", res, r)
	}
	if r[0].de != "ana@example.org" || !slices.Equal(r[0].para, []string{"luis@example.org", "marta@example.org"}) {
		t.Fatalf("sobre: %q %q", r[0].de, r[0].para)
	}
	h, metodo, cal, adjunto := partes(t, r[0].datos)
	if metodo != "REQUEST" || !strings.Contains(cal, "METHOD:REQUEST") || !strings.Contains(cal, "Sala 3") || cal != adjunto ||
		!strings.Contains(cal, "SEQUENCE:1") || strings.Contains(cal, "VALARM") {
		t.Fatalf("calendario (%s): %s", metodo, cal)
	}
	if asunto, _ := new(mime.WordDecoder).DecodeHeader(h.Get("Subject")); asunto != "Invitación actualizada: Revisión del presupuesto" {
		t.Fatalf("asunto: %q", asunto)
	}

	// Solo el recordatorio: nada que contar a nadie.
	res = cambiar(func(d string) string { return strings.ReplaceAll(d, "TRIGGER:-PT15M", "TRIGGER:-PT30M") })
	if r := ss.tomar(); res.Subidos != 1 || len(r) != 0 {
		t.Fatalf("recordatorio: %+v %+v", res, r)
	}

	// Se quita a Marta: los demás reciben el cambio y ella, la cancelación.
	res = cambiar(func(d string) string {
		return strings.ReplaceAll(d, "ATTENDEE;CN=Marta;PARTSTAT=ACCEPTED:mailto:marta@example.org\r\n", "")
	})
	r = ss.tomar()
	if len(r) != 2 {
		t.Fatalf("quitar a Marta: %+v %+v", res, r)
	}
	_, m1, _, _ := partes(t, r[0].datos)
	_, m2, c2, _ := partes(t, r[1].datos)
	if m1 != "REQUEST" || !slices.Equal(r[0].para, []string{"luis@example.org"}) || m2 != "CANCEL" ||
		!slices.Equal(r[1].para, []string{"marta@example.org"}) || !strings.Contains(c2, "marta@example.org") || strings.Contains(c2, "luis@") {
		t.Fatalf("quitar a Marta: %+v", r)
	}

	// Se borra: CANCEL a quien queda.
	id, _ := local()
	if err := m.repo.BorrarObjetoCrudo(t.Context(), id, ""); err != nil {
		t.Fatal(err)
	}
	res = m.sincronizar()
	r = ss.tomar()
	if res.Borrados != 1 || len(r) != 1 {
		t.Fatalf("borrar: %+v %+v", res, r)
	}
	_, metodo, cal, _ = partes(t, r[0].datos)
	if metodo != "CANCEL" || !strings.Contains(cal, "STATUS:CANCELLED") || !slices.Equal(r[0].para, []string{"luis@example.org"}) {
		t.Fatalf("cancelación: %s %s", metodo, cal)
	}

	// Organiza otro; Ana contesta aquí («¿Vas a ir?»): REPLY a quien organiza, con solo
	// su respuesta.
	otro := strings.NewReplacer("UID:reunion-1", "UID:reunion-2", "ORGANIZER;CN=Ana:mailto:ana@example.org", "ORGANIZER;CN=Pedro:mailto:pedro@example.org",
		"ATTENDEE;CN=Ana;PARTSTAT=ACCEPTED", "ATTENDEE;CN=Ana;PARTSTAT=NEEDS-ACTION").Replace(datos(inicio))
	m.srv.Poner(m.col+"reunion-2.ics", []byte(otro))
	m.sincronizar()
	o := m.locales()["reunion-2"]
	if _, err := m.repo.PonerObjetoCrudo(t.Context(), m.cal, o.ID, "", strings.Replace(o.Datos, "PARTSTAT=NEEDS-ACTION:mailto:ana@", "PARTSTAT=ACCEPTED:mailto:ana@", 1)); err != nil {
		t.Fatal(err)
	}
	m.sincronizar()
	r = ss.tomar()
	if len(r) != 1 || !slices.Equal(r[0].para, []string{"pedro@example.org"}) {
		t.Fatalf("respuesta: %+v", r)
	}
	h, metodo, cal, _ = partes(t, r[0].datos)
	asunto, _ := new(mime.WordDecoder).DecodeHeader(h.Get("Subject"))
	if metodo != "REPLY" || strings.Contains(cal, "luis@") || !strings.Contains(cal, "PARTSTAT=ACCEPTED:mailto:ana@example.org") ||
		asunto != "Accepted: Revisión del presupuesto" || r[0].de != "ana@example.org" {
		t.Fatalf("REPLY: %s %q %s", metodo, asunto, cal)
	}

	// Una reunión de la cuenta sin cuenta de Mail con su dirección: no se manda nada y
	// se avisa.
	m.s.Invitar = (&agenda.Cartero{Correo: correoDeMail{rem}, Propia: "ana@icloud.example"}).Invitar
	sinMail := strings.NewReplacer("UID:reunion-1", "UID:reunion-3", "mailto:ana@example.org", "mailto:ana@icloud.example").Replace(datos(inicio))
	m.srv.Poner(m.col+"reunion-3.ics", []byte(sinMail))
	m.sincronizar()
	o = m.locales()["reunion-3"]
	if _, err := m.repo.PonerObjetoCrudo(t.Context(), m.cal, o.ID, "", strings.ReplaceAll(o.Datos, "Sala 2", "Sala 9")); err != nil {
		t.Fatal(err)
	}
	res = m.sincronizar()
	var sin agenda.SinRemitente
	if r := ss.tomar(); len(r) != 0 || len(res.Invitaciones) != 1 || !errors.As(res.Invitaciones[0], &sin) || sin.Direccion != "ana@icloud.example" {
		t.Fatalf("sin remitente: %+v %+v", res, r)
	}

	// Con un servidor que planifica, él manda las invitaciones: aquí nada.
	m.srv.Planifica = true
	m.s.Invitar = (&agenda.Cartero{Correo: correoDeMail{rem}, Propia: "ana@example.org"}).Invitar
	m.srv.Poner(m.col+"reunion-4.ics", []byte(strings.Replace(datos(inicio), "UID:reunion-1", "UID:reunion-4", 1)))
	m.sincronizar()
	o = m.locales()["reunion-4"]
	if _, err := m.repo.PonerObjetoCrudo(t.Context(), m.cal, o.ID, "", strings.ReplaceAll(o.Datos, "Sala 2", "Sala 5")); err != nil {
		t.Fatal(err)
	}
	if res := m.sincronizar(); res.Subidos != 1 || len(ss.tomar()) != 0 {
		t.Fatalf("con planificación: %+v", res)
	}
}
