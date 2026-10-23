// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/id/internal/oidc"
)

const (
	// mailServiceSubject identifica al remitente transaccional del IdP dentro de
	// Sytue Mail. Estable a propósito: Mail indexa las cuentas por (tenant,
	// subject), así que cambiarlo crearía un buzón nuevo y abandonaría el anterior.
	mailServiceSubject = "sytue-id-transactional"
	// mailServiceAudience es la audiencia que exige Sytue Mail (OIDC_AUDIENCE).
	mailServiceAudience = "sytue-mail"
)

// claimsDelRemitente son los del token con que el IdP se autentica en el MSA de
// Sytue Mail, uno nuevo por envío. El scope lleva `email` además de `mail` porque
// IssueAccessToken solo escribe la dirección si se concede `email`, y Mail rechaza
// un token que no acredite ninguna (COM-14). Sin ella, desde v3.12.6 no salía ni un
// correo del IdP: invitaciones, verificación y recuperación de contraseña.
//
// El subject es estable (no un UUID por envío) para que el buzón que Mail
// aprovisiona la primera vez sea SIEMPRE el mismo, y por tanto el «Enviados» de
// no-reply no se fragmente en una cuenta por correo.
func claimsDelRemitente(cfg Config) oidc.TokenClaims {
	return oidc.TokenClaims{
		Subject:       mailServiceSubject,
		Audience:      mailServiceAudience,
		Scope:         "mail email",
		Tenant:        cfg.PlatformOrgSlug,
		Email:         mailAddressOnly(cfg.MailFrom),
		EmailVerified: true,
		// Minutos, no horas: el token solo tiene que sobrevivir al diálogo SMTP
		// que lo acaba de pedir.
		TTL: 5 * time.Minute,
	}
}

// Mailer envía los correos transaccionales del IdP (el enlace de reseteo de
// contraseña y el de invitación a una organización). Una interfaz mínima permite
// el fallback de dev (log) sin bloquear el flujo cuando no hay SMTP, y un envío
// SMTP real en prod, sin acoplar el servicio de correo de la suite (services/mail
// es un servidor IMAP/JMAP/SMTP completo, no un cliente de envío; aquí basta
// net/smtp de la stdlib).
type Mailer interface {
	SendPasswordReset(ctx context.Context, to, link string) error
	// SendInvitation envía el enlace de aceptación de una invitación a `org`
	// (nombre visible de la organización) donde el invitado fija su contraseña.
	SendInvitation(ctx context.Context, to, org, link string) error
	// SendEmailVerification envía el enlace de verificación de la dirección `to`.
	SendEmailVerification(ctx context.Context, to, link string) error
	// SendPlatformScopeChange avisa de que alguien ha recibido o perdido el
	// alcance de plataforma. `sujeto` es a quién le pasa, `actor` quién lo hizo.
	// No lleva enlaces: no hay nada que aceptar ni confirmar, solo enterarse.
	SendPlatformScopeChange(ctx context.Context, to, sujeto, actor, motivo string, concedido bool) error
}

// newMailer elige la implementación según la configuración: con SMTP_ADDR ⇒ envío
// real; sin él ⇒ logMailer (dev), que registra el enlace y NO bloquea el flujo.
// Esa segunda rama solo existe para el desarrollo local: en producción el
// servicio no llega a construirse sin SMTP_ADDR (ver Load en config.go).
//
// `mintToken` permite autenticarse en el MSA de Sytue Mail, que NO acepta una
// contraseña estática: exige un access token de Sytue ID con scope `mail` cuyo
// email coincida con el remitente. Como el emisor de esos tokens es este mismo
// servicio, se firma uno corto en cada envío en lugar de custodiar una
// credencial eterna. Si hay SMTP_USERNAME/PASSWORD, mandan esos (relay ajeno).
func newMailer(cfg Config, logger *slog.Logger, mintToken func() (string, error)) Mailer {
	if cfg.SMTPAddr == "" {
		return logMailer{log: logger}
	}
	host := cfg.SMTPAddr
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	m := &smtpMailer{
		addr: cfg.SMTPAddr,
		host: host,
		from: cfg.MailFrom,
		user: cfg.SMTPUsername,
		pass: cfg.SMTPPassword,
		// Se conecta al Service y exige el nombre del host de correo: dentro
		// del clúster no hay certificado para el nombre del Service.
		tlsServerName: cfg.SMTPTLSServerName,
	}
	if m.user == "" && mintToken != nil {
		// El usuario SASL es la dirección remitente, tal como exige el MSA
		// (`Mail()` compara MAIL FROM con la identidad autenticada).
		m.user = mailAddressOnly(cfg.MailFrom)
		m.mintToken = mintToken
	}
	return m
}

