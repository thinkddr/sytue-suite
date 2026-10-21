// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestMensaje(t *testing.T) {
	t.Parallel()
	crudo := string(Mensaje("Sytue ERP <erp@sytue.eu>", "ana@acme.es", "Quedan 10 días: Modelo 303\r\nBcc: todos@x.es",
		"Hola.\nTu fecha es el 8.", time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)))
	msg, err := mail.ReadMessage(strings.NewReader(crudo))
	if err != nil {
		t.Fatal(err)
	}
	asunto, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil || asunto != "Quedan 10 días: Modelo 303  Bcc: todos@x.es" {
		t.Fatalf("asunto %q (%v): el salto de línea no puede abrir otra cabecera", asunto, err)
	}
	if msg.Header.Get("Bcc") != "" || !strings.HasSuffix(msg.Header.Get("Message-ID"), "@sytue.eu>") {
		t.Fatalf("cabeceras: %v", msg.Header)
	}
	if !strings.Contains(crudo, "Hola.\r\nTu fecha es el 8.\r\n") {
		t.Fatalf("el cuerpo va con CRLF: %q", crudo)
	}
	if err := Enviar(context.Background(), Config{}, "ana@acme.es", "x", "y"); !errors.Is(err, ErrSinConfigurar) {
		t.Fatalf("sin servidor no se intenta: %v", err)
	}
}

// El mensaje iMIP se deja leer con la biblioteca estándar: dos partes, la de texto y la
// `text/calendar; method=REQUEST` con el iCalendar ENTERO de vuelta al decodificarla
// (tildes y líneas largas incluidas), y un método con basura se queda en sus letras.
func TestMensajeCalendarioEsIMIP(t *testing.T) {
	ics := []byte("BEGIN:VCALENDAR\r\nMETHOD:REQUEST\r\nBEGIN:VEVENT\r\nSUMMARY:Reunión de áreas " +
		strings.Repeat("x", 120) + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
	crudo := MensajeCalendario("Sytue Calendar <calendar@sytue.eu>", "ana@fuera.example",
		"Invitación: Reunión", "Te han invitado.", ics, "REQUEST;\r\nX: y", time.Now())
	m, err := mail.ReadMessage(bytes.NewReader(crudo))
	if err != nil {
		t.Fatal(err)
	}
	tipo, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil || tipo != "multipart/alternative" {
		t.Fatalf("Content-Type %q: %v", m.Header.Get("Content-Type"), err)
	}
	r := multipart.NewReader(m.Body, params["boundary"])
	texto, err := r.NextPart()
	if err != nil || !strings.HasPrefix(texto.Header.Get("Content-Type"), "text/plain") {
		t.Fatalf("primera parte: %v %v", texto, err)
	}
	cal, err := r.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	ct, cp, _ := mime.ParseMediaType(cal.Header.Get("Content-Type"))
	if ct != "text/calendar" || cp["method"] != "REQUESTXY" {
		t.Fatalf("parte de calendario: %q", cal.Header.Get("Content-Type"))
	}
	datos, _ := io.ReadAll(base64.NewDecoder(base64.StdEncoding, cal))
	if !bytes.Equal(datos, ics) {
		t.Fatalf("el iCalendar no vuelve entero:\n%q", datos)
	}
	if _, err := r.NextPart(); err != io.EOF {
		t.Fatalf("sobra una parte: %v", err)
	}
}

// Con Opciones, el nombre visible es el que se pide pero la dirección sigue siendo la
// del buzón de la suite: un nombre con comillas, «<» o saltos no abre otro remitente
// ni otra cabecera. El Reply-To es UNA dirección, sin nombre, y no es destinatario.
func TestOpcionesNoFabricanOtroRemitente(t *testing.T) {
	t.Parallel()
	const buzon = "Sytue Web <pedidos@sytue.eu>"
	for _, nombre := range []string{
		"Panadería Aurora (vía Sytue Web)",
		`Aurora" <ladron@evil.example>`,
		"Aurora <ladron@evil.example>",
		"Aurora\r\nBcc: todos@evil.example",
		`=?utf-8?q?ladron@evil.example?=`,
	} {
		from, responder, err := presentar(buzon, Opciones{NombreRemitente: nombre, ResponderA: "Tienda <hola@aurora.es>"})
		if err != nil {
			t.Fatalf("%q: %v", nombre, err)
		}
		crudo := string(mensaje(from, "ana@acme.es", responder, "Tu pedido P-000123", "Gracias.", time.Now()))
		msg, err := mail.ReadMessage(strings.NewReader(crudo))
		if err != nil {
			t.Fatalf("%q: %v\n%s", nombre, err, crudo)
		}
		de, err := mail.ParseAddress(msg.Header.Get("From"))
		if err != nil || de.Address != "pedidos@sytue.eu" {
			t.Errorf("%q: From %q (%v): la dirección tiene que ser la del buzón", nombre, msg.Header.Get("From"), err)
			continue
		}
		if de.Name != nombre {
			t.Errorf("%q: el nombre vuelve como %q", nombre, de.Name)
		}
		if msg.Header.Get("Bcc") != "" || msg.Header.Get("Reply-To") != "<hola@aurora.es>" {
			t.Errorf("%q: cabeceras %v", nombre, msg.Header)
		}
	}
	for _, mala := range []string{"hola@aurora.es, otra@aurora.es", "hola@aurora.es\r\nBcc: x@evil.example", "no-es-un-correo"} {
		if _, _, err := presentar(buzon, Opciones{ResponderA: mala}); err == nil {
			t.Errorf("Reply-To %q aceptado", mala)
		}
	}
	// Sin opciones, el From es el del buzón tal cual, como con Enviar.
	if from, responder, err := presentar(buzon, Opciones{}); err != nil || from != buzon || responder != "" {
		t.Errorf("sin opciones: %q %q %v", from, responder, err)
	}
	if err := EnviarCon(context.Background(), Config{}, Opciones{NombreRemitente: "x"}, "ana@acme.es", "x", "y"); !errors.Is(err, ErrSinConfigurar) {
		t.Errorf("sin servidor: %v", err)
	}
}
