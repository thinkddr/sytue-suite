// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package adyen

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pagos"
)

func TestCrearPagoYReembolsar(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if got := r.Header.Get("X-API-Key"); got != "api-key" {
			t.Errorf("X-API-Key = %q", got)
		}
		if got := r.Header.Get("Idempotency-Key"); got == "" {
			t.Error("falta Idempotency-Key")
		}
		switch r.URL.Path {
		case "/sessions":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			// La referencia es el id del pago: es lo único que la notificación devuelve.
			if body["merchantAccount"] != "Cuenta_123" || body["mode"] != "hosted" || body["reference"] != "pay-123" {
				t.Fatalf("cuerpo de sesión = %#v", body)
			}
			_, _ = w.Write([]byte(`{"id":"CS1234567890","url":"https://checkoutshopper-test.adyen.com/checkout?sessionId=x"}`))
		case "/payments/9912345678901234/refunds":
			if r.Method != http.MethodPost {
				t.Fatalf("método = %s", r.Method)
			}
			_, _ = w.Write([]byte(`{"pspReference":"9912345678901235"}`))
		default:
			t.Fatalf("ruta inesperada: %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	a, err := New("api-key", "Cuenta_123", pagos.ModoPrueba, "")
	if err != nil {
		t.Fatal(err)
	}
	a.baseURL = srv.URL
	creado, err := a.CrearPago(context.Background(), pagos.Solicitud{
		Centimos: 1234, Divisa: pagos.DivisaEUR, Descripcion: "Pedido", Referencia: "order-123",
		ClaveIdempotencia: "create-123", URLRetorno: "https://pages.example/return", URLWebhook: "https://pages.example/webhook",
		Caduca: time.Now().Add(15 * time.Minute), Metadatos: map[string]string{pagos.MetadatoPago: "pay-123"},
	})
	if err != nil || creado.Ref != "CS1234567890" {
		t.Fatalf("CrearPago() = %#v, %v", creado, err)
	}
	if err := a.Reembolsar(context.Background(), "9912345678901234", 1234, "refund-123"); err != nil {
		t.Fatalf("Reembolsar() = %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestNotificacionVerificarFirmaEjemploOficial(t *testing.T) {
	n := Notificacion{
		PSPReference:        "7914073381342284",
		MerchantAccountCode: "TestMerchant",
		MerchantReference:   "TestPayment-1407325143704",
		EventCode:           "AUTHORISATION",
		Success:             "true",
		AdditionalData:      map[string]string{"hmacSignature": "coqCmt/IZ4E3CzPvMY8zTjQVL5hYJUiBRg8UU+iCWo0="},
	}
	n.Amount.Value, n.Amount.Currency = 1130, "EUR"
	if err := n.VerificarFirma("44782DEF547AAA06C910C43932B1EB0C71FC68D9D0C057550C48EC2ACF6BA056"); err != nil {
		t.Fatalf("VerificarFirma() = %v", err)
	}
	n.Success = "false"
	if err := n.VerificarFirma("44782DEF547AAA06C910C43932B1EB0C71FC68D9D0C057550C48EC2ACF6BA056"); err == nil {
		t.Fatal("VerificarFirma() aceptó contenido modificado")
	}
}

func TestNotificacionEstado(t *testing.T) {
	n := Notificacion{PSPReference: "7914073381342284", MerchantAccountCode: "Cuenta_123", EventCode: "AUTHORISATION", Success: "true"}
	n.Amount.Value, n.Amount.Currency = 1130, "EUR"
	e, err := n.Estado(pagos.ModoPrueba, map[string]string{pagos.MetadatoPago: "pay_123"})
	if err != nil {
		t.Fatalf("Estado() = %v", err)
	}
	if e.Estado != pagos.EstadoPagado || e.Importe != 1130 || e.Comercio != "Cuenta_123" {
		t.Fatalf("Estado() = %#v", e)
	}

	cancelFail := n
	cancelFail.EventCode = "CANCELLATION"
	cancelFail.Success = "false"
	if ec, err := cancelFail.Estado(pagos.ModoPrueba, nil); err != nil || ec.Estado != pagos.EstadoAbierto {
		t.Fatalf("CANCELLATION fallida = %#v, %v; quiero EstadoAbierto", ec, err)
	}

	cancelOK := cancelFail
	cancelOK.Success = "true"
	if ec, err := cancelOK.Estado(pagos.ModoPrueba, nil); err != nil || ec.Estado != pagos.EstadoCancelado {
		t.Fatalf("CANCELLATION exitosa = %#v, %v; quiero EstadoCancelado", ec, err)
	}

	ref := n
	ref.EventCode = "REFUND"
	if er, err := ref.Estado(pagos.ModoPrueba, nil); err != nil || er.Reembolsado != 1130 {
		t.Fatalf("REFUND = %#v, %v; quiero Reembolsado=1130", er, err)
	}
}
