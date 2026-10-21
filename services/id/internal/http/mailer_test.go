// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/mail"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"
)

// El MSA de Sytue Mail compara el MAIL FROM con la identidad SASL, así que el
// usuario tiene que ser la dirección DESNUDA. Mandar «Sytue <no-reply@…>» hace
// fallar el AUTH y el correo no sale, que es exactamente el fallo que este
// camino viene a arreglar.
func TestMailAddressOnlyDesnudaElRemitente(t *testing.T) {
	casos := map[string]string{
		"Sytue <no-reply@sytue.eu>":  "no-reply@sytue.eu",
		"no-reply@sytue.eu":          "no-reply@sytue.eu",
		"\"Sytue, S.L.\" <a@b.test>": "a@b.test",
	}
	for entrada, esperado := range casos {
		if got := mailAddressOnly(entrada); got != esperado {
			t.Errorf("mailAddressOnly(%q) = %q, se esperaba %q", entrada, got, esperado)
		}
	}
}

// Sin SMTP se cae al mailer de log (dev). Con SMTP y sin credenciales estáticas
// se usa la identidad de servicio del propio IdP: el usuario SASL pasa a ser el
// remitente y el secreto se firma en cada envío.
func TestNewMailerEligeLaIdentidadDeServicio(t *testing.T) {
	log := slog.Default()

	if _, ok := newMailer(Config{}, log, nil).(logMailer); !ok {
		t.Fatal("sin SMTP_ADDR debería usarse el mailer de log")
	}

	cfg := Config{SMTPAddr: "sytue-one-mail-mx:587", MailFrom: "Sytue <no-reply@sytue.eu>"}
	m, ok := newMailer(cfg, log, func() (string, error) { return "jwt", nil }).(*smtpMailer)
	if !ok {
		t.Fatal("con SMTP_ADDR debería usarse el mailer SMTP")
	}
	if m.user != "no-reply@sytue.eu" {
		t.Errorf("usuario SASL = %q, se esperaba la dirección desnuda", m.user)
	}
	if m.mintToken == nil {
		t.Error("sin credenciales estáticas debería firmarse un token por envío")
	}
	if m.host != "sytue-one-mail-mx" {
		t.Errorf("host TLS = %q, se esperaba sin el puerto", m.host)
	}

	// Un relay ajeno con credenciales propias manda sobre la identidad de
	// servicio: no se le presenta un JWT que no sabría validar.
	cfg.SMTPUsername, cfg.SMTPPassword = "usuario", "clave"
	m2, _ := newMailer(cfg, log, func() (string, error) { return "jwt", nil }).(*smtpMailer)
	if m2.mintToken != nil || m2.user != "usuario" {
		t.Error("con SMTP_USERNAME deberían mandar las credenciales estáticas")
	}
}

// --- El diálogo SMTP de verdad, contra un MSA que habla la misma librería ------

// recibido es lo que el MSA vio: el sobre y el mensaje completo.
type recibido struct {
	from  string
	rcpts []string
	datos string
}

type buzonPrueba struct{ ch chan recibido }

func (b *buzonPrueba) NewSession(*gosmtp.Conn) (gosmtp.Session, error) {
	return &sesionPrueba{b: b}, nil
}

type sesionPrueba struct {
	b   *buzonPrueba
	rec recibido
}

// Sin mecanismos anunciados no se ofrece AUTH, que es justo lo que hace falta
// para poder mirar el sobre y el cuerpo sin montar una autoridad de certificación.
func (s *sesionPrueba) AuthMechanisms() []string { return nil }

func (s *sesionPrueba) Auth(string) (sasl.Server, error) { return nil, gosmtp.ErrAuthUnsupported }

func (s *sesionPrueba) Mail(from string, _ *gosmtp.MailOptions) error {
	s.rec.from = from
	return nil
}

func (s *sesionPrueba) Rcpt(to string, _ *gosmtp.RcptOptions) error {
	s.rec.rcpts = append(s.rec.rcpts, to)
	return nil
}

func (s *sesionPrueba) Data(r io.Reader) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.rec.datos = string(raw)
	s.b.ch <- s.rec
	return nil
}

func (s *sesionPrueba) Reset() {}

func (s *sesionPrueba) Logout() error { return nil }

// msaFalso levanta un servidor de submission con emersion/go-smtp, la MISMA
// librería sobre la que corre el MSA de Sytue Mail. Un doble escrito a mano
// aceptaría cosas que el servidor real rechaza, y el fallo que se busca aquí vive
// precisamente en los detalles del diálogo.
func msaFalso(t *testing.T) (string, <-chan recibido) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	b := &buzonPrueba{ch: make(chan recibido, 1)}
	s := gosmtp.NewServer(b)
	s.Domain = "msa.test"
	s.MaxMessageBytes = 1 << 20
	s.MaxRecipients = 10
	go func() { _ = s.Serve(l) }()
	t.Cleanup(func() { _ = s.Close() })
	return l.Addr().String(), b.ch
}