// mailAddressOnly extrae la dirección de un From con nombre visible
// («Sytue <no-reply@sytue.eu>» ⇒ «no-reply@sytue.eu»). El SASL y el MAIL FROM
// llevan la dirección desnuda; mandar el nombre completo hace fallar el AUTH.
func mailAddressOnly(from string) string {
	if a, err := mail.ParseAddress(from); err == nil {
		return a.Address
	}
	return from
}

// logMailer es el Mailer de desarrollo: sin SMTP configurado, registra el enlace
// de reseteo en el log (WARN) para que el desarrollador lo copie. En producción
// no es alcanzable: Load() se niega a levantar el servicio sin SMTP_ADDR, porque
// este mailer devuelve nil y desde fuera un envío que no ocurrió es idéntico a
// uno que sí. Aparte, el enlace es material sensible y no debe vivir en un log.
type logMailer struct{ log *slog.Logger }

func (m logMailer) SendPasswordReset(ctx context.Context, to, link string) error {
	m.log.WarnContext(ctx, "SMTP no configurado: enlace de reseteo SOLO en el log (dev)",
		"to", to, "reset_link", link)
	return nil
}

func (m logMailer) SendInvitation(ctx context.Context, to, org, link string) error {
	m.log.WarnContext(ctx, "SMTP no configurado: enlace de invitación SOLO en el log (dev)",
		"to", to, "org", org, "invite_link", link)
	return nil
}

func (m logMailer) SendEmailVerification(ctx context.Context, to, link string) error {
	m.log.WarnContext(ctx, "SMTP no configurado: enlace de verificación SOLO en el log (dev)",
		"to", to, "verify_link", link)
	return nil
}

func (m logMailer) SendPlatformScopeChange(ctx context.Context, to, sujeto, actor, motivo string, concedido bool) error {
	m.log.WarnContext(ctx, "SMTP no configurado: aviso de alcance de plataforma SOLO en el log (dev)",
		"to", to, "sujeto", sujeto, "actor", actor, "concedido", concedido, "motivo", motivo)
	return nil
}

// smtpMailer envía por SMTP con net/smtp (stdlib), con el diálogo explícito para
// poder exigir un nombre de certificado distinto de la dirección de conexión.
type smtpMailer struct {
	addr, host, from, user, pass string
	// tlsServerName: nombre exigido en el certificado, si difiere del de addr.
	tlsServerName string
	// mintToken firma un access token nuevo para cada envío (MSA de Sytue Mail).
	// nil ⇒ se usa `pass` tal cual (relay SMTP clásico).
	mintToken func() (string, error)
}

func (m *smtpMailer) SendPasswordReset(ctx context.Context, to, link string) error {
	// net/smtp es bloqueante y no toma context; el llamante ya invoca en segundo
	// plano (respuesta uniforme del forgot-password), así que ctx solo documenta.
	_ = ctx
	return m.send(to, buildResetEmail(m.from, to, link))
}

func (m *smtpMailer) SendInvitation(ctx context.Context, to, org, link string) error {
	_ = ctx
	return m.send(to, buildInviteEmail(m.from, to, org, link))
}

func (m *smtpMailer) SendEmailVerification(ctx context.Context, to, link string) error {
	_ = ctx
	return m.send(to, buildVerifyEmail(m.from, to, link))
}

func (m *smtpMailer) SendPlatformScopeChange(ctx context.Context, to, sujeto, actor, motivo string, concedido bool) error {
	_ = ctx
	return m.send(to, buildPlatformScopeEmail(m.from, to, sujeto, actor, motivo, concedido))
}

