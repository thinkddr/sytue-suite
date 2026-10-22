// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package inboundhook

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/services/mail/internal/delivery"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// doble registra lo que le llega y devuelve lo que se le pida.
type doble struct {
	errResolve   map[string]error
	errDeliver   error
	entregado    *delivery.InboundMessage
	cuerpo       string
	resoluciones []string
}

func (d *doble) Resolve(_ context.Context, address string) (store.ResolvedAddress, error) {
	d.resoluciones = append(d.resoluciones, address)
	if err, ok := d.errResolve[address]; ok {
		return store.ResolvedAddress{}, err
	}
	return store.ResolvedAddress{}, nil
}

func (d *doble) Deliver(_ context.Context, msg delivery.InboundMessage) error {
	copia := msg
	if msg.Body != nil {
		b, _ := io.ReadAll(msg.Body)
		d.cuerpo = string(b)
	}
	copia.Body = nil
	d.entregado = &copia
	return d.errDeliver
}

func montar(t *testing.T, d *doble, token string) http.Handler {
	t.Helper()
	r := chi.NewRouter()
	Mount(r, Deps{Inbound: d, Token: token})
	return r
}

func peticion(token, de, para, cuerpo string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, Ruta, strings.NewReader(cuerpo))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if de != "" {
		req.Header.Set(CabeceraDe, de)
	}
	if para != "" {
		req.Header.Set(CabeceraPara, para)
	}
	return req
}

const secreto = "un-token-largo-y-secreto"

// SIN TOKEN NO HAY WEBHOOK. Es la comprobación que impide que esto sea un relé
// abierto, así que va la primera.
func TestSinTokenNoSeMonta(t *testing.T) {
	d := &doble{}
	r := chi.NewRouter()
	Mount(r, Deps{Inbound: d, Token: ""})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, peticion("", "a@b.c", "x@sytue.eu", "hola"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("sin token la ruta no debe existir; salió %d", w.Code)
	}
}

func TestTokenIncorrectoRechaza(t *testing.T) {
	d := &doble{}
	w := httptest.NewRecorder()
	montar(t, d, secreto).ServeHTTP(w, peticion("otro-token", "a@b.c", "x@sytue.eu", "hola"))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("esperaba 401, salió %d", w.Code)
	}
	if d.entregado != nil {
		t.Fatal("entregó un mensaje sin token válido")
	}
}

// EL SOBRE NO SE INVENTA. Sin MAIL FROM / RCPT TO no se puede entregar, y el
// error tiene que ser 4xx para que el reenviador NO reintente en bucle.
func TestSinSobreEsErrorDefinitivo(t *testing.T) {
	for _, c := range []struct{ nombre, de, para string }{
		{"sin remitente", "", "x@sytue.eu"},
		{"sin destinatario", "a@b.c", ""},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			d := &doble{}
			w := httptest.NewRecorder()
			montar(t, d, secreto).ServeHTTP(w, peticion(secreto, c.de, c.para, "hola"))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("esperaba 400 (definitivo), salió %d", w.Code)
			}
		})
	}
}

// UN DESTINATARIO QUE NO EXISTE NO ES UN ERROR DEL REENVIADOR. Devolver 5xx
// haría que lo reintentara durante días contra una dirección que no va a existir.
func TestDestinatarioDesconocidoNoSeReintenta(t *testing.T) {
	d := &doble{errResolve: map[string]error{"nadie@sytue.eu": domain.ErrNoSuchUser}}
	w := httptest.NewRecorder()
	montar(t, d, secreto).ServeHTTP(w, peticion(secreto, "a@b.c", "nadie@sytue.eu", "hola"))
	if w.Code != http.StatusOK {
		t.Fatalf("esperaba 200 (definitivo, sin reintento), salió %d", w.Code)
	}
	if d.entregado != nil {
		t.Fatal("entregó a un destinatario que no existe")
	}
}

