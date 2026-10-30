// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
)

// ---------------------------------------------------------------------------
// Aviso por correo
// ---------------------------------------------------------------------------
//
// El notificador solo sabía hablar con webhooks, y su propio comentario llevaba
// desde la Ola B diciendo «cuando exista un helper de correo». Sigue sin existir
// en platform/, y crearlo ahí es tocar un paquete que usan siete servicios: para
// mandar un aviso de veinte líneas basta `net/smtp`, que es stdlib.
//
// El canal de una alerta ya es una URL, así que un `mailto:` entra sin columna
// nueva, sin migración y sin un segundo camino en el consumidor.

// mailerMissingOnce evita repetir el aviso de «sin SMTP configurado» en cada
// alerta: es un no-op esperado en una instalación que solo usa webhooks.
var mailerMissingOnce sync.Once

// sendAlertMail entrega el aviso por correo.
//
// STARTTLS obligatorio: la alerta lleva el nombre del proyecto y su URL, y aunque
// no sea un secreto, mandar credenciales SMTP en claro por la red no se hace.
// Sin `SMTPHost` configurado no falla: se dice una vez y se sigue, porque una
// instalación sin correo es una opción legítima y no un error que deba reintentar
// el dispatcher para siempre.
func sendAlertMail(ctx context.Context, cfg config.Config, to, subject, body string) error {
	if strings.TrimSpace(cfg.SMTPHost) == "" {
		mailerMissingOnce.Do(func() {
			// El contexto REAL, no `context.TODO()`: ya viaja en la firma, así que
			// el aviso sale con la traza y los campos del dispatcher en vez de con
			// el logger por defecto.
			log.From(ctx).Info("alerta por correo omitida: falta SYTUE_CLOUD_SMTP_HOST " +
				"(las alertas por webhook siguen funcionando)")
		})
		return nil
	}
	from := cfg.SMTPFrom
	if from == "" {
		return fmt.Errorf("service: falta el remitente del correo de alertas")
	}
	message, err := mailMessage(from, to, subject, body)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(cfg.SMTPHost, portOrDefault(cfg.SMTPPort))
	// Plazo corto: esto corre DENTRO de la transacción del dispatcher del outbox,
	// que mantiene el lock de la fila mientras dura. Un servidor de correo lento
	// no puede frenar la entrega de las alertas de los demás.
	// `Dialer.DialContext` y no `net.DialTimeout`: el plazo tiene que poder
	// CANCELARSE con el contexto del dispatcher, no solo vencer solo. Si el
	// proceso se está apagando, una conexión a medias con un servidor lento
	// retrasa el cierre hasta agotar los cinco segundos.
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("service: conectar con el servidor de correo: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))

	c, err := smtp.NewClient(conn, cfg.SMTPHost)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("service: sesión SMTP: %w", err)
	}
	defer func() { _ = c.Close() }()

	// Obligatorio de verdad: antes solo se negociaba si el servidor lo ofrecía, y
	// quien estuviera en medio quitaba la extensión y leía el aviso (CLOUD-19).
	if ok, _ := c.Extension("STARTTLS"); !ok {
		return fmt.Errorf("service: el servidor de correo no ofrece STARTTLS")
	}
	if err := c.StartTLS(&tls.Config{ServerName: cfg.SMTPHost, MinVersion: tls.VersionTLS12}); err != nil {
		return fmt.Errorf("service: STARTTLS: %w", err)
	}
	if cfg.SMTPUser != "" {
		if err := c.Auth(smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPassword, cfg.SMTPHost)); err != nil {
			return fmt.Errorf("service: autenticación SMTP: %w", err)
		}
	}
	if err := c.Mail(message.from); err != nil {
		return fmt.Errorf("service: remitente rechazado: %w", err)
	}
	if err := c.Rcpt(message.to); err != nil {
		return fmt.Errorf("service: destinatario rechazado: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("service: cuerpo del correo: %w", err)
	}
	if _, err := w.Write([]byte(message.content)); err != nil {
		_ = w.Close()
		return fmt.Errorf("service: escribir el correo: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("service: cerrar el correo: %w", err)
	}
	return c.Quit()
}

type alertMailMessage struct {
	from, to, content string
}

// mailMessage valida las cabeceras antes de conectar y codifica el cuerpo como
// MIME text/plain. Los datos de una alerta nunca se interpretan como cabeceras,
// instrucciones SMTP ni marcado HTML, incluso si contienen saltos de línea.
func mailMessage(from, to, subject, body string) (alertMailMessage, error) {
	for _, header := range []string{from, to, subject} {
		for _, r := range header {
			if r < 32 || r == 127 {
				return alertMailMessage{}, fmt.Errorf("service: carácter de control en cabecera de correo")
			}
		}
	}
	sender, err := mail.ParseAddress(from)
	if err != nil {
		return alertMailMessage{}, fmt.Errorf("service: remitente de correo inválido")
	}
	recipient, err := mail.ParseAddress(to)
	if err != nil {
		return alertMailMessage{}, fmt.Errorf("service: destinatario de correo inválido")
	}
	var b strings.Builder
	b.WriteString("From: " + sender.String() + "\r\n")
	b.WriteString("To: " + recipient.String() + "\r\n")
	b.WriteString("Subject: " + mime.BEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n")
	b.WriteString("\r\n")
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	for encoded != "" {
		n := min(76, len(encoded))
		b.WriteString(encoded[:n])
		b.WriteString("\r\n")
		encoded = encoded[n:]
	}
	return alertMailMessage{from: sender.Address, to: recipient.Address, content: b.String()}, nil
}

func portOrDefault(p string) string {
	if strings.TrimSpace(p) == "" {
		return "587" // submission con STARTTLS, que es lo que habla el correo de la casa
	}
	return p
}
