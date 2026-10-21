// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// oneClickPOST es la ruta crítica de SSRF: rechaza no-https y cualquier destino no
// enrutable públicamente (privadas/loopback/link-local/metadata) ANTES de dialar.
// Los literales IP se validan en la barrera 1 sin DNS; `localhost` ejerce la
// RESOLUCIÓN (DNS → loopback). La barrera 2 (dial) la aporta ssrfhttp.Transport,
// probada en platform/pkg/ssrfhttp; aquí basta con confirmar el rechazo temprano.
func TestOneClickPOSTRejectsInsecureAndPrivate(t *testing.T) {
	ctx := context.Background()
	for _, u := range []string{
		"http://ex.com/u",                           // no https
		"ftp://ex.com/u",                            // esquema ajeno
		"https://127.0.0.1/u",                       // loopback
		"https://10.0.0.1/u",                        // privada RFC1918
		"https://192.168.1.1/u",                     // privada
		"https://169.254.169.254/latest/meta-data/", // metadata cloud
		"https://[::1]/u",                           // loopback IPv6
		"https://[fd00::1]/u",                       // ULA IPv6
		"https://100.64.0.1/u",                      // CGNAT
		"https://localhost/u",                       // nombre que resuelve a loopback
	} {
		if err := oneClickPOST(ctx, u); err == nil {
			t.Fatalf("%s debería rechazarse por la barrera SSRF/https", u)
		}
	}
}

// --- Handlers getUnsubscribe / postUnsubscribe (fakes, sin red ni PG) ----

// unsubReq construye una petición con cuenta+tenant y el messageID de ruta, y un
// fakeStore/fakeBlobs cuyo mensaje pertenece a la cuenta y trae el .eml `raw`.
func unsubFixture(raw string) (*fakeStore, *fakeBlobs, *fakeOutbound, uuid.UUID) {
	accID := uuid.New()
	msgID := uuid.New()
	st := &fakeStore{
		acc:     domain.Account{ID: accID, Address: "ada@sytue.eu"},
		message: domain.Message{ID: msgID, AccountID: accID, BlobKey: "messages/x/y"},
	}
	return st, &fakeBlobs{getBody: raw}, &fakeOutbound{}, msgID
}

func TestGetUnsubscribeReportsMethods(t *testing.T) {
	raw := "From: n@ex.com\r\n" +
		"List-Unsubscribe: <https://ex.com/u>, <mailto:u@ex.com>\r\n" +
		"List-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n\r\nhola"
	st, blobs, _, msgID := unsubFixture(raw)
	a := newAPI(st, blobs, nil)
	rec := httptest.NewRecorder()
	a.getUnsubscribe(rec, req(http.MethodGet, "/x", "", true, map[string]string{"messageID": msgID.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	m := decodeBody(t, rec)
	if m["one_click"] != true || m["mailto"] != true {
		t.Fatalf("respuesta = %+v, quiero one_click+mailto", m)
	}
}

// Un mensaje de OTRA cuenta (mismo tenant) responde 404: aislamiento por dueño.
func TestGetUnsubscribeOwnerIsolation(t *testing.T) {
	st, blobs, _, msgID := unsubFixture("List-Unsubscribe: <mailto:u@ex.com>\r\n\r\nx")
	st.message.AccountID = uuid.New() // distinto de st.acc.ID
	a := newAPI(st, blobs, nil)
	rec := httptest.NewRecorder()
	a.getUnsubscribe(rec, req(http.MethodGet, "/x", "", true, map[string]string{"messageID": msgID.String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("mensaje ajeno => %d, quiero 404", rec.Code)
	}
}

// Solo mailto: el POST envía el correo de baja DESDE la cuenta, al mailto.
func TestPostUnsubscribeMailtoSends(t *testing.T) {
	st, blobs, out, msgID := unsubFixture("List-Unsubscribe: <mailto:unsub@lists.ex.com?subject=stop>\r\n\r\nx")
	a := newAPI(st, blobs, out)
	rec := httptest.NewRecorder()
	a.postUnsubscribe(rec, req(http.MethodPost, "/x", "{}", true, map[string]string{"messageID": msgID.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if m := decodeBody(t, rec); m["status"] != "sent" {
		t.Fatalf("status = %v, quiero sent", m["status"])
	}
	if out.lastFrom != "ada@sytue.eu" {
		t.Fatalf("from = %q, quiero la dirección de la cuenta", out.lastFrom)
	}
	if len(out.lastRcpts) != 1 || out.lastRcpts[0] != "unsub@lists.ex.com" {
		t.Fatalf("destinatarios = %v, quiero [unsub@lists.ex.com]", out.lastRcpts)
	}
}

// one-click a un destino bloqueado (127.0.0.1) => status failed, sin romper la
// vista (200) y sin salir a la red.
func TestPostUnsubscribeOneClickBlocked(t *testing.T) {
	raw := "List-Unsubscribe: <https://127.0.0.1/u>\r\n" +
		"List-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n\r\nx"
	st, blobs, out, msgID := unsubFixture(raw)
	a := newAPI(st, blobs, out)
	rec := httptest.NewRecorder()
	a.postUnsubscribe(rec, req(http.MethodPost, "/x", "{}", true, map[string]string{"messageID": msgID.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d, quiero 200 con status", rec.Code)
	}
	if m := decodeBody(t, rec); m["status"] != "failed" {
		t.Fatalf("status = %v, quiero failed (SSRF bloqueó)", m["status"])
	}
}

func TestPostUnsubscribeNoMethod(t *testing.T) {
	st, blobs, out, msgID := unsubFixture("Subject: hola\r\n\r\nsin cabecera de baja")
	a := newAPI(st, blobs, out)
	rec := httptest.NewRecorder()
	a.postUnsubscribe(rec, req(http.MethodPost, "/x", "{}", true, map[string]string{"messageID": msgID.String()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sin método de baja => %d, quiero 400", rec.Code)
	}
}
