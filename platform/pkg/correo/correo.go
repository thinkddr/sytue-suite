// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package correo manda avisos de texto por SMTP (submission con STARTTLS) desde un
// servicio de la suite, con la credencial de un buzón propio.
//
// EXISTE PORQUE IBA POR LA TERCERA COPIA. Sytue ID y Cloud mandan correo cada uno
// con su `net/smtp`, y el ERP necesitaba lo mismo para los avisos de obligaciones.
// Aquí están juntas las dos lecciones que costaron un despliegue cada una:
//
//   - el nombre exigido en el certificado puede no ser el de la conexión (dentro
//     del clúster se conecta al Service y el certificado es de `mx.<dominio>`);
//   - con credenciales y sin STARTTLS se falla CERRADO, diciendo por qué.
//
// Solo texto plano: un aviso es una frase y un enlace, y cada parte MIME de más es
// una ocasión más de acabar en la carpeta de spam. La excepción es la invitación de
// calendario (iMIP, RFC 6047), que necesita su parte `text/calendar` para que el
// cliente de quien la recibe la reconozca y la conteste: EnviarCalendario.
package correo

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// Config es el buzón con el que se envía.
type Config struct {
	// Addr es host:puerto del servidor de envío (submission, 587).
	Addr string
	// From es el remitente, con nombre visible si se quiere («Sytue ERP <erp@…>»).
	From string
	// User y Password son la credencial del buzón. Vacíos = sin AUTH.
	User, Password string
	// TLSServerName es el nombre exigido en el certificado si no es el de Addr.
	TLSServerName string
}

// Activo dice si hay con qué enviar.
func (c Config) Activo() bool {
	return strings.TrimSpace(c.Addr) != "" && strings.TrimSpace(c.From) != ""
}

// ErrSinConfigurar es un envío pedido sin servidor ni remitente.
var ErrSinConfigurar = errors.New("correo: sin servidor de envío configurado")

// Enviar manda un mensaje de texto a UN destinatario. Uno por mensaje a propósito:
// un aviso a varias personas en un solo correo enseña a cada una la dirección de
// las demás.
func Enviar(ctx context.Context, c Config, para, asunto, cuerpo string) error {
	return enviar(ctx, c, para, func(from, to string) []byte {
		return Mensaje(from, to, asunto, cuerpo, time.Now())
	})
}

// Opciones cambian, en un envío, cómo se presenta el remitente. La DIRECCIÓN del
// remitente sigue siendo siempre la de Config.From (la que el relé firma con DKIM):
// un nombre que manda un tercero no puede fabricar otra.
type Opciones struct {
	// NombreRemitente sustituye el nombre visible de Config.From («Panadería Aurora
	// (vía Sytue Web)»). Se entrecomilla o se codifica: con «"» o «<» dentro no
	// abre otra dirección.
	NombreRemitente string
	// ResponderA va en la cabecera Reply-To. Nunca es destinatario: el correo sale
	// solo para `para`.
	ResponderA string
}

// EnviarCon es Enviar con Opciones: el correo de una tienda sale de la dirección de
// la suite, con el nombre de la tienda y respondiendo a la tienda.
func EnviarCon(ctx context.Context, c Config, op Opciones, para, asunto, cuerpo string) error {
	if !c.Activo() {
		return ErrSinConfigurar
	}
	from, responder, err := presentar(c.From, op)
	if err != nil {
		return err
	}
	return enviar(ctx, c, para, func(_, to string) []byte {
		return mensaje(from, to, responder, asunto, cuerpo, time.Now())
	})
}

// presentar compone el From y el Reply-To de un envío con Opciones. Los dos salen de
// mail.Address.String(), que entrecomilla o codifica el nombre; el Reply-To tiene que
// ser UNA dirección y va sin nombre.
func presentar(from string, op Opciones) (remitente, responder string, err error) {
	remitente = from
	if op.NombreRemitente != "" {
		a, err := mail.ParseAddress(from)
		if err != nil {
			return "", "", fmt.Errorf("correo: remitente inválido: %w", err)
		}
		remitente = (&mail.Address{Name: op.NombreRemitente, Address: a.Address}).String()
	}
	if op.ResponderA != "" {
		a, err := mail.ParseAddress(op.ResponderA)
		if err != nil {
			return "", "", fmt.Errorf("correo: Reply-To inválido: %w", err)
		}
		responder = (&mail.Address{Address: a.Address}).String()
	}
	return remitente, responder, nil
}

// EnviarCalendario manda una invitación iMIP (RFC 6047) a UN destinatario: el texto
// para quien lo lea a mano y la parte `text/calendar; method=<metodo>` con el objeto
// iTIP (REQUEST, CANCEL…) para que su cliente de correo la pinte y la conteste.
func EnviarCalendario(ctx context.Context, c Config, para, asunto, cuerpo string, ics []byte, metodo string) error {
	return enviar(ctx, c, para, func(from, to string) []byte {
		return MensajeCalendario(from, to, asunto, cuerpo, ics, metodo, time.Now())
	})
}

