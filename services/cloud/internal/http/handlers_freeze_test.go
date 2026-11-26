// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// La interfaz dice «producción está congelada» por el CÓDIGO, no por el texto: si
// el error cayera en el 409 genérico, el panel enseñaría «estado inválido» a
// quien acaba de pulsar «Desplegar» y no sabría que alguien lo paró a propósito.
func TestDeployCongeladoTieneCodigoPropio(t *testing.T) {
	h := newHandlers(&fakeSvc{deployErr: domain.ErrDeployFrozen})
	rr := httptest.NewRecorder()
	h.CreateDeployment(rr, req(http.MethodPost, "/", validUUID, `{"target":"preview","pr_number":1}`))
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, quiero 409", rr.Code)
	}
	if code := errCode(t, rr); code != "deploy_frozen" {
		t.Fatalf("code = %q, quiero deploy_frozen", code)
	}
}

func TestCongelarLeerYDescongelar(t *testing.T) {
	f := &fakeSvc{}
	h := newHandlers(f)

	rr := httptest.NewRecorder()
	h.GetDeployFreeze(rr, req(http.MethodGet, "/", validUUID, ""))
	if rr.Code != http.StatusOK || decodeEnvelope(t, rr)["frozen"] != false {
		t.Fatalf("sin congelar: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.FreezeDeploys(rr, req(http.MethodPut, "/", validUUID, `{"reason":"cierre contable"}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("congelar: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if m := decodeEnvelope(t, rr); m["frozen"] != true || m["reason"] != "cierre contable" {
		t.Fatalf("congelar devolvió %v", m)
	}

	rr = httptest.NewRecorder()
	h.GetDeployFreeze(rr, req(http.MethodGet, "/", validUUID, ""))
	if m := decodeEnvelope(t, rr); m["frozen"] != true {
		t.Fatalf("tras congelar debe leerse congelado, got %v", m)
	}

	rr = httptest.NewRecorder()
	h.UnfreezeDeploys(rr, req(http.MethodDelete, "/", validUUID, ""))
	if rr.Code != http.StatusNoContent || f.freeze != nil {
		t.Fatalf("descongelar: code=%d freeze=%v", rr.Code, f.freeze)
	}

	// Sin cuerpo también vale: congelar sin explicar el porqué es mejor que no poder.
	rr = httptest.NewRecorder()
	h.FreezeDeploys(rr, req(http.MethodPut, "/", validUUID, ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("congelar sin cuerpo: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.FreezeDeploys(rr, req(http.MethodPut, "/", "no-es-uuid", ""))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("id inválido: code=%d", rr.Code)
	}
}