// De varios destinatarios, solo se entrega a los locales y los demás se ignoran
// en silencio: es lo mismo que hace el MX en su fase RCPT.
func TestSoloLosLocalesLleganAEntrega(t *testing.T) {
	d := &doble{errResolve: map[string]error{"fuera@otro.eu": domain.ErrNoSuchUser}}
	w := httptest.NewRecorder()
	montar(t, d, secreto).ServeHTTP(w,
		peticion(secreto, "a@b.c", "fuera@otro.eu, dentro@sytue.eu", "hola"))
	if w.Code != http.StatusNoContent {
		t.Fatalf("esperaba 204, salió %d", w.Code)
	}
	if d.entregado == nil || len(d.entregado.Recipients) != 1 || d.entregado.Recipients[0] != "dentro@sytue.eu" {
		t.Fatalf("destinatarios entregados incorrectos: %+v", d.entregado)
	}
}

// EL CAMINO FELIZ, comprobando que el sobre y el cuerpo llegan intactos.
func TestEntregaElSobreYElCuerpo(t *testing.T) {
	d := &doble{}
	cuerpo := "From: a@b.c\r\nSubject: hola\r\n\r\ncuerpo\r\n"
	w := httptest.NewRecorder()
	montar(t, d, secreto).ServeHTTP(w, peticion(secreto, "a@b.c", "x@sytue.eu", cuerpo))
	if w.Code != http.StatusNoContent {
		t.Fatalf("esperaba 204, salió %d", w.Code)
	}
	if d.entregado.MailFrom != "a@b.c" {
		t.Fatalf("MailFrom llegó como %q", d.entregado.MailFrom)
	}
	if d.cuerpo != cuerpo {
		t.Fatalf("el cuerpo no llegó intacto:\n%q", d.cuerpo)
	}
}

// La cabecera repetida y la lista con comas tienen que dar lo mismo: no todos
// los reenviadores dejan repetir una cabecera.
func TestCabeceraRepetidaYListaConComasSonEquivalentes(t *testing.T) {
	d := &doble{}
	req := httptest.NewRequest(http.MethodPost, Ruta, strings.NewReader("x"))
	req.Header.Set("Authorization", "Bearer "+secreto)
	req.Header.Set(CabeceraDe, "a@b.c")
	req.Header.Add(CabeceraPara, "uno@sytue.eu")
	req.Header.Add(CabeceraPara, "dos@sytue.eu")
	w := httptest.NewRecorder()
	montar(t, d, secreto).ServeHTTP(w, req)
	if d.entregado == nil || len(d.entregado.Recipients) != 2 {
		t.Fatalf("con cabecera repetida esperaba 2 destinatarios: %+v", d.entregado)
	}
}

// LOS CÓDIGOS DE ERROR DICEN SI REINTENTAR. Es la misma decisión que toman los
// códigos SMTP del MX, y equivocarse cuesta lo mismo.
func TestLosErroresSeTraducenAReintentarONo(t *testing.T) {
	for _, c := range []struct {
		nombre   string
		err      error
		esperado int
	}{
		{"demasiado grande: definitivo", domain.ErrMessageTooLarge, http.StatusRequestEntityTooLarge},
		{"cuota: temporal, se libera sola", domain.ErrQuotaExceeded, http.StatusInsufficientStorage},
		{"DMARC: definitivo", delivery.ErrRejectedPolicy, http.StatusUnprocessableEntity},
		{"desconocido: temporal", errors.New("vaya"), http.StatusInternalServerError},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			d := &doble{errDeliver: c.err}
			w := httptest.NewRecorder()
			montar(t, d, secreto).ServeHTTP(w, peticion(secreto, "a@b.c", "x@sytue.eu", "hola"))
			if w.Code != c.esperado {
				t.Fatalf("esperaba %d, salió %d", c.esperado, w.Code)
			}
		})
	}
}