// enviar es la conversación SMTP común: `construir` arma el mensaje con el remitente
// y el destinatario ya validados.
func enviar(ctx context.Context, c Config, para string, construir func(from, to string) []byte) error {
	if !c.Activo() {
		return ErrSinConfigurar
	}
	destino, err := mail.ParseAddress(para)
	if err != nil {
		return fmt.Errorf("correo: destinatario inválido: %w", err)
	}
	remitente, err := mail.ParseAddress(c.From)
	if err != nil {
		return fmt.Errorf("correo: remitente inválido: %w", err)
	}
	host, _, err := net.SplitHostPort(c.Addr)
	if err != nil {
		return fmt.Errorf("correo: dirección del servidor %q: %w", c.Addr, err)
	}
	nombreTLS := c.TLSServerName
	if nombreTLS == "" {
		nombreTLS = host
	}

	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", c.Addr)
	if err != nil {
		return fmt.Errorf("correo: conectar con %s: %w", c.Addr, err)
	}
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	cli, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("correo: sesión SMTP: %w", err)
	}
	defer func() { _ = cli.Close() }()

	if ok, _ := cli.Extension("STARTTLS"); ok {
		if err := cli.StartTLS(&tls.Config{ServerName: nombreTLS, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("correo: STARTTLS con %s (nombre exigido %q): %w", c.Addr, nombreTLS, err)
		}
	} else if c.User != "" {
		return fmt.Errorf("correo: %s no anuncia STARTTLS y hay credenciales: no se mandan en claro", c.Addr)
	}
	if c.User != "" {
		// El host de PlainAuth es el de la CONEXIÓN: el certificado ya se comprobó
		// contra nombreTLS en el StartTLS.
		if err := cli.Auth(smtp.PlainAuth("", c.User, c.Password, host)); err != nil {
			return fmt.Errorf("correo: autenticar en %s: %w", c.Addr, err)
		}
	}
	if err := cli.Mail(remitente.Address); err != nil {
		return fmt.Errorf("correo: remitente rechazado: %w", err)
	}
	if err := cli.Rcpt(destino.Address); err != nil {
		return fmt.Errorf("correo: destinatario rechazado: %w", err)
	}
	w, err := cli.Data()
	if err != nil {
		return fmt.Errorf("correo: cuerpo: %w", err)
	}
	if _, err := w.Write(construir(c.From, destino.String())); err != nil {
		_ = w.Close()
		return fmt.Errorf("correo: escribir: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("correo: cerrar: %w", err)
	}
	return cli.Quit()
}

// Mensaje arma el correo RFC 5322: cabeceras sin saltos de línea (un salto en el
// asunto escribiría cabeceras de más), asunto codificado para que las tildes
// lleguen, y el cuerpo con finales CRLF.
func Mensaje(from, to, asunto, cuerpo string, fecha time.Time) []byte {
	return mensaje(from, to, "", asunto, cuerpo, fecha)
}

func mensaje(from, to, responderA, asunto, cuerpo string, fecha time.Time) []byte {
	var b strings.Builder
	cabeceras(&b, from, to, asunto, fecha)
	if responderA != "" {
		b.WriteString("Reply-To: " + strings.NewReplacer("\r", " ", "\n", " ").Replace(responderA) + "\r\n")
	}
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(crlf(cuerpo))
	b.WriteString("\r\n")
	return []byte(b.String())
}

// cabeceras escribe las comunes: sin saltos de línea (un salto en el asunto
// escribiría cabeceras de más) y con el asunto codificado para que las tildes lleguen.
func cabeceras(b *strings.Builder, from, to, asunto string, fecha time.Time) {
	limpio := strings.NewReplacer("\r", " ", "\n", " ")
	dominio := "localhost"
	if a, err := mail.ParseAddress(from); err == nil {
		if _, d, ok := strings.Cut(a.Address, "@"); ok {
			dominio = d
		}
	}
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	b.WriteString("From: " + limpio.Replace(from) + "\r\n")
	b.WriteString("To: " + limpio.Replace(to) + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", limpio.Replace(asunto)) + "\r\n")
	b.WriteString("Date: " + fecha.Format(time.RFC1123Z) + "\r\n")
	b.WriteString("Message-ID: <" + hex.EncodeToString(id) + "@" + dominio + ">\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
}

func crlf(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
}

// MensajeCalendario es el mensaje iMIP: `multipart/alternative` con el texto y la
// parte `text/calendar; method=…` (RFC 6047 §2.4). El iCalendar va en base64: sus
// líneas plegadas a 75 octetos no necesitan más, pero un DESCRIPTION con tildes y un
// relé que recodifique el 8bit son justo cómo se rompe una firma o una invitación.
func MensajeCalendario(from, to, asunto, cuerpo string, ics []byte, metodo string, fecha time.Time) []byte {
	metodo = strings.ToUpper(strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r == '-' {
			return r
		}
		return -1
	}, metodo))
	limite := make([]byte, 12)
	_, _ = rand.Read(limite)
	frontera := "sytue-" + hex.EncodeToString(limite)
	var b strings.Builder
	cabeceras(&b, from, to, asunto, fecha)
	b.WriteString("Content-Type: multipart/alternative; boundary=\"" + frontera + "\"\r\n\r\n")
	b.WriteString("--" + frontera + "\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(crlf(cuerpo) + "\r\n")
	b.WriteString("--" + frontera + "\r\n")
	b.WriteString("Content-Type: text/calendar; charset=utf-8; method=" + metodo + "\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	enc := base64.StdEncoding.EncodeToString(ics)
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc + "\r\n")
	b.WriteString("--" + frontera + "--\r\n")
	return []byte(b.String())
}
