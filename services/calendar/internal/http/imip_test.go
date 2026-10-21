// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendarhttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/calendar/internal/config"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/service"
)

// El REPLY que entrega Sytue Mail: sin el secreto de la suite no entra; con él, llega
// al servicio con la organización y el remitente de sus cabeceras. Un REPLY de otra
// persona es 403, y uno que ya no tiene nada que contestar, 204 (Mail no reintenta).
func TestLaRespuestaPorCorreoEntraConElSecretoDeLaSuite(t *testing.T) {
	f := &fakeService{}
	h := &Handlers{svc: f, cfg: config.Config{}}
	pedir := func(secreto string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, RutaRespuestaIMIP, strings.NewReader("BEGIN:VCALENDAR"))
		r.Header.Set("Authorization", "Bearer "+secreto)
		r.Header.Set(CabeceraTenantIMIP, "acme")
		r.Header.Set(CabeceraRemitenteIMIP, "luis@gmail.com")
		w := httptest.NewRecorder()
		httpx.SecretoInterno("s3creto")(h.respuestaIMIP()).ServeHTTP(w, r)
		return w
	}
	if w := pedir("otro"); w.Code != http.StatusUnauthorized || f.gotIMIP != "" {
		t.Fatalf("sin el secreto: %d (llegó %q)", w.Code, f.gotIMIP)
	}
	if w := pedir("s3creto"); w.Code != http.StatusNoContent || f.gotIMIP != "acme|luis@gmail.com|BEGIN:VCALENDAR" {
		t.Fatalf("con el secreto: %d, llegó %q", w.Code, f.gotIMIP)
	}
	f.imipErr = service.ErrRespuestaAjena
	if w := pedir("s3creto"); w.Code != http.StatusForbidden {
		t.Fatalf("un REPLY ajeno: %d", w.Code)
	}
	f.imipErr = domain.ErrNotFound
	if w := pedir("s3creto"); w.Code != http.StatusNoContent {
		t.Fatalf("sin invitación pendiente: %d", w.Code)
	}
	// Con el secreto vacío en la configuración, la puerta está cerrada.
	r := httptest.NewRequest(http.MethodPost, RutaRespuestaIMIP, strings.NewReader("x"))
	r.Header.Set("Authorization", "Bearer ")
	w := httptest.NewRecorder()
	httpx.SecretoInterno("")(h.respuestaIMIP()).ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("sin secreto configurado: %d", w.Code)
	}
}
