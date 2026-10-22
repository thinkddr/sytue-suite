// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package delivery

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestTokenSoloDelDominioDeCanales es la verja: si el dominio no se comprobara, CUALQUIER
// correo acabaría publicado en un canal en vez de en su buzón.
func TestTokenSoloDelDominioDeCanales(t *testing.T) {
	b := NewChannelBridge("rooms.sytue.eu", "http://rooms:8085")
	for _, c := range []struct {
		addr  string
		want  string
		canal bool
	}{
		{"abc123@rooms.sytue.eu", "abc123", true},
		{"ABC123@Rooms.Sytue.EU", "ABC123", true}, // dominio insensible; token NO
		// La forma que enseña la interfaz: el nombre del canal delante, para que la
		// dirección se pueda leer, dictar y reconocer. Lo que autoriza va al final.
		{"general+abc123@rooms.sytue.eu", "abc123", true},
		{"avisos-de-produccion+abc123@rooms.sytue.eu", "abc123", true},
		// Un nombre con «+» no despista: cuenta el ÚLTIMO.
		{"a+b+abc123@rooms.sytue.eu", "abc123", true},
		{"+abc123@rooms.sytue.eu", "abc123", true},
		{"general+@rooms.sytue.eu", "", false},
		{"ana@sytue.eu", "", false},                    // buzón normal
		{"abc123@rooms.sytue.eu.malo.test", "", false}, // sufijo: no es el dominio
		{"abc123@evil.test", "", false},
		{"@rooms.sytue.eu", "", false},
		{"sin-arroba", "", false},
	} {
		got, ok := b.Token(c.addr)
		if ok != c.canal || got != c.want {
			t.Fatalf("Token(%q) = (%q,%v), quiero (%q,%v)", c.addr, got, ok, c.want, c.canal)
		}
	}
}

// TestPuenteApagado: sin dominio o sin Rooms no se enciende, y consultarlo no revienta —
// la entrega normal sigue igual.
func TestPuenteApagado(t *testing.T) {
	for _, b := range []*ChannelBridge{
		NewChannelBridge("", "http://rooms:8085"),
		NewChannelBridge("rooms.sytue.eu", ""),
		NewChannelBridge("  ", "  "),
	} {
		if b != nil {
			t.Fatal("el puente debería quedar apagado")
		}
		if _, ok := b.Token("x@rooms.sytue.eu"); ok {
			t.Fatal("apagado pero aceptó una dirección")
		}
	}
}

const correoConAsunto = "From: Ana Ruiz <ana@ejemplo.test>\r\n" +
	"To: tok@rooms.sytue.eu\r\n" +
	"Subject: Fallo en produccion\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"El despliegue de las 14:00 no responde.\r\n"

// TestPublicaAsuntoYCuerpo: el asunto es el título del mensaje y el cuerpo va debajo.
// Si solo se publicara el asunto, media alerta se perdería.
func TestPublicaAsuntoYCuerpo(t *testing.T) {
	var ruta, texto string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ruta = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(raw, &body)
		texto = body.Text
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b := NewChannelBridge("rooms.sytue.eu", srv.URL)
	if err := b.Publish(context.Background(), "tok-123", []byte(correoConAsunto)); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	// El token va en la RUTA: es lo que dice a qué canal se publica.
	if ruta != "/hooks/tok-123" {
		t.Fatalf("ruta = %q", ruta)
	}
	if !strings.Contains(texto, "**Fallo en produccion**") {
		t.Fatalf("sin el asunto como título: %q", texto)
	}
	if !strings.Contains(texto, "El despliegue de las 14:00 no responde.") {
		t.Fatalf("sin el cuerpo: %q", texto)
	}
	if !strings.Contains(texto, "ana@ejemplo.test") {
		t.Fatalf("sin el remitente, en el canal no se sabe quién avisa: %q", texto)
	}
}

// TestPublicaCorreoSoloHTML: las herramientas que avisan por email mandan HTML a secas.
// Sin degradarlo a texto llegaría una tarjeta vacía y parecería que se perdió el aviso.
func TestPublicaCorreoSoloHTML(t *testing.T) {
	const soloHTML = "From: alertas@ejemplo.test\r\n" +
		"Subject: Aviso\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" +
		"<html><head><style>.x{color:red}</style></head>" +
		"<body><p>Disco al <b>93&nbsp;%</b></p><script>alert(1)</script></body></html>\r\n"

	var texto string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(raw, &body)
		texto = body.Text
	}))
	defer srv.Close()

	b := NewChannelBridge("rooms.sytue.eu", srv.URL)
	if err := b.Publish(context.Background(), "tok", []byte(soloHTML)); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if !strings.Contains(texto, "Disco al") || !strings.Contains(texto, "93") {
		t.Fatalf("el HTML no llegó como texto: %q", texto)
	}
	// El contenido de <style> y <script> NO es texto del correo: publicarlo sería
	// volcar código en el canal.
	if strings.Contains(texto, "color:red") || strings.Contains(texto, "alert(1)") {
		t.Fatalf("se publicó el contenido de style/script: %q", texto)
	}
}

// TestRechazoDeRoomsSePropaga: si Rooms dice que no (token revocado, rate-limit), el MX
// tiene que enterarse. Tragárselo dejaría al remitente creyendo que llegó.
func TestRechazoDeRoomsSePropaga(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	b := NewChannelBridge("rooms.sytue.eu", srv.URL)
	err := b.Publish(context.Background(), "tok", []byte(correoConAsunto))
	if err == nil {
		t.Fatal("un 404 de Rooms se dio por entregado")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("el error no dice qué pasó: %v", err)
	}
}

// TestAutomaticosNoSePublican: una autorespuesta en el canal es la mitad de un bucle.
func TestAutomaticosNoSePublican(t *testing.T) {
	for nombre, cab := range map[string]string{
		"vacaciones": "Auto-Submitted: auto-replied\r\n",
		"masivo":     "Precedence: bulk\r\n",
		"lista":      "List-Id: <avisos.ejemplo.test>\r\n",
	} {
		correo := "From: x@ejemplo.test\r\nSubject: s\r\n" + cab + "\r\ncuerpo\r\n"
		if !esAutomatico([]byte(correo)) {
			t.Fatalf("%s: no se detectó como automático", nombre)
		}
	}
	if esAutomatico([]byte(correoConAsunto)) {
		t.Fatal("un correo normal se tomó por automático")
	}
}

// COM-11. La parte local de la dirección va escapada en la ruta: `../x?y#z` no sale
// del /hooks/ ni mete query ni fragmento en la petición a Rooms.
func TestElTokenVaEscapadoEnLaRuta(t *testing.T) {
	var ruta, crudo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ruta, crudo = r.URL.Path, r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b := NewChannelBridge("rooms.sytue.eu", srv.URL)
	if err := b.Publish(context.Background(), "../slack/x?y=1", []byte(correoConAsunto)); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if ruta != "/hooks/../slack/x?y=1" || crudo != "" {
		t.Fatalf("ruta=%q query=%q; el token tenía que llegar entero y escapado", ruta, crudo)
	}
}
