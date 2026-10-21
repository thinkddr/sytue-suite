// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests unitarios del parseo MIME (sin BD/red): extracción del sobre indexable.
package mime

import (
	"strings"
	"testing"
)

const plainMsg = "From: Ada Lovelace <ada@sytue.eu>\r\n" +
	"To: Bob <bob@example.com>, carol@example.com\r\n" +
	"Cc: dave@example.com\r\n" +
	"Subject: Hola mundo\r\n" +
	"Message-Id: <abc123@sytue.eu>\r\n" +
	"In-Reply-To: <parent@sytue.eu>\r\n" +
	"Date: Mon, 30 Jun 2026 10:00:00 +0000\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"Este es el cuerpo del mensaje de prueba.\r\n"

func TestParsePlain(t *testing.T) {
	p, err := Parse(strings.NewReader(plainMsg))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	e := p.Envelope
	if e.Subject != "Hola mundo" {
		t.Errorf("Subject = %q", e.Subject)
	}
	if e.FromAddr != "ada@sytue.eu" {
		t.Errorf("FromAddr = %q", e.FromAddr)
	}
	if e.FromName != "Ada Lovelace" {
		t.Errorf("FromName = %q", e.FromName)
	}
	if e.MessageID != "abc123@sytue.eu" {
		t.Errorf("MessageID = %q", e.MessageID)
	}
	if e.InReplyTo != "parent@sytue.eu" {
		t.Errorf("InReplyTo = %q", e.InReplyTo)
	}
	if len(e.To) != 2 || e.To[0] != "bob@example.com" || e.To[1] != "carol@example.com" {
		t.Errorf("To = %v", e.To)
	}
	if len(e.Cc) != 1 || e.Cc[0] != "dave@example.com" {
		t.Errorf("Cc = %v", e.Cc)
	}
	if !strings.Contains(e.Snippet, "cuerpo del mensaje") {
		t.Errorf("Snippet = %q", e.Snippet)
	}
	if e.SizeBytes != int64(len(plainMsg)) {
		t.Errorf("SizeBytes = %d, quiero %d", e.SizeBytes, len(plainMsg))
	}
	if e.SentAt.Year() != 2026 || e.SentAt.Month() != 6 {
		t.Errorf("SentAt = %v", e.SentAt)
	}
}

func TestParseThreadingRefsFallback(t *testing.T) {
	// Sin In-Reply-To: threading toma el último id de References.
	msg := "From: a@sytue.eu\r\nSubject: R\r\n" +
		"References: <r1@x> <r2@x> <r3@x>\r\n\r\nhola\r\n"
	p, err := Parse(strings.NewReader(msg))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Envelope.InReplyTo != "r3@x" {
		t.Errorf("InReplyTo (References fallback) = %q, quiero r3@x", p.Envelope.InReplyTo)
	}
}

func TestParseMultipartAttachment(t *testing.T) {
	msg := "From: a@sytue.eu\r\nTo: b@sytue.eu\r\nSubject: con adjunto\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=BOUND\r\n\r\n" +
		"--BOUND\r\nContent-Type: text/plain\r\n\r\ncuerpo\r\n" +
		"--BOUND\r\nContent-Type: application/pdf\r\n" +
		"Content-Disposition: attachment; filename=doc.pdf\r\n\r\nPDFDATA\r\n" +
		"--BOUND--\r\n"
	p, err := Parse(strings.NewReader(msg))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !p.Envelope.HasAttach {
		t.Error("HasAttach = false, quiero true")
	}
	if !strings.Contains(p.Envelope.Snippet, "cuerpo") {
		t.Errorf("Snippet = %q", p.Envelope.Snippet)
	}
}

func TestParseRawPreserved(t *testing.T) {
	p, err := Parse(strings.NewReader(plainMsg))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if string(p.Raw) != plainMsg {
		t.Error("Raw debe conservar los bytes originales intactos")
	}
}

// LAS FOTOS DEL IPHONE. `multipart/mixed` con el texto y las imágenes sueltas,
// marcadas `Content-Disposition: inline`, sin parte HTML y sin `Content-ID`.
//
// Es la forma en la que iPhone Mail manda una foto, medida sobre un mensaje real
// (X-Mailer: iPhone Mail 23G83). Aquí solo se miraba `AttachmentHeader`, así que
// se guardaban con `has_attachment` en falso: sin clip, y —lo que de verdad
// importa— el lector ni pedía las partes, porque esa petición va condicionada a
// la marca. Las fotos llegaban y no las veía nadie.
func TestParseImagenEnLineaCuentaComoAdjunto(t *testing.T) {
	msg := "From: a@sytue.eu\r\nTo: b@sytue.eu\r\nSubject: con foto\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=BOUND\r\n\r\n" +
		"--BOUND\r\nContent-Type: text/plain\r\n\r\nHolaaa\r\n" +
		"--BOUND\r\nContent-Type: image/png; name=image0.png\r\n" +
		"Content-Disposition: inline; filename=image0.png\r\n\r\nPNGDATA\r\n" +
		"--BOUND--\r\n"
	p, err := Parse(strings.NewReader(msg))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !p.Envelope.HasAttach {
		t.Error("HasAttach = false: el lector no llegaría a pedir la imagen")
	}
	if !strings.Contains(p.Envelope.Snippet, "Holaaa") {
		t.Errorf("Snippet = %q", p.Envelope.Snippet)
	}
}