// send entrega un mensaje ya construido por SMTP. Compartido por reseteo,
// invitación y verificación.
//
// No usa `smtp.SendMail` porque ese ayudante no deja fijar el nombre del
// certificado: derivaría la comprobación TLS de la dirección de conexión, y
// dentro del clúster esa dirección es el nombre del Service, que ningún
// certificado público cubre. Con el diálogo explícito se conecta a un sitio y se
// exige otro nombre, que es verificación de verdad y no una excepción.
//
// Y falla CERRADO si hay credenciales que enviar y el servidor no ofrece
// STARTTLS. La stdlib ya lo hacía, pero con un «unencrypted connection» que no
// dice de quién es la culpa: costó una noche averiguar que el MSA no lo
// anunciaba porque se creía en desarrollo.
func (m *smtpMailer) send(to string, msg []byte) error {
	// Defensa en profundidad contra inyección de comandos/cabeceras SMTP: valida el
	// destinatario con el parser de la stdlib (rechaza CR/LF y basura) ANTES de abrir
	// el diálogo. El mensaje ya construido no sale si esto falla.
	addr, err := mail.ParseAddress(to)
	if err != nil {
		return fmt.Errorf("mailer: destinatario inválido: %w", err)
	}
	secret := m.pass
	if m.mintToken != nil {
		// Token corto y de un solo uso: si falla, el envío falla en vez de
		// intentar un AUTH que el MSA rechazaría igualmente.
		t, err := m.mintToken()
		if err != nil {
			return fmt.Errorf("mailer: no se pudo firmar el token de envío: %w", err)
		}
		secret = t
	}
	nombreTLS := m.tlsServerName
	if nombreTLS == "" {
		nombreTLS = m.host
	}

	c, err := smtp.Dial(m.addr)
	if err != nil {
		return fmt.Errorf("mailer: conectar con %s: %w", m.addr, err)
	}
	defer func() { _ = c.Close() }()

	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: nombreTLS, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("mailer: STARTTLS con %s (nombre exigido %q): %w", m.addr, nombreTLS, err)
		}
	} else if m.user != "" {
		return fmt.Errorf(
			"mailer: %s no anuncia STARTTLS y hay credenciales que enviar; "+
				"no se manda un token por un canal en claro", m.addr,
		)
	}

	if m.user != "" {
		// El host de PlainAuth es el de la CONEXIÓN, no el del certificado: la
		// stdlib lo compara con el nombre con el que se abrió el diálogo y aborta
		// si no coincide («wrong host name»). Son dos comprobaciones distintas y
		// confundirlas cuesta un despliegue: el certificado ya quedó verificado
		// contra `nombreTLS` en el StartTLS de arriba, que es donde importa.
		if err := c.Auth(smtp.PlainAuth("", m.user, secret, m.host)); err != nil {
			return fmt.Errorf("mailer: autenticar en %s: %w", m.addr, err)
		}
	}
	// G707: el análisis de taint no reconoce el filtrado de senderAddress como
	// saneamiento, y marca la ruta config -> MAIL FROM. La dirección sale de la
	// configuración del operador (cfg.MailFrom), no de una petición, y
	// senderAddress ya elimina CR, LF y NUL, que es lo único con lo que se podría
	// partir el comando.
	if err := c.Mail(senderAddress(m.from)); err != nil { //nolint:gosec // Ver arriba: origen de configuración y CR/LF/NUL filtrados en senderAddress.
		return fmt.Errorf("mailer: remitente rechazado: %w", err)
	}
	if err := c.Rcpt(addr.Address); err != nil {
		return fmt.Errorf("mailer: destinatario rechazado: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mailer: abrir el cuerpo: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("mailer: escribir el cuerpo: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mailer: cerrar el cuerpo: %w", err)
	}
	return c.Quit()
}

