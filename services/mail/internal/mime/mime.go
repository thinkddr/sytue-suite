// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package mime parsea y construye mensajes de correo RFC 5322 / MIME usando la
// librería permisiva emersion/go-message (MIT). Extrae el sobre indexable
// (asunto, remitente, destinatarios, Message-ID, threading, snippet, adjuntos)
// que Sytue Mail guarda en PostgreSQL; el .eml crudo se conserva íntegro en S3.
//
// Clean-room: implementado sobre los estándares (RFC 5322 IMF, RFC 2045-2047
// MIME, RFC 2822 references) y go-message; sin código de terceros AGPL.
package mime

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/mail"
	"strings"
	"time"

	gomessage "github.com/emersion/go-message"
	gomail "github.com/emersion/go-message/mail"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// maxSnippet es la longitud máxima del extracto de texto plano para vistas lista.
const maxSnippet = 280

// Parsed es el resultado de analizar un mensaje crudo: el sobre indexable más
// los bytes originales (que se suben tal cual a S3, sin reescribir).
type Parsed struct {
	Envelope domain.Message
	Raw      []byte
}

// Parse analiza un mensaje RFC 5322 desde r, extrayendo el sobre indexable.
// No falla por cabeceras malformadas no esenciales: degrada con valores vacíos.
func Parse(r io.Reader) (Parsed, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return Parsed{}, fmt.Errorf("mime: leer mensaje: %w", err)
	}

	mr, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		// Reintento como mensaje genérico (algunos remitentes envían cuerpos no
		// estrictamente conformes); si tampoco parsea, devolvemos el error.
		if _, gerr := gomessage.Read(bytes.NewReader(raw)); gerr != nil {
			return Parsed{}, fmt.Errorf("mime: parsear mensaje: %w", err)
		}
		return Parsed{Envelope: envelopeFromHeaders(raw), Raw: raw}, nil
	}
	defer mr.Close()

	env := domain.Message{Flags: []string{}, SizeBytes: int64(len(raw))}
	h := mr.Header

	env.Subject, _ = h.Subject()
	env.MessageID = trimAngle(h.Get("Message-Id"))
	env.InReplyTo = firstRef(h.Get("In-Reply-To"), h.Get("References"))
	if d, err := h.Date(); err == nil {
		env.SentAt = d.UTC()
	} else {
		env.SentAt = time.Now().UTC()
	}

	if from, err := h.AddressList("From"); err == nil && len(from) > 0 {
		env.FromAddr = strings.ToLower(from[0].Address)
		env.FromName = from[0].Name
	} else {
		env.FromAddr, env.FromName = rescatarDireccion(h.Get("From"))
	}
	env.To = addrList(h, "To")
	env.Cc = addrList(h, "Cc")
	env.Bcc = addrList(h, "Bcc")
	env.ReplyTo = addrList(h, "Reply-To")

	// Recorrer las partes: snippet del primer text/plain, detectar adjuntos.
	var snippet strings.Builder
	for {
		p, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			break // degradar: lo esencial (cabeceras) ya está extraído
		}
		switch ph := p.Header.(type) {
		case *gomail.InlineHeader:
			ct, _, _ := ph.ContentType()
			if strings.HasPrefix(ct, "text/plain") && snippet.Len() < maxSnippet {
				b, _ := io.ReadAll(io.LimitReader(p.Body, maxSnippet*4))
				snippet.WriteString(collapseSpace(string(b)))
			}
			// UNA PARTE EN LÍNEA QUE NO ES TEXTO ES UN FICHERO DENTRO DEL MENSAJE,
			// y cuenta como adjunto aunque venga marcada `inline`.
			//
			// El iPhone manda las fotos así: `multipart/mixed` con el texto y las
			// imágenes sueltas, `Content-Disposition: inline`. Aquí solo se miraba
			// `AttachmentHeader`, así que esos mensajes se guardaban con
			// `has_attachment` en falso: sin clip en la lista y —lo que de verdad
			// importa— el lector ni llegaba a pedir las partes, porque la petición
			// va condicionada a esa marca. Las fotos existían y no las veía nadie.
			//
			// Lo mismo valía para las imágenes `cid:` de un `multipart/related`,
			// que también viajan `inline`: la maqueta las citaba y el lector no
			// tenía con qué resolverlas.
			//
			// `multipart/` fuera: es el envoltorio, no un fichero.
			if !strings.HasPrefix(ct, "text/") && !strings.HasPrefix(ct, "multipart/") && !esFirma(ct) {
				env.HasAttach = true
			}
		case *gomail.AttachmentHeader:
			ct, _, _ := ph.ContentType()
			if !esFirma(ct) {
				env.HasAttach = true
			}
		}
	}
	env.Snippet = truncate(snippet.String(), maxSnippet)

	return Parsed{Envelope: env, Raw: raw}, nil
}