// Y un mensaje de SOLO TEXTO no gana un clip por tener dos cuerpos.
// `multipart/alternative` trae texto y HTML, y ninguno es un fichero.
func TestParseAlternativeNoEsAdjunto(t *testing.T) {
	msg := "From: a@sytue.eu\r\nTo: b@sytue.eu\r\nSubject: sin nada\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/alternative; boundary=BOUND\r\n\r\n" +
		"--BOUND\r\nContent-Type: text/plain\r\n\r\ncuerpo\r\n" +
		"--BOUND\r\nContent-Type: text/html\r\n\r\n<p>cuerpo</p>\r\n" +
		"--BOUND--\r\n"
	p, err := Parse(strings.NewReader(msg))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Envelope.HasAttach {
		t.Error("HasAttach = true: un cuerpo alternativo no es un fichero adjunto")
	}
}

// El caso que dejó seis correos «Desconocido» en una bandeja real: el propio
// webmail componía el From con un display-name que era otra dirección, sin
// comillas. Eso no es RFC 5322 válido, `AddressList` lo rechaza entero y antes el
// mensaje se guardaba SIN remitente pese a tenerlo delante.
const fromTorcido = "From: ops@sytue.com <ops@corporate.sytue.eu>\r\n" +
	"To: alguien@uno.example <alguien@dos.example>\r\n" +
	"Subject: Plan de guardias\r\n" +
	"Date: Fri, 11 Sep 2026 18:17:00 +0000\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"Os paso el reparto.\r\n"

func TestParseRescataElRemitenteDeUnaCabeceraTorcida(t *testing.T) {
	p, err := Parse(strings.NewReader(fromTorcido))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	e := p.Envelope
	// La dirección buena es la de los ÁNGULOS; lo de delante es el nombre.
	if e.FromAddr != "ops@corporate.sytue.eu" {
		t.Errorf("FromAddr = %q, quiero la de los ángulos", e.FromAddr)
	}
	if e.FromName != "ops@sytue.com" {
		t.Errorf("FromName = %q", e.FromName)
	}
	if len(e.To) != 1 || e.To[0] != "alguien@dos.example" {
		t.Errorf("To = %v, quiero rescatado igual que el From", e.To)
	}
	if e.Subject != "Plan de guardias" {
		t.Errorf("Subject = %q", e.Subject)
	}
}

func TestRescatarDireccionNoSeInventaNada(t *testing.T) {
	casos := map[string]struct{ addr, nombre string }{
		"":                             {"", ""},
		"Ana Ruiz":                     {"", ""},
		"<>":                           {"", ""},
		"ana@x.example":                {"ana@x.example", ""},
		"ANA@X.example":                {"ana@x.example", ""},
		`"Ana" <ana@x.example>`:        {"ana@x.example", "Ana"},
		"ana@uno.example <a@dos.test>": {"a@dos.test", "ana@uno.example"},
		"Ana <no-es-una-direccion>":    {"", ""},
	}
	for valor, quiero := range casos {
		addr, nombre := rescatarDireccion(valor)
		if addr != quiero.addr || nombre != quiero.nombre {
			t.Errorf("rescatarDireccion(%q) = (%q, %q), quiero (%q, %q)",
				valor, addr, nombre, quiero.addr, quiero.nombre)
		}
	}
}

// Todo el correo que sale de Sytue va firmado con S/MIME, así que la firma
// marcaba el clip de adjunto en TODOS los mensajes: la lista prometía un fichero
// que no existe.
func TestLaFirmaSMIMENoEsUnAdjunto(t *testing.T) {
	firmado := "From: Ada <ada@sytue.eu>\r\n" +
		"To: bob@example.com\r\n" +
		"Subject: Firmado\r\n" +
		"MIME-Version: 1.0\r\n" +
		`Content-Type: multipart/signed; protocol="application/pkcs7-signature"; ` +
		"micalg=sha-256; boundary=\"sytue-smime-abc\"\r\n" +
		"\r\n" +
		"--sytue-smime-abc\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"El cuerpo.\r\n" +
		"--sytue-smime-abc\r\n" +
		"Content-Type: application/pkcs7-signature; name=\"smime.p7s\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"Content-Disposition: attachment; filename=\"smime.p7s\"\r\n" +
		"\r\n" +
		"MIIBogYJKoZIhvcNAQcCoIIBkzCCAY8CAQE=\r\n" +
		"--sytue-smime-abc--\r\n"
	p, err := Parse(strings.NewReader(firmado))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Envelope.HasAttach {
		t.Error("un mensaje solo firmado se marcó como con adjunto")
	}
	if !strings.Contains(p.Envelope.Snippet, "El cuerpo") {
		t.Errorf("Snippet = %q, se perdió el texto de la parte firmada", p.Envelope.Snippet)
	}
}
