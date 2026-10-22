// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package delivery

import (
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

const replyICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nMETHOD:REPLY\r\nBEGIN:VEVENT\r\nUID:u1\r\n" +
	"ATTENDEE;PARTSTAT=ACCEPTED:mailto:luis@gmail.com\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

// Un REPLY como lo manda un cliente de fuera: `multipart/mixed` con una alternativa y,
// dentro, la parte `text/calendar; method=REPLY` en base64 partido en líneas.
func replyDeFuera() []byte {
	b64 := base64.StdEncoding.EncodeToString([]byte(replyICS))
	return []byte("From: Luis <luis@gmail.com>\r\nTo: owner@sytue.eu\r\nSubject: Aceptado: Comité\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=\"A\"\r\n\r\n" +
		"--A\r\nContent-Type: multipart/alternative; boundary=\"B\"\r\n\r\n" +
		"--B\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nLuis ha aceptado.\r\n" +
		"--B\r\nContent-Type: text/calendar; charset=\"UTF-8\"; method=REPLY\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
		b64[:40] + "\r\n" + b64[40:] + "\r\n--B--\r\n--A--\r\n")
}

func TestElReplyDeUnaInvitacionSeEncuentraEnElCorreo(t *testing.T) {
	ics, ok := ReplyDeCalendario(replyDeFuera())
	if !ok || string(ics) != replyICS {
		t.Fatalf("ReplyDeCalendario = %v %q", ok, ics)
	}
	peticion := "From: a@b.c\r\nContent-Type: text/calendar; method=REQUEST\r\n\r\n" + strings.Replace(replyICS, "METHOD:REPLY", "METHOD:REQUEST", 1)
	if _, ok := ReplyDeCalendario([]byte(peticion)); ok {
		t.Fatal("una invitación (REQUEST) no es una respuesta")
	}
	if _, ok := ReplyDeCalendario([]byte("From: a@b.c\r\nContent-Type: text/plain\r\n\r\nhola\r\n")); ok {
		t.Fatal("un correo sin calendario no trae REPLY")
	}
}

// El puente pasa el REPLY a Calendar con la organización y el remitente, SOLO si el
// remitente está autenticado: un From sin autenticar lo puede poner cualquiera.
func TestElPuentePasaElReplySoloSiElRemitenteEstaAutenticado(t *testing.T) {
	llegan, cuerpos, p := calendarioDePrueba(t)
	env := domain.Message{FromAddr: "luis@gmail.com"}
	p.Pasar(t.Context(), "acme", env, replyDeFuera(), false)
	noLlega(t, llegan, "un REPLY sin autenticar ha llegado a Calendar")
	// «DKIM pass» a secas ya no abre el puente: lo consigue quien firme con su dominio.
	env.DKIMResult = domain.AuthPass
	p.Pasar(t.Context(), "acme", env, replyDeFuera(), false)
	noLlega(t, llegan, "un REPLY con DKIM de cualquiera ha llegado a Calendar")

	p.Pasar(t.Context(), "acme", env, replyDeFuera(), true)
	select {
	case r := <-llegan:
		if r.URL.Path != rutaRespuestaIMIP || r.Header.Get("Authorization") != "Bearer s3creto" ||
			r.Header.Get("X-Sytue-Tenant") != "acme" || r.Header.Get("X-Sytue-Remitente") != "luis@gmail.com" {
			t.Fatalf("petición a Calendar: %s %v", r.URL.Path, r.Header)
		}
		if c := <-cuerpos; c != replyICS {
			t.Fatalf("cuerpo: %q", c)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("el REPLY autenticado no ha llegado a Calendar")
	}
	if NewPuenteCalendario("", "s", nil) != nil || NewPuenteCalendario("http://x", "", nil) != nil {
		t.Fatal("sin URL o sin secreto no hay puente")
	}
	var nulo *PuenteCalendario
	nulo.Pasar(t.Context(), "acme", env, replyDeFuera(), true) // sin puente, no hace nada
}

func calendarioDePrueba(t *testing.T) (llegan chan *http.Request, cuerpos chan string, p *PuenteCalendario) {
	t.Helper()
	llegan, cuerpos = make(chan *http.Request, 2), make(chan string, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		llegan <- r
		cuerpos <- string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return llegan, cuerpos, NewPuenteCalendario(srv.URL, "s3creto", slog.New(slog.DiscardHandler))
}

func noLlega(t *testing.T, llegan chan *http.Request, mensaje string) {
	t.Helper()
	select {
	case <-llegan:
		t.Fatal(mensaje)
	case <-time.After(200 * time.Millisecond):
	}
}