// envelopeFromHeaders es el camino de degradación: extrae lo esencial con el
// parser de cabeceras estándar cuando go-message/mail no puede leer el cuerpo.
func envelopeFromHeaders(raw []byte) domain.Message {
	env := domain.Message{Flags: []string{}, SizeBytes: int64(len(raw)), SentAt: time.Now().UTC()}
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return env
	}
	dec := new(mime.WordDecoder)
	if s, err := dec.DecodeHeader(m.Header.Get("Subject")); err == nil {
		env.Subject = s
	}
	env.MessageID = trimAngle(m.Header.Get("Message-Id"))
	env.InReplyTo = firstRef(m.Header.Get("In-Reply-To"), m.Header.Get("References"))
	if addrs, err := m.Header.AddressList("From"); err == nil && len(addrs) > 0 {
		env.FromAddr = strings.ToLower(addrs[0].Address)
		env.FromName = addrs[0].Name
	} else {
		env.FromAddr, env.FromName = rescatarDireccion(m.Header.Get("From"))
	}
	env.To = headerAddrList(m.Header, "To")
	env.Cc = headerAddrList(m.Header, "Cc")
	if d, err := m.Header.Date(); err == nil {
		env.SentAt = d.UTC()
	}
	return env
}

func addrList(h gomail.Header, key string) []string {
	addrs, err := h.AddressList(key)
	if err != nil || len(addrs) == 0 {
		return rescatarDirecciones(h.Get(key))
	}
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, strings.ToLower(a.Address))
	}
	return out
}

func headerAddrList(h mail.Header, key string) []string {
	addrs, err := h.AddressList(key)
	if err != nil || len(addrs) == 0 {
		return rescatarDirecciones(h.Get(key))
	}
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, strings.ToLower(a.Address))
	}
	return out
}

// esFirma reconoce la parte de firma S/MIME, que NO es un adjunto.
//
// Todo correo que sale de Sytue va firmado, así que el clip aparecía en todos:
// la lista prometía un fichero que no existe y el lector abría una sección de
// adjuntos con un `smime.p7s` dentro. Lo que hay que enseñar de una firma es que
// el mensaje está firmado, no un fichero que nadie va a abrir.
func esFirma(contentType string) bool {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "application/pkcs7-signature", "application/x-pkcs7-signature":
		return true
	}
	return false
}

// rescatarDireccion saca la dirección de una cabecera que el parser estándar ha
// rechazado ENTERA.
//
// El caso que lo motiva salió de nuestro propio webmail: una cuenta sin nombre
// cae a su correo como nombre visible, y la cabecera acababa siendo
// «From: ana@uno.example <ana@dos.example>». Un display-name con arroba y sin
// comillas no es RFC 5322 válido, `AddressList` devuelve error y el mensaje se
// guardaba SIN remitente: la bandeja lo enseña como «Desconocido», la búsqueda
// por remitente no lo encuentra y responder no rellena el Para.
//
// La dirección está a la vista entre los ángulos, así que se rescata en vez de
// tirarla. Vale para cualquier remitente de fuera con la cabecera torcida, que
// es algo que pasa todos los días y que no se puede arreglar en su servidor.
func rescatarDireccion(valor string) (addr, nombre string) {
	valor = strings.TrimSpace(valor)
	if valor == "" {
		return "", ""
	}
	i := strings.IndexByte(valor, '<')
	j := strings.IndexByte(valor, '>')
	if i < 0 || j < i {
		// Sin ángulos: o es una dirección suelta o no hay nada que rescatar.
		if pareceDireccion(valor) {
			return strings.ToLower(valor), ""
		}
		return "", ""
	}
	dir := strings.TrimSpace(valor[i+1 : j])
	if !pareceDireccion(dir) {
		return "", ""
	}
	return strings.ToLower(dir), strings.Trim(strings.TrimSpace(valor[:i]), `"`)
}

// rescatarDirecciones hace lo mismo con una cabecera de VARIAS direcciones (To,
// Cc). Parte por comas, que es suficiente aquí: lo que se rescata es una cabecera
// ya rota, y una coma dentro de un nombre entrecomillado como mucho deja un
// trozo fuera en vez de perder la lista entera.
func rescatarDirecciones(valor string) []string {
	var out []string
	for _, trozo := range strings.Split(valor, ",") {
		if dir, _ := rescatarDireccion(trozo); dir != "" {
			out = append(out, dir)
		}
	}
	return out
}

// pareceDireccion filtra lo que no puede ser un buzón antes de guardarlo.
func pareceDireccion(s string) bool {
	i := strings.IndexByte(s, '@')
	return i > 0 && i < len(s)-1 && !strings.ContainsAny(s, " \t\r\n\"")
}

// trimAngle quita los <> de un Message-ID.
func trimAngle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimSuffix(s, ">")
	return s
}

// firstRef devuelve el Message-ID padre para threading: prioriza In-Reply-To,
// si no, el último id de References (RFC 5322 §3.6.4).
func firstRef(inReplyTo, references string) string {
	if id := trimAngle(inReplyTo); id != "" {
		return id
	}
	refs := strings.Fields(references)
	if len(refs) > 0 {
		return trimAngle(refs[len(refs)-1])
	}
	return ""
}

func collapseSpace(s string) string { return strings.Join(strings.Fields(s), " ") + " " }

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n]
}
