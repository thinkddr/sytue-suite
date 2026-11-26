// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
)

// Todo en proceso y sin red de verdad: un servidor IMAP en memoria (el de la propia
// biblioteca) y uno SMTP de prueba, en 127.0.0.1.

const (
	usuario = "ana@ejemplo.eu"
	clave   = "contraseña-de-app"
)

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

type servidorIMAP struct {
	user *imapmemserver.User
	srv  *imapserver.Server
	ln   net.Listener
}

func nuevoIMAP(t *testing.T) *servidorIMAP {
	t.Helper()
	mem := imapmemserver.New()
	u := imapmemserver.NewUser(usuario, clave)
	for _, n := range []string{"INBOX", "Sent", "Trash", "Drafts"} {
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
		Logger:       nadaLogger{},
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return &servidorIMAP{user: u, srv: srv, ln: ln}
}

// marcarDirecto marca sin portero: los servidores de estas pruebas escuchan en este
// equipo. Sin Red.Marcar el correo no sale (R-RED, Red.marcar falla cerrado).
func marcarDirecto(ctx context.Context, red, direccion string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, red, direccion)
}

type nadaLogger struct{}

func (nadaLogger) Printf(string, ...any) {}

func (s *servidorIMAP) entregar(t *testing.T, buzon, crudo string) {
	t.Helper()
	r := strings.NewReader(strings.ReplaceAll(crudo, "\n", "\r\n"))
	if _, err := s.user.Append(buzon, r, &imap.AppendOptions{Time: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

func (s *servidorIMAP) cuantos(t *testing.T, buzon string) (total, sinLeer uint32) {
	t.Helper()
	st, err := s.user.Status(buzon, &imap.StatusOptions{NumMessages: true, NumUnseen: true})
	if err != nil {
		t.Fatal(err)
	}
	return *st.NumMessages, *st.NumUnseen
}

// ---- SMTP de prueba ----

type recibido struct {
	de    string
	para  []string
	datos []byte
}

type servidorSMTP struct {
	mu        sync.Mutex
	recibidos []recibido
	ln        net.Listener
}

type sesionSMTP struct {
	s  *servidorSMTP
	r  recibido
	ok bool
}

func (x *sesionSMTP) AuthMechanisms() []string { return []string{sasl.Plain} }
func (x *sesionSMTP) Auth(string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(_, u, p string) error {
		if u != usuario || p != clave {
			return errors.New("credenciales")
		}
		x.ok = true
		return nil
	}), nil
}

func (x *sesionSMTP) Mail(de string, _ *smtp.MailOptions) error {
	if !x.ok {
		return smtp.ErrAuthRequired
	}
	x.r.de = de
	return nil
}

func (x *sesionSMTP) Rcpt(para string, _ *smtp.RcptOptions) error {
	x.r.para = append(x.r.para, para)
	return nil
}

func (x *sesionSMTP) Data(r io.Reader) error {
	d, err := io.ReadAll(r)
	x.r.datos = d
	x.s.mu.Lock()
	x.s.recibidos = append(x.s.recibidos, x.r)
	x.s.mu.Unlock()
	return err
}
func (x *sesionSMTP) Reset()        { x.r = recibido{} }
func (x *sesionSMTP) Logout() error { return nil }

func nuevoSMTP(t *testing.T) *servidorSMTP {
	t.Helper()
	s := &servidorSMTP{}
	srv := smtp.NewServer(smtp.BackendFunc(func(*smtp.Conn) (smtp.Session, error) { return &sesionSMTP{s: s}, nil }))
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

func (s *servidorSMTP) lista() []recibido {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.recibidos)
}

func puerto(ln net.Listener) int { return ln.Addr().(*net.TCPAddr).Port }

func abrir(t *testing.T, si *servidorIMAP, ss *servidorSMTP) *Buzon {
	t.Helper()
	b, err := Abrir(t.TempDir(), &secretosEnMemoria{m: map[string]string{}}, Red{Marcar: marcarDirecto})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Cerrar)
	_, err = b.Anadir(Cuenta{
		Proveedor: ProveedorIMAP, Direccion: usuario, Nombre: "Ana",
		IMAP: Servidor{Host: "127.0.0.1", Puerto: puerto(si.ln), Seguridad: SeguridadNinguna},
		SMTP: Servidor{Host: "127.0.0.1", Puerto: puerto(ss.ln), Seguridad: SeguridadNinguna},
	}, clave)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

const primero = `From: Luis <luis@ejemplo.eu>
To: ana@ejemplo.eu
Subject: =?utf-8?q?Reuni=C3=B3n_del_martes?=
Date: Mon, 16 Nov 2026 10:00:00 +0100
Message-ID: <uno@ejemplo.eu>
Content-Type: text/plain; charset=utf-8

¿Nos vemos el martes a las diez?
`

const segundo = `From: Marta <marta@ejemplo.eu>
To: ana@ejemplo.eu
Subject: Presupuesto
Date: Tue, 17 Nov 2026 09:00:00 +0100
Message-ID: <dos@ejemplo.eu>
Content-Type: text/plain; charset=utf-8

Te adjunto el presupuesto revisado.
`

func TestDeExtremoAExtremo(t *testing.T) {
	ctx := context.Background()
	si, ss := nuevoIMAP(t), nuevoSMTP(t)
	si.entregar(t, "INBOX", primero)
	si.entregar(t, "INBOX", segundo)
	b := abrir(t, si, ss)

	// Sin sincronizar todavía: las carpetas fijas, vacías, y ninguna conexión.
	cs, err := b.Mailboxes(ctx)
	if err != nil || len(cs) < 4 || cs[0].ID != "rol:inbox" {
		t.Fatalf("carpetas iniciales: %+v %v", cs, err)
	}

	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	cs, _ = b.Mailboxes(ctx)
	if cs[0].TotalEmails != 2 || cs[0].UnreadEmails != 2 {
		t.Fatalf("Recibidos tras sincronizar: %+v", cs[0])
	}
	ms, siguiente, err := b.Messages(ctx, "rol:inbox", "", 50)
	if err != nil || len(ms) != 2 || siguiente != "" {
		t.Fatalf("lista: %d %q %v", len(ms), siguiente, err)
	}
	if ms[0].Subject != "Presupuesto" || ms[1].Subject != "Reunión del martes" || ms[1].FromName != "Luis" {
		t.Fatalf("orden o cabeceras: %q %q %q", ms[0].Subject, ms[1].Subject, ms[1].FromName)
	}
	if !strings.Contains(ms[1].Preview, "martes a las diez") {
		t.Fatalf("el cuerpo no se bajó para leer sin conexión: %q", ms[1].Preview)
	}
	// Paginación por clave.
	p1, cur, _ := b.Messages(ctx, "rol:inbox", "", 1)
	p2, cur2, _ := b.Messages(ctx, "rol:inbox", cur, 1)
	if len(p1) != 1 || len(p2) != 1 || p1[0].ID == p2[0].ID || cur2 != "" {
		t.Fatalf("páginas: %v %v %q", p1, p2, cur2)
	}
	// Búsqueda sin acentos.
	if r, _ := b.Search(ctx, "reunion", 0, 10); len(r) != 1 || r[0].ID != ms[1].ID {
		t.Fatalf("búsqueda: %+v", r)
	}

	// Leer: el .eml está en el equipo.
	rc, err := b.Raw(ctx, ms[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	m, err := eml.Leer(rc)
	rc.Close()
	if err != nil || !strings.Contains(m.Texto, "martes") {
		t.Fatalf("cuerpo: %q %v", m.Texto, err)
	}

	// Marcar como leído: en local ya y en el servidor tras la vuelta.
	estado, _ := b.Changes(ctx, "")
	if err := b.SetFlagMany(ctx, []string{ms[1].ID}, api.MailFlagSeen, true); err != nil {
		t.Fatal(err)
	}
	delta, _ := b.Changes(ctx, estado.State)
	if len(delta.Changed) != 1 || !slices.Contains(delta.Changed[0].Keywords, api.MailFlagSeen) {
		t.Fatalf("delta tras marcar: %+v", delta)
	}
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	if _, sinLeer := si.cuantos(t, "INBOX"); sinLeer != 1 {
		t.Fatalf("el servidor no recibió la marca: %d sin leer", sinLeer)
	}

	// Llega correo nuevo: el aviso de llegadas salta y el delta lo trae.
	avisos := make(chan struct{}, 8)
	lctx, parar := context.WithCancel(ctx)
	defer parar()
	go func() { _ = b.Llegadas(lctx, func() { avisos <- struct{}{} }) }()
	time.Sleep(20 * time.Millisecond)
	estado, _ = b.Changes(ctx, "")
	si.entregar(t, "INBOX", strings.ReplaceAll(strings.ReplaceAll(segundo, "dos@", "tres@"), "Subject: Presupuesto", "Subject: Re: Presupuesto\nIn-Reply-To: <dos@ejemplo.eu>"))
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-avisos:
	case <-time.After(2 * time.Second):
		t.Fatal("no llegó el aviso de correo nuevo")
	}
	delta, _ = b.Changes(ctx, estado.State)
	if len(delta.Changed) != 1 || delta.Changed[0].Subject != "Re: Presupuesto" {
		t.Fatalf("delta de llegada: %+v", delta)
	}
	// Hilo por In-Reply-To.
	if h, _ := b.Thread(ctx, delta.Changed[0].ThreadID); len(h) != 2 {
		t.Fatalf("hilo: %d mensajes", len(h))
	}

	// Mover a Papelera: en el servidor tras la vuelta, sin duplicar en local.
	if err := b.MoveMany(ctx, []string{ms[0].ID}, "rol:trash"); err != nil {
		t.Fatal(err)
	}
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := si.cuantos(t, "Trash"); n != 1 {
		t.Fatalf("Papelera del servidor: %d", n)
	}
	if tr, _, _ := b.Messages(ctx, "rol:trash", "", 50); len(tr) != 1 {
		t.Fatalf("Papelera local: %d", len(tr))
	}

	// Enviar: sale por SMTP, queda en Enviados y se sube con APPEND una sola vez.
	crudo, err := eml.Componer(eml.Salida{
		De: usuario, DeNombre: "Ana", Para: []string{"luis@ejemplo.eu"}, Cco: []string{"oculto@ejemplo.eu"},
		Asunto: "Re: Reunión del martes", Texto: "Allí estaré.", EnRespuestaA: "<uno@ejemplo.eu>",
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	env, err := b.Send(ctx, usuario, []string{"luis@ejemplo.eu", "oculto@ejemplo.eu"}, crudo, 0)
	if err != nil || env.ID == "" {
		t.Fatalf("enviar: %+v %v", env, err)
	}
	b.VaciarSalida(ctx)
	rs := ss.lista()
	if len(rs) != 1 || rs[0].de != usuario || len(rs[0].para) != 2 || !bytes.Contains(rs[0].datos, []byte("In-Reply-To: <uno@ejemplo.eu>")) {
		t.Fatalf("SMTP: %d mensajes", len(rs))
	}
	if err := b.CancelOutbound(ctx, env.ID); !errors.Is(err, api.ErrDemasiadoTarde) {
		t.Fatalf("deshacer un envío ya salido: %v", err)
	}
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := si.cuantos(t, "Sent"); n != 1 {
		t.Fatalf("Enviados del servidor: %d", n)
	}
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	if env, _, _ := b.Messages(ctx, "rol:sent", "", 50); len(env) != 1 {
		t.Fatalf("Enviados local (sin duplicar): %d", len(env))
	}

	// Deshacer dentro de la ventana: no sale.
	env2, _ := b.Send(ctx, usuario, []string{"luis@ejemplo.eu"}, crudo, 10)
	if err := b.CancelOutbound(ctx, env2.ID); err != nil {
		t.Fatal(err)
	}
	b.VaciarSalida(ctx)
	if len(ss.lista()) != 1 {
		t.Fatal("salió un envío deshecho")
	}

	// Sin red: se sigue leyendo lo descargado, y lo enviado espera.
	_ = si.srv.Close()
	_ = ss.ln.Close()
	if err := b.Sincronizar(ctx); err == nil {
		t.Fatal("sincronizar sin servidor debería fallar")
	}
	ms, _, err = b.Messages(ctx, "rol:inbox", "", 50)
	if err != nil || len(ms) != 2 {
		t.Fatalf("lista sin red: %d %v", len(ms), err)
	}
	for _, x := range ms {
		rc, err := b.Raw(ctx, x.ID)
		if err != nil {
			t.Fatalf("leer sin red: %v", err)
		}
		rc.Close()
	}
	env3, err := b.Send(ctx, usuario, []string{"luis@ejemplo.eu"}, crudo, 0)
	if err != nil {
		t.Fatal(err)
	}
	b.VaciarSalida(ctx)
	var estadoSalida string
	_ = b.x.db.QueryRow("SELECT estado FROM salida WHERE id = ?", env3.ID).Scan(&estadoSalida)
	if estadoSalida != salidaPendiente {
		t.Fatalf("sin red el envío debería esperar: %q", estadoSalida)
	}
	if c := b.Cuentas()[0]; c.Estado != EstadoOK {
		t.Fatalf("sin red la cuenta no pide volver a entrar: %+v", c)
	}
}

func TestContrasenaRechazada(t *testing.T) {
	ctx := context.Background()
	si, ss := nuevoIMAP(t), nuevoSMTP(t)
	b := abrir(t, si, ss)
	c := b.Cuentas()[0]
	if _, err := b.Anadir(c, "otra"); err != nil {
		t.Fatal(err)
	}
	var avisada error
	b.AlFallar = func(_ Cuenta, err error) { avisada = err }
	if err := b.Sincronizar(ctx); !errors.Is(err, ErrAcceso) {
		t.Fatalf("error: %v", err)
	}
	if b.Cuentas()[0].Estado != EstadoReconectar || !errors.Is(avisada, ErrAcceso) {
		t.Fatalf("estado: %+v, aviso %v", b.Cuentas()[0], avisada)
	}
	if err := b.Probar(ctx, c); !errors.Is(err, ErrAcceso) {
		t.Fatalf("probar: %v", err)
	}
}

func TestBorradoresYEtiquetas(t *testing.T) {
	ctx := context.Background()
	si, ss := nuevoIMAP(t), nuevoSMTP(t)
	si.entregar(t, "INBOX", primero)
	b := abrir(t, si, ss)
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	crudo, _ := eml.Componer(eml.Salida{De: usuario, Asunto: "Borrador", Texto: "a medias", Borrador: true}, time.Now())
	d1, err := b.SaveDraft(ctx, "", usuario, crudo)
	if err != nil || d1.MailboxID != "rol:drafts" {
		t.Fatalf("borrador: %+v %v", d1, err)
	}
	d2, err := b.SaveDraft(ctx, d1.ID, usuario, crudo)
	if err != nil || d2.ID == d1.ID {
		t.Fatalf("resguardar: %+v %v", d2, err)
	}
	if ds, _, _ := b.Messages(ctx, "rol:drafts", "", 10); len(ds) != 1 {
		t.Fatalf("borradores: %d", len(ds))
	}
	l, err := b.CreateLabel(ctx, "Trabajo", "blue")
	if err != nil {
		t.Fatal(err)
	}
	ms, _, _ := b.Messages(ctx, "rol:inbox", "", 10)
	if err := b.ApplyLabel(ctx, l.ID, []string{ms[0].ID}, true); err != nil {
		t.Fatal(err)
	}
	if es, _, _ := b.LabelMessages(ctx, l.ID, "", 10); len(es) != 1 || !slices.Contains(es[0].Keywords, l.ID) {
		t.Fatalf("etiqueta: %+v", es)
	}
	if err := b.SetSignature(ctx, "Ana"); err != nil {
		t.Fatal(err)
	}
	if f, _ := b.Signature(ctx); f != "Ana" {
		t.Fatalf("firma: %q", f)
	}
	if p, a, _ := b.Aliases(ctx); p != usuario || len(a) != 0 {
		t.Fatalf("De: %q %v", p, a)
	}
	// La regla mueve lo que llega de Marta a Papelera.
	if _, err := b.CreateFilter(ctx, api.MailFilter{Name: "Marta", Enabled: true, MatchFrom: "marta", MoveTo: "rol:trash"}); err != nil {
		t.Fatal(err)
	}
	si.entregar(t, "INBOX", segundo)
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := si.cuantos(t, "Trash"); n != 1 {
		t.Fatalf("la regla no movió en el servidor: %d", n)
	}
}

func TestDetectar(t *testing.T) {
	for dir, prov := range map[string]string{
		"x@gmail.com": ProveedorGmail, "x@hotmail.es": ProveedorMicrosoft, "x@icloud.com": ProveedorICloud,
		"x@yahoo.fr": ProveedorYahoo, "x@sytue.eu": ProveedorSytue, "x@empresa.es": ProveedorIMAP,
	} {
		if p := Detectar(dir); p.Proveedor != prov {
			t.Errorf("%s: %s", dir, p.Proveedor)
		}
	}
	if p := Detectar("x@empresa.es"); p.IMAP.Host != "imap.empresa.es" || p.SMTP.Puerto != 587 {
		t.Errorf("IMAP genérico: %+v", p)
	}
}
