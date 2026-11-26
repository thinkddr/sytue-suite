// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	portero "git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// esperar repite cond hasta que se cumple o pasa el plazo.
func esperar(t *testing.T, plazo time.Duration, que string, cond func() bool) {
	t.Helper()
	fin := time.Now().Add(plazo)
	for !cond() {
		if time.Now().After(fin) {
			t.Fatalf("no ha pasado a tiempo: %s", que)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestIdleTraeElCorreoEnElActo: con la app abierta, la entrada de la cuenta espera en
// IDLE; lo que llega se baja en el acto, sin esperar al sondeo (que aquí es de una hora),
// la pantalla se entera y sale el aviso de llegada.
func TestIdleTraeElCorreoEnElActo(t *testing.T) {
	ctx := context.Background()
	si, ss := nuevoIMAP(t), nuevoSMTP(t)
	si.entregar(t, "INBOX", primero)
	b := abrir(t, si, ss)
	b.Intervalo = time.Hour
	var mu sync.Mutex
	var llegadas []Llegada
	b.AlLlegar = func(ls []Llegada) {
		mu.Lock()
		defer mu.Unlock()
		llegadas = append(llegadas, ls...)
	}
	cambios := make(chan struct{}, 16)
	b.AlCambiar(func() {
		select {
		case cambios <- struct{}{}:
		default:
		}
	})
	b.Arrancar()
	id := b.Cuentas()[0].ID
	esperar(t, 5*time.Second, "la entrada en IDLE", func() bool { return b.Vigilando(id) })
	esperar(t, 5*time.Second, "la primera vuelta", func() bool {
		ms, _, _ := b.Messages(ctx, "rol:inbox", "", 10)
		return len(ms) == 1
	})
	for len(cambios) > 0 {
		<-cambios
	}

	si.entregar(t, "INBOX", segundo)
	esperar(t, 5*time.Second, "el correo nuevo por IDLE", func() bool {
		ms, _, _ := b.Messages(ctx, "rol:inbox", "", 10)
		return len(ms) == 2
	})
	select {
	case <-cambios:
	case <-time.After(2 * time.Second):
		t.Fatal("la pantalla no se ha enterado")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(llegadas) != 1 || llegadas[0].Mensaje.Subject != "Presupuesto" || llegadas[0].Cuenta.ID != id {
		t.Fatalf("aviso de llegada: %+v", llegadas)
	}
}

// TestBorradorSeSubeConAppend: el borrador va a «Drafts» del servidor y, al
// resguardarlo, el anterior se borra allí: uno solo en cada lado.
func TestBorradorSeSubeConAppend(t *testing.T) {
	ctx := context.Background()
	si, ss := nuevoIMAP(t), nuevoSMTP(t)
	b := abrir(t, si, ss)
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	crudo, _ := eml.Componer(eml.Salida{De: usuario, Asunto: "Borrador", Texto: "a medias", Borrador: true}, time.Now())
	d1, err := b.SaveDraft(ctx, "", usuario, crudo)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := si.cuantos(t, "Drafts"); n != 1 {
		t.Fatalf("borradores en el servidor: %d", n)
	}
	crudo2, _ := eml.Componer(eml.Salida{De: usuario, Asunto: "Borrador", Texto: "ya casi", Borrador: true}, time.Now())
	if _, err := b.SaveDraft(ctx, d1.ID, usuario, crudo2); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := b.Sincronizar(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := si.cuantos(t, "Drafts"); n != 1 {
		t.Fatalf("borradores en el servidor tras resguardar: %d", n)
	}
	ds, _, _ := b.Messages(ctx, "rol:drafts", "", 10)
	if len(ds) != 1 || !strings.Contains(ds[0].Preview, "ya casi") {
		t.Fatalf("borradores en local: %+v", ds)
	}
}

// otraCuenta da de alta una segunda cuenta contra otro servidor IMAP.
func otraCuenta(t *testing.T, b *Buzon, si *servidorIMAP, ss *servidorSMTP, dir string) Cuenta {
	t.Helper()
	c, err := b.Anadir(Cuenta{
		Proveedor: ProveedorIMAP, Direccion: dir,
		IMAP: Servidor{Host: "127.0.0.1", Puerto: puerto(si.ln), Seguridad: SeguridadNinguna, Usuario: usuario},
		SMTP: Servidor{Host: "127.0.0.1", Puerto: puerto(ss.ln), Seguridad: SeguridadNinguna, Usuario: usuario},
	}, clave)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestCarpetasEnLaCuentaElegida: con dos cuentas, la carpeta nueva va a la que se elige
// (no siempre a la primera), y se renombra y se borra en su servidor.
func TestCarpetasEnLaCuentaElegida(t *testing.T) {
	ctx := context.Background()
	si1, si2, ss := nuevoIMAP(t), nuevoIMAP(t), nuevoSMTP(t)
	b := abrir(t, si1, ss)
	c2 := otraCuenta(t, b, si2, ss, "ana.trabajo@ejemplo.eu")
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	if cs := b.CuentasCarpetas(); len(cs) != 2 || cs[1].ID != c2.ID {
		t.Fatalf("cuentas para carpetas: %+v", cs)
	}
	m, err := b.CreateMailboxIn(ctx, c2.ID, "Proyectos")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := si2.user.Status("Proyectos", &imap.StatusOptions{}); err != nil {
		t.Fatalf("la carpeta no está en la segunda cuenta: %v", err)
	}
	if _, err := si1.user.Status("Proyectos", &imap.StatusOptions{}); err == nil {
		t.Fatal("la carpeta ha ido a la primera cuenta")
	}
	if _, err := b.CreateMailboxIn(ctx, c2.ID, "Proyectos"); err == nil {
		t.Fatal("crear una carpeta que ya existe debería fallar")
	}
	// La pantalla la enseña con « · cuenta» (hay dos): renombrar no se lo lleva.
	var vista api.Mailbox
	cs, _ := b.Mailboxes(ctx)
	for _, x := range cs {
		if x.ID == m.ID {
			vista = x
		}
	}
	if !strings.HasSuffix(vista.Name, " · "+c2.Titulo()) {
		t.Fatalf("nombre en el lateral: %q", vista.Name)
	}
	if motivo, err := b.RenameMailbox(ctx, m.ID, "Clientes · "+c2.Titulo()); err != nil || motivo != "" {
		t.Fatalf("renombrar: %q %v", motivo, err)
	}
	if _, err := si2.user.Status("Clientes", &imap.StatusOptions{}); err != nil {
		t.Fatalf("no se renombró en el servidor: %v", err)
	}
	if _, err := b.DeleteMailbox(ctx, idCarpeta(c2.ID, "Clientes", "")); err != nil {
		t.Fatal(err)
	}
	if _, err := si2.user.Status("Clientes", &imap.StatusOptions{}); err == nil {
		t.Fatal("la carpeta sigue en el servidor")
	}
}

// TestSinCifrarSoloEnEsteEquipo: «ninguna» solo vale con localhost.
func TestSinCifrarSoloEnEsteEquipo(t *testing.T) {
	b, err := Abrir(t.TempDir(), &secretosEnMemoria{m: map[string]string{}}, Red{Marcar: marcarDirecto})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Cerrar()
	c := Cuenta{
		Direccion: "ana@ejemplo.eu", IMAP: Servidor{Host: "imap.ejemplo.eu", Puerto: 143, Seguridad: SeguridadNinguna},
		SMTP: Servidor{Host: "smtp.ejemplo.eu", Puerto: 587, Seguridad: SeguridadSTARTTLS},
	}
	if _, err := b.Anadir(c, "x"); !errors.Is(err, ErrSinCifrar) {
		t.Fatalf("alta sin cifrar a un servidor de fuera: %v", err)
	}
	for _, h := range []string{"localhost", "127.0.0.1", "::1", "[::1]", "LOCALHOST."} {
		if !HostLocal(h) {
			t.Errorf("%s debería ser local", h)
		}
	}
	for _, h := range []string{"imap.ejemplo.eu", "192.168.1.10", "10.0.0.1", "localhost.ejemplo.eu"} {
		if HostLocal(h) {
			t.Errorf("%s no es de este equipo", h)
		}
	}
	// Un cuentas.json tocado a mano tampoco abre una conexión en claro hacia fuera.
	if _, err := (Red{}).marcar(context.Background(), Servidor{Host: "192.0.2.1", Puerto: 143, Seguridad: SeguridadNinguna}); !errors.Is(err, ErrSinCifrar) {
		t.Fatalf("marcar sin cifrar: %v", err)
	}
}

// TestPosponerYSilenciar: lo pospuesto sale de Recibidos y vuelve solo, sin leer y
// arriba; lo silenciado se va al Archivo, y también lo que llega después.
func TestPosponerYSilenciar(t *testing.T) {
	ctx := context.Background()
	si, ss := nuevoIMAP(t), nuevoSMTP(t)
	if err := si.user.Create("Archive", nil); err != nil {
		t.Fatal(err)
	}
	si.entregar(t, "INBOX", primero)
	si.entregar(t, "INBOX", segundo)
	b := abrir(t, si, ss)
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	ms, _, _ := b.Messages(ctx, "rol:inbox", "", 10)
	reunion := ms[1]
	if err := b.SetFlagMany(ctx, []string{reunion.ID}, api.MailFlagSeen, true); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SnoozeThread(ctx, reunion.ThreadID, time.Now().Add(time.Second)); !errors.Is(err, api.ErrSnoozeInvalido) {
		t.Fatalf("posponer un segundo: %v", err)
	}
	if _, err := b.SnoozeThread(ctx, reunion.ThreadID, time.Now().Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if ms, _, _ := b.Messages(ctx, "rol:inbox", "", 10); len(ms) != 1 {
		t.Fatalf("lo pospuesto sigue en Recibidos: %d", len(ms))
	}
	if ps, _, _ := b.Messages(ctx, "rol:snoozed", "", 10); len(ps) != 1 || ps[0].ID != reunion.ID {
		t.Fatalf("Pospuestos: %+v", ps)
	}
	if e, _ := b.ThreadState(ctx, reunion.ThreadID); e.SnoozedUntil == nil {
		t.Fatalf("estado del hilo: %+v", e)
	}
	cs, _ := b.Mailboxes(ctx)
	if cs[1].ID != "rol:snoozed" || cs[1].TotalEmails != 1 || cs[0].TotalEmails != 1 {
		t.Fatalf("contadores: %+v %+v", cs[0], cs[1])
	}
	// Vence: vuelve sin leer y arriba, con su hora de vuelta.
	if _, err := b.x.db.Exec("UPDATE mensajes SET pospuesto = ? WHERE pospuesto > 0", time.Now().Add(-time.Minute).Unix()); err != nil {
		t.Fatal(err)
	}
	b.despertarPospuestos(ctx)
	ms, _, _ = b.Messages(ctx, "rol:inbox", "", 10)
	if len(ms) != 2 || ms[0].ID != reunion.ID || ms[0].UnsnoozedAt == nil || slices.Contains(ms[0].Keywords, api.MailFlagSeen) {
		t.Fatalf("tras volver: %+v", ms)
	}

	// Silenciar: lo de Recibidos al Archivo (también en el servidor), y la respuesta
	// que llega después, igual.
	presupuesto := ms[1]
	if n, err := b.MuteThread(ctx, presupuesto.ThreadID, true); err != nil || n != 1 {
		t.Fatalf("silenciar: %d %v", n, err)
	}
	si.entregar(t, "INBOX", strings.ReplaceAll(strings.ReplaceAll(segundo, "dos@", "cuatro@"), "Subject: Presupuesto", "Subject: Re: Presupuesto\nIn-Reply-To: <dos@ejemplo.eu>"))
	for range 2 {
		if err := b.Sincronizar(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := si.cuantos(t, "Archive"); n != 2 {
		t.Fatalf("Archivo del servidor: %d", n)
	}
	if ms, _, _ := b.Messages(ctx, "rol:inbox", "", 10); len(ms) != 1 {
		t.Fatalf("Recibidos tras silenciar: %d", len(ms))
	}
	if e, _ := b.ThreadState(ctx, presupuesto.ThreadID); !e.Muted {
		t.Fatal("el hilo no consta como silenciado")
	}
}

// TestBajaDeListas: un clic (POST de RFC 8058) y, si no, el correo de baja.
func TestBajaDeListas(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	var posts []string
	web := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		mu.Lock()
		posts = append(posts, r.Method+" "+r.URL.Path+" "+string(cuerpo))
		mu.Unlock()
	}))
	defer web.Close()
	si, ss := nuevoIMAP(t), nuevoSMTP(t)
	si.entregar(t, "INBOX", "From: Boletín <noticias@ejemplo.eu>\nTo: ana@ejemplo.eu\nSubject: Novedades\nMessage-ID: <b1@ejemplo.eu>\n"+
		"List-Unsubscribe: <mailto:baja@ejemplo.eu?subject=baja>, <"+web.URL+"/baja?u=1>\nList-Unsubscribe-Post: List-Unsubscribe=One-Click\n\nHola\n")
	si.entregar(t, "INBOX", "From: Otro <otro@ejemplo.eu>\nTo: ana@ejemplo.eu\nSubject: Otro\nMessage-ID: <b2@ejemplo.eu>\n"+
		"List-Unsubscribe: <mailto:salir@ejemplo.eu?subject=fuera>\n\nHola\n")
	b := abrir(t, si, ss)
	b.red.Web = web.Client()
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	ms, _, _ := b.Messages(ctx, "rol:inbox", "", 10)
	porAsunto := map[string]string{}
	for _, m := range ms {
		porAsunto[m.Subject] = m.ID
	}
	if bj, err := b.Unsubscribe(ctx, porAsunto["Novedades"]); err != nil || !bj.OneClick || !bj.Mailto {
		t.Fatalf("métodos: %+v %v", bj, err)
	}
	if r, err := b.DoUnsubscribe(ctx, porAsunto["Novedades"]); err != nil || r != api.BajaHecha {
		t.Fatalf("un clic: %q %v", r, err)
	}
	mu.Lock()
	if len(posts) != 1 || posts[0] != "POST /baja List-Unsubscribe=One-Click" {
		t.Fatalf("POST: %v", posts)
	}
	mu.Unlock()
	if bj, _ := b.Unsubscribe(ctx, porAsunto["Otro"]); bj.OneClick || !bj.Mailto {
		t.Fatalf("solo correo: %+v", bj)
	}
	if r, err := b.DoUnsubscribe(ctx, porAsunto["Otro"]); err != nil || r != api.BajaEnviada {
		t.Fatalf("por correo: %q %v", r, err)
	}
	b.VaciarSalida(ctx)
	if rs := ss.lista(); len(rs) != 1 || rs[0].para[0] != "salir@ejemplo.eu" || !strings.Contains(string(rs[0].datos), "Subject: fuera") {
		t.Fatalf("correo de baja: %+v", rs)
	}

	// Sin cliente inyectado, lo que pide un correo no sale (R-RED): el permiso puntual del
	// portero es quien lo deja ir, y solo a direcciones públicas (red.Publica).
	b.red.Web = nil
	if _, err := b.ImagenRemota(ctx, web.URL+"/pixel.gif"); !errors.Is(err, portero.ErrDenegado) {
		t.Fatalf("imagen sin el portero: %v", err)
	}
	if err := b.ConfiarRemitente(ctx, "Noticias@Ejemplo.eu"); err != nil || !b.RemitenteConfiado(ctx, "noticias@ejemplo.eu") {
		t.Fatalf("confiar: %v", err)
	}
}

const invitacion = `From: Luis <luis@ejemplo.eu>
To: ana@ejemplo.eu
Subject: Invitación: Revisión
Message-ID: <inv@ejemplo.eu>
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="b"

--b
Content-Type: text/plain; charset=utf-8

Te invito.
--b
Content-Type: text/calendar; charset=utf-8; method=REQUEST

BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Prueba//ES
METHOD:REQUEST
BEGIN:VEVENT
UID:evento-1@ejemplo.eu
SEQUENCE:2
DTSTAMP:20261120T090000Z
DTSTART:20261201T090000Z
DTEND:20261201T100000Z
SUMMARY:Revisión trimestral
LOCATION:Sala 3
ORGANIZER;CN=Luis:mailto:luis@ejemplo.eu
ATTENDEE;CN=Ana;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:ana@ejemplo.eu
ATTENDEE;CN=Marta;PARTSTAT=ACCEPTED:mailto:marta@ejemplo.eu
END:VEVENT
END:VCALENDAR
--b--
`

// TestInvitacionSeContestaConReply: el lector resume la invitación y, al contestar, a
// quien organiza le llega el REPLY de iTIP y queda la copia con la respuesta.
func TestInvitacionSeContestaConReply(t *testing.T) {
	ctx := context.Background()
	si, ss := nuevoIMAP(t), nuevoSMTP(t)
	si.entregar(t, "INBOX", invitacion)
	b := abrir(t, si, ss)
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	ms, _, _ := b.Messages(ctx, "rol:inbox", "", 10)
	inv, err := b.Invitacion(ctx, ms[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Resumen != "Revisión trimestral" || inv.Lugar != "Sala 3" || inv.Organizador != "luis@ejemplo.eu" || inv.NombreOrganizador != "Luis" ||
		!inv.Inicio.Equal(time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC)) || !inv.Contestable() || inv.TodoElDia {
		t.Fatalf("resumen: %+v", inv)
	}
	copia, enviado, err := b.ResponderInvitacion(ctx, ms[0].ID, RespuestaSi)
	if err != nil || !enviado {
		t.Fatalf("responder: %v %v", enviado, err)
	}
	cp := string(copia)
	if strings.Contains(cp, "METHOD") || !strings.Contains(cp, "PARTSTAT=ACCEPTED") || strings.Contains(cp, "NEEDS-ACTION") || !strings.Contains(cp, "marta@") {
		t.Fatalf("copia: %s", cp)
	}
	b.VaciarSalida(ctx)
	rs := ss.lista()
	if len(rs) != 1 || rs[0].para[0] != "luis@ejemplo.eu" {
		t.Fatalf("REPLY: %+v", rs)
	}
	d := strings.ReplaceAll(string(rs[0].datos), "\r\n", "\n")
	for _, x := range []string{"method=REPLY", "METHOD:REPLY", "UID:evento-1@ejemplo.eu", "SEQUENCE:2", "PARTSTAT=ACCEPTED", "mailto:ana@ejemplo.eu", "Subject: =?utf-8?q?Accepted:"} {
		if !strings.Contains(d, x) {
			t.Errorf("al REPLY le falta %q", x)
		}
	}
	if strings.Contains(d, "marta@") {
		t.Error("el REPLY solo lleva a quien contesta")
	}
}

// ---- Un IMAP de guion, para CONDSTORE y QUOTA (el servidor en memoria no los tiene) ----

type guionIMAP struct {
	ln net.Listener

	mu      sync.Mutex
	ordenes []string
	modseq  uint64
	msgs    []guionMsg
}

type guionMsg struct {
	uid    uint32
	flags  string
	modseq uint64
	crudo  string
}

func nuevoGuion(t *testing.T) *guionIMAP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := &guionIMAP{ln: ln, modseq: 10}
	for i, c := range []string{primero, segundo} {
		g.msgs = append(g.msgs, guionMsg{uid: uint32(i + 1), modseq: 10, crudo: strings.ReplaceAll(c, "\n", "\r\n")})
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go g.servir(conn)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return g
}

func (g *guionIMAP) vistas() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.ordenes)
}

var (
	reOrden   = regexp.MustCompile(`^(\S+) (UID )?(\S+) ?(.*)$`)
	reDesde   = regexp.MustCompile(`CHANGEDSINCE (\d+)`)
	reNumeros = regexp.MustCompile(`^([0-9:,*]+)`)
)

func enConjunto(conjunto string, n uint32) bool {
	for _, r := range strings.Split(conjunto, ",") {
		a, z, rango := strings.Cut(r, ":")
		ini, _ := strconv.ParseUint(a, 10, 32)
		fin := ini
		if rango {
			if z == "*" {
				fin = 1 << 31
			} else {
				fin, _ = strconv.ParseUint(z, 10, 32)
			}
		}
		if ini > fin {
			ini, fin = fin, ini
		}
		if uint64(n) >= ini && uint64(n) <= fin {
			return true
		}
	}
	return false
}

func (g *guionIMAP) servir(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	const caps = "IMAP4rev1 CONDSTORE QUOTA UIDPLUS"
	fmt.Fprintf(w, "* OK [CAPABILITY %s] listo\r\n", caps)
	w.Flush()
	for {
		linea, err := r.ReadString('\n')
		if err != nil {
			return
		}
		linea = strings.TrimRight(linea, "\r\n")
		m := reOrden.FindStringSubmatch(linea)
		if m == nil {
			continue
		}
		tag, uid, orden, resto := m[1], m[2] != "", strings.ToUpper(m[3]), m[4]
		g.mu.Lock()
		g.ordenes = append(g.ordenes, strings.TrimPrefix(linea, tag+" "))
		switch orden {
		case "CAPABILITY":
			fmt.Fprintf(w, "* CAPABILITY %s\r\n%s OK hecho\r\n", caps, tag)
		case "LOGIN", "NOOP", "ENABLE":
			fmt.Fprintf(w, "%s OK hecho\r\n", tag)
		case "LIST":
			fmt.Fprintf(w, "* LIST () \"/\" INBOX\r\n%s OK hecho\r\n", tag)
		case "SELECT", "EXAMINE":
			ultimo := uint32(0)
			for _, x := range g.msgs {
				ultimo = max(ultimo, x.uid)
			}
			fmt.Fprintf(w, "* %d EXISTS\r\n* OK [UIDVALIDITY 7] v\r\n* OK [UIDNEXT %d] n\r\n* OK [HIGHESTMODSEQ %d] m\r\n%s OK [READ-ONLY] hecho\r\n",
				len(g.msgs), ultimo+1, g.modseq, tag)
		case "FETCH":
			conjunto := reNumeros.FindString(resto)
			var desde uint64
			if d := reDesde.FindStringSubmatch(resto); d != nil {
				desde, _ = strconv.ParseUint(d[1], 10, 64)
			}
			for i, x := range g.msgs {
				clave := uint32(i + 1)
				if uid {
					clave = x.uid
				}
				if !enConjunto(conjunto, clave) || (desde > 0 && x.modseq <= desde) {
					continue
				}
				partes := []string{fmt.Sprintf("UID %d", x.uid)}
				if strings.Contains(resto, "FLAGS") {
					partes = append(partes, "FLAGS ("+x.flags+")")
				}
				if strings.Contains(resto, "INTERNALDATE") {
					partes = append(partes, `INTERNALDATE "17-Nov-2026 09:00:00 +0100"`)
				}
				if strings.Contains(resto, "RFC822.SIZE") {
					partes = append(partes, fmt.Sprintf("RFC822.SIZE %d", len(x.crudo)))
				}
				if strings.Contains(resto, "BODY.PEEK[HEADER]") {
					cab, _, _ := strings.Cut(x.crudo, "\r\n\r\n")
					cab += "\r\n\r\n"
					partes = append(partes, fmt.Sprintf("BODY[HEADER] {%d}\r\n%s", len(cab), cab))
				} else if strings.Contains(resto, "BODY.PEEK[]") {
					partes = append(partes, fmt.Sprintf("BODY[] {%d}\r\n%s", len(x.crudo), x.crudo))
				}
				fmt.Fprintf(w, "* %d FETCH (%s)\r\n", i+1, strings.Join(partes, " "))
			}
			fmt.Fprintf(w, "%s OK hecho\r\n", tag)
		case "SEARCH":
			conjunto := strings.TrimPrefix(strings.TrimSpace(resto), "UID ")
			var uids []string
			for _, x := range g.msgs {
				if enConjunto(conjunto, x.uid) {
					uids = append(uids, strconv.Itoa(int(x.uid)))
				}
			}
			fmt.Fprintf(w, "* SEARCH %s\r\n%s OK hecho\r\n", strings.Join(uids, " "), tag)
		case "GETQUOTAROOT":
			fmt.Fprintf(w, "* QUOTAROOT INBOX \"\"\r\n* QUOTA \"\" (STORAGE 512 1024)\r\n%s OK hecho\r\n", tag)
		case "LOGOUT":
			fmt.Fprintf(w, "* BYE adiós\r\n%s OK hecho\r\n", tag)
			g.mu.Unlock()
			w.Flush()
			return
		default:
			fmt.Fprintf(w, "%s BAD no sé\r\n", tag)
		}
		g.mu.Unlock()
		w.Flush()
	}
}

// TestCondstoreNoVuelvePorLasMarcas: con CONDSTORE, una vuelta en la que nada ha
// cambiado no pide marcas; un cambio pide solo lo cambiado (CHANGEDSINCE) y un borrado
// se nota por el EXISTS. Y la cuota de la cuenta llega al medidor.
func TestCondstoreNoVuelvePorLasMarcas(t *testing.T) {
	ctx := context.Background()
	g, ss := nuevoGuion(t), nuevoSMTP(t)
	b, err := Abrir(t.TempDir(), &secretosEnMemoria{m: map[string]string{}}, Red{Marcar: marcarDirecto})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Cerrar)
	if _, err := b.Anadir(Cuenta{
		Proveedor: ProveedorIMAP, Direccion: usuario,
		IMAP: Servidor{Host: "127.0.0.1", Puerto: puerto(g.ln), Seguridad: SeguridadNinguna},
		SMTP: Servidor{Host: "127.0.0.1", Puerto: puerto(ss.ln), Seguridad: SeguridadNinguna},
	}, "clave"); err != nil {
		t.Fatal(err)
	}
	vuelta := func() []string {
		t.Helper()
		antes := len(g.vistas())
		if err := b.Sincronizar(ctx); err != nil {
			t.Fatal(err)
		}
		return g.vistas()[antes:]
	}
	conFetch := func(os []string) []string {
		var out []string
		for _, o := range os {
			if strings.Contains(o, "FETCH") || strings.Contains(o, "SEARCH") {
				out = append(out, o)
			}
		}
		return out
	}
	primera := vuelta()
	if !slices.ContainsFunc(primera, func(o string) bool { return strings.Contains(o, "(CONDSTORE)") }) {
		t.Fatalf("SELECT sin CONDSTORE: %v", primera)
	}
	if ms, _, _ := b.Messages(ctx, "rol:inbox", "", 10); len(ms) != 2 {
		t.Fatalf("primera vuelta: %d", len(ms))
	}
	if q, _ := b.Quota(ctx); q.UsedBytes != 512*1024 || q.QuotaBytes != 1024*1024 {
		t.Fatalf("cuota: %+v", q)
	}

	if f := conFetch(vuelta()); len(f) != 0 {
		t.Fatalf("sin cambios no se pide nada: %v", f)
	}

	g.mu.Lock()
	g.modseq, g.msgs[0].flags, g.msgs[0].modseq = 11, `\Flagged`, 11
	g.mu.Unlock()
	f := conFetch(vuelta())
	if len(f) != 1 || !strings.Contains(f[0], "CHANGEDSINCE 10") {
		t.Fatalf("un cambio de marcas: %v", f)
	}
	ms, _, _ := b.Messages(ctx, "rol:inbox", "", 10)
	if !slices.ContainsFunc(ms, func(m api.MailMessage) bool { return slices.Contains(m.Keywords, api.MailFlagFlagged) }) {
		t.Fatal("la marca no llegó")
	}

	g.mu.Lock()
	g.modseq, g.msgs = 12, g.msgs[:1]
	g.mu.Unlock()
	f = conFetch(vuelta())
	if !slices.ContainsFunc(f, func(o string) bool { return strings.Contains(o, "SEARCH") }) {
		t.Fatalf("un borrado se busca: %v", f)
	}
	if ms, _, _ := b.Messages(ctx, "rol:inbox", "", 10); len(ms) != 1 {
		t.Fatalf("tras el borrado: %d", len(ms))
	}
}

// Un envío que no se puede reclamar (el contexto cortado al cerrar la app, la base
// ocupada) se queda en la bandeja como estaba. Antes se le preguntaban las filas al
// Result nil de ExecContext, y eso era un pánico.
func TestEnvioSinReclamarNoRevienta(t *testing.T) {
	b, err := Abrir(t.TempDir(), &secretosEnMemoria{m: map[string]string{}}, Red{Marcar: marcarDirecto})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Cerrar)
	ctx, cancelar := context.WithCancel(context.Background())
	cancelar()
	b.enviarUno(ctx, envio{id: "1", cuenta: "nadie"})
}
