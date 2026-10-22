// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// El contrato con Sweego: cabecera Api-Key, SMS transaccional, el número con su
// país y el remitente solo si está configurado. Y un rechazo del proveedor sale
// como error con su motivo, no como un envío que no ocurrió.
func TestSweegoSMS(t *testing.T) {
	var cuerpo map[string]any
	var clave string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clave = r.Header.Get("Api-Key")
		cuerpo = map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&cuerpo)
		if strings.Contains(cuerpo["message-txt"].(string), "sin crédito") {
			w.WriteHeader(http.StatusPaymentRequired)
			_, _ = w.Write([]byte(`{"detail":"Not enough credits"}`))
			return
		}
		_, _ = w.Write([]byte(`{"channel":"sms","transaction_id":"t1"}`))
	}))
	defer srv.Close()
	s := sweegoSMS{url: srv.URL, apiKey: "clave-de-prueba", client: srv.Client()}

	if err := s.Send(context.Background(), "+351912345678", "Tu código de Sytue: 123456"); err != nil {
		t.Fatalf("envío: %v", err)
	}
	destinos, _ := cuerpo["recipients"].([]any)
	destino, _ := destinos[0].(map[string]any)
	if clave != "clave-de-prueba" || cuerpo["channel"] != "sms" || cuerpo["provider"] != "sweego" ||
		cuerpo["campaign-type"] != "transac" || cuerpo["message-txt"] != "Tu código de Sytue: 123456" ||
		destino["num"] != "+351912345678" || destino["region"] != "PT" {
		t.Fatalf("petición inesperada: clave=%q cuerpo=%v", clave, cuerpo)
	}
	if _, conRemitente := cuerpo["sender-id"]; conRemitente {
		t.Fatal("sin remitente configurado no se manda sender-id")
	}

	s.sender = "Sytue"
	if err := s.Send(context.Background(), "+34600123456", "Code: 123456"); err != nil || cuerpo["sender-id"] != "Sytue" {
		t.Fatalf("con remitente: %v %v", err, cuerpo)
	}
	err := s.Send(context.Background(), "+34600123456", "sin crédito")
	if err == nil || !strings.Contains(err.Error(), "Not enough credits") {
		t.Fatalf("el rechazo de Sweego debe llegar con su motivo: %v", err)
	}

	for movil, region := range map[string]string{
		"+34600123456": "ES", "+351912345678": "PT", "+353851234567": "IE", "+4915123456789": "DE",
		"+12025550123": "",
	} {
		if got := regionDelMovil(movil); got != region {
			t.Errorf("regionDelMovil(%q) = %q; quiero %q", movil, got, region)
		}
	}
}