// El correo de invitación tiene que llegar entero al MSA y en condiciones de
// sobrevivir al filtro del destinatario.
//
// Le faltaba el Message-ID: el MSA respondía 250 y el correo moría después, en
// casa del invitado, que es el peor sitio donde perderlo porque de este lado todo
// indica que salió bien. Se comprueba también que el envelope-from vaya desnudo:
// el MSA compara el MAIL FROM con la identidad autenticada y un «Sytue <...>»
// entero no coincide con nada.
func TestSmtpMailerEntregaLaInvitacionCompleta(t *testing.T) {
	addr, recibidos := msaFalso(t)
	cfg := Config{SMTPAddr: addr, MailFrom: "Sytue <no-reply@sytue.eu>"}
	m := newMailer(cfg, slog.Default(), nil)

	const enlace = "https://id.sytue.eu/accept-invitation?token=abc.def"
	if err := m.SendInvitation(context.Background(), "nuevo@ejemplo.test", "ACME", enlace); err != nil {
		t.Fatalf("SendInvitation: %v", err)
	}

	var got recibido
	select {
	case got = <-recibidos:
	case <-time.After(10 * time.Second):
		t.Fatal("el MSA no recibió ningún mensaje")
	}

	if got.from != "no-reply@sytue.eu" {
		t.Errorf("MAIL FROM = %q, se esperaba la dirección desnuda", got.from)
	}
	if len(got.rcpts) != 1 || got.rcpts[0] != "nuevo@ejemplo.test" {
		t.Errorf("RCPT TO = %v, se esperaba solo el invitado", got.rcpts)
	}

	msg, err := mail.ReadMessage(strings.NewReader(got.datos))
	if err != nil {
		t.Fatalf("el mensaje no es un RFC 5322 válido: %v", err)
	}
	id := msg.Header.Get("Message-Id")
	if !strings.HasPrefix(id, "<") || !strings.HasSuffix(id, "@sytue.eu>") {
		t.Errorf("Message-ID = %q; sin él los buzones grandes tratan el correo como automatismo mal formado", id)
	}
	if to := msg.Header.Get("To"); to != "nuevo@ejemplo.test" {
		t.Errorf("To = %q", to)
	}
	if asunto := msg.Header.Get("Subject"); asunto != "Te han invitado a Sytue" {
		t.Errorf("Subject = %q", asunto)
	}
	cuerpo, err := io.ReadAll(msg.Body)
	if err != nil {
		t.Fatalf("leer el cuerpo: %v", err)
	}
	for _, quiero := range []string{"ACME", enlace} {
		if !strings.Contains(string(cuerpo), quiero) {
			t.Errorf("el cuerpo no menciona %q:\n%s", quiero, cuerpo)
		}
	}
}

// El secreto que se le presenta al MSA es un access token de Sytue ID con scope
// de correo: vale para leer el buzón del remitente, no solo para enviar. Si el
// servidor no ofrece STARTTLS, el envío se cae en vez de sacarlo por un canal en
// claro, y el mensaje no llega a salir.
func TestSmtpMailerNoPresentaElTokenSinCifrar(t *testing.T) {
	addr, recibidos := msaFalso(t)
	cfg := Config{SMTPAddr: addr, MailFrom: "Sytue <no-reply@sytue.eu>"}
	m := newMailer(cfg, slog.Default(), func() (string, error) { return "jwt-de-prueba", nil })

	err := m.SendInvitation(context.Background(), "nuevo@ejemplo.test", "ACME",
		"https://id.sytue.eu/accept-invitation?token=abc.def")
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("con credenciales y sin STARTTLS el envío debía fallar: %v", err)
	}
	select {
	case got := <-recibidos:
		t.Fatalf("el mensaje salió igualmente por un canal en claro: %+v", got)
	default:
	}
}

// TestElTokenDelRemitenteAcreditaSuDireccion: el token con que el IdP entra en el MSA
// de Sytue Mail tiene que llevar la dirección del remitente. IssueAccessToken solo
// la escribe con el scope `email`, y Mail rechaza un token sin ninguna (COM-14,
// TestElTokenSinDireccionNoAcreditaNinguna). Con `mail` a secas, desde v3.12.6 no
// salió ni un correo del IdP: «535 5.7.8 Authentication failed» en cada invitación.
// Visto en rojo dejando el scope en "mail".
func TestElTokenDelRemitenteAcreditaSuDireccion(t *testing.T) {
	c := claimsDelRemitente(Config{MailFrom: "Sytue <no-reply@sytue.eu>", PlatformOrgSlug: "sytue"})
	scopes := strings.Fields(c.Scope)
	if !slices.Contains(scopes, "mail") || !slices.Contains(scopes, "email") {
		t.Fatalf("scope = %q: Mail exige `mail` y la dirección solo viaja con `email`", c.Scope)
	}
	// La misma que el usuario SASL y el MAIL FROM (newMailer): si no, Mail la rechaza
	// por no coincidir con el token.
	if c.Email != "no-reply@sytue.eu" || !c.EmailVerified {
		t.Fatalf("email = %q (verificado %v), quiero no-reply@sytue.eu verificado", c.Email, c.EmailVerified)
	}
	if c.Subject != mailServiceSubject || c.Audience != mailServiceAudience || c.Tenant != "sytue" {
		t.Fatalf("identidad del remitente = %q/%q/%q", c.Subject, c.Audience, c.Tenant)
	}
}