// mailHeaders escribe la cabecera RFC 5322 común a todos los transaccionales y
// deja el flujo listo para el cuerpo. CRLF en todas partes, como exige el estándar.
//
// Los cuatro mensajes repetían estas siete líneas y a los cuatro les faltaba la
// misma: el Message-ID. Un correo sin él sale del MSA con un 250 impecable y
// muere después, en el filtro del destinatario, porque los buzones grandes lo
// leen como automatismo mal formado; y ese es el peor sitio donde perderlo,
// porque de este lado todo parece haber ido bien. Encima el firmante DKIM ya lo
// lleva en su lista de cabeceras firmadas, así que no emitirlo dejaba hueco para
// que cualquiera en el camino añadiera el suyo sin romper la firma.
func mailHeaders(b *strings.Builder, from, to, subject string) {
	fmt.Fprintf(b, "From: %s\r\n", from)
	fmt.Fprintf(b, "To: %s\r\n", to)
	fmt.Fprintf(b, "Subject: %s\r\n", subject)
	fmt.Fprintf(b, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	fmt.Fprintf(b, "Message-ID: <%s@%s>\r\n", uuid.NewString(), messageIDDomain(from))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
}

func textoCorreo(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ", "\x00", " ").Replace(s)
}

// messageIDDomain toma el dominio del remitente, que es el que avala la firma
// DKIM: poner otro convertiría el identificador en una incoherencia más que
// sumar al filtro antispam. Un From sin arroba es configuración rota y no algo
// que deba impedir el envío, así que se cae al dominio de la suite.
func messageIDDomain(from string) string {
	dir := mailAddressOnly(from)
	if i := strings.LastIndexByte(dir, '@'); i >= 0 && i < len(dir)-1 {
		return dir[i+1:]
	}
	return "sytue.eu"
}

// buildResetEmail arma el mensaje (texto plano) de reseteo con el enlace.
func buildResetEmail(from, to, link string) []byte {
	var b strings.Builder
	mailHeaders(&b, from, to, "Restablece tu contrasena de Sytue")
	b.WriteString("Has solicitado restablecer tu contrasena de Sytue.\r\n\r\n")
	b.WriteString("Abre este enlace para elegir una nueva (caduca en breve y es de un solo uso):\r\n\r\n")
	fmt.Fprintf(&b, "%s\r\n\r\n", textoCorreo(link))
	b.WriteString("Si no fuiste tu, ignora este mensaje: tu contrasena no cambia.\r\n")
	return []byte(b.String())
}

// buildInviteEmail arma el mensaje (texto plano) de invitación a una organización
// con el enlace de aceptación. org es el nombre visible de la organización.
func buildInviteEmail(from, to, org, link string) []byte {
	var b strings.Builder
	mailHeaders(&b, from, to, "Te han invitado a Sytue")
	fmt.Fprintf(&b, "Te han invitado a unirte a %s en Sytue.\r\n\r\n", textoCorreo(org))
	b.WriteString("Abre este enlace para aceptar la invitacion y elegir tu contrasena ")
	b.WriteString("(caduca en breve y es de un solo uso):\r\n\r\n")
	fmt.Fprintf(&b, "%s\r\n\r\n", textoCorreo(link))
	b.WriteString("Si no esperabas esta invitacion, puedes ignorar este mensaje.\r\n")
	return []byte(b.String())
}

// buildVerifyEmail arma el mensaje (texto plano) de verificación de email con el
// enlace de confirmación. Mismo formato RFC 5322 mínimo que el reseteo.
func buildVerifyEmail(from, to, link string) []byte {
	var b strings.Builder
	mailHeaders(&b, from, to, "Verifica tu correo en Sytue")
	b.WriteString("Confirma que esta direccion es tuya para completar tu cuenta de Sytue.\r\n\r\n")
	b.WriteString("Abre este enlace para verificarla (caduca en breve y es de un solo uso):\r\n\r\n")
	fmt.Fprintf(&b, "%s\r\n\r\n", textoCorreo(link))
	b.WriteString("Si no esperabas este mensaje, puedes ignorarlo.\r\n")
	return []byte(b.String())
}

// buildPlatformScopeEmail redacta el aviso de un cambio de alcance de plataforma.
//
// Va a quien lo recibe o lo pierde Y al resto de administradores globales: que
// alguien gane el permiso más amplio del sistema sin que nadie más se entere es
// exactamente el agujero que esto cierra. El registro ya lo escribía, pero un
// registro solo lo lee quien va a mirarlo, y a estas cosas se llega tarde.
//
// Sin enlaces: no hay nada que aceptar ni confirmar. Si el aviso sorprende, lo
// que toca es hablar con quien aparece como autor, y por eso el autor va dentro.
func buildPlatformScopeEmail(from, to, sujeto, actor, motivo string, concedido bool) []byte {
	var b strings.Builder
	asunto := "Alcance de administracion de plataforma retirado"
	if concedido {
		asunto = "Alcance de administracion de plataforma concedido"
	}
	mailHeaders(&b, from, to, asunto)
	if concedido {
		fmt.Fprintf(&b, "%s puede administrar la plataforma de Sytue: todas las organizaciones.\r\n", textoCorreo(sujeto))
	} else {
		fmt.Fprintf(&b, "%s ya no puede administrar la plataforma de Sytue.\r\n", textoCorreo(sujeto))
	}
	fmt.Fprintf(&b, "Lo ha hecho %s.\r\n", textoCorreo(actor))
	if motivo != "" {
		fmt.Fprintf(&b, "Motivo: %s\r\n", textoCorreo(motivo))
	}
	b.WriteString("\r\n")
	b.WriteString("Poder administrar la plataforma no es estar administrandola: hace falta\r\n")
	b.WriteString("activarlo cada vez, diciendo para que, y caduca solo.\r\n\r\n")
	b.WriteString("Si esto no deberia haber pasado, habla con quien figura arriba.\r\n")
	return []byte(b.String())
}

// senderAddress extrae la dirección del envelope-from de una cabecera From que
// puede llevar display name ("Sytue <no-reply@sytue.eu>" ⇒ "no-reply@sytue.eu").
func senderAddress(from string) string {
	dir := strings.TrimSpace(from)
	if i := strings.LastIndexByte(from, '<'); i >= 0 {
		if j := strings.IndexByte(from[i:], '>'); j >= 0 {
			dir = from[i+1 : i+j]
		}
	}
	// Un CR o LF aquí partiría el comando MAIL FROM en dos y lo que fuera detrás
	// llegaría al servidor SMTP como un comando propio. Hoy el valor sale de la
	// configuración del operador y no de nadie de fuera, así que no hay ataque
	// posible; se corta igualmente porque el coste es una línea y el día que esta
	// función reciba una dirección de otro sitio, el agujero ya no estará.
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == 0 {
			return -1
		}
		return r
	}, dir)
}
