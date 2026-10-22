// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package redsys

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pagos"
)

func TestCrearPagoProduceFormularioFirmado(t *testing.T) {
	r, err := New("999008881", "1", "sq7HjrUOBfKmC576ILgskD5srU870gJ7", pagos.ModoPrueba)
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.CrearPago(context.Background(), pagos.Solicitud{
		Centimos: 2910, Divisa: pagos.DivisaEUR, Referencia: "P-000123", Descripcion: "Pedido",
		URLRetorno: "https://site.example/order", URLWebhook: "https://pages.example/webhook",
		Caduca: time.Now().Add(time.Hour), Metadatos: map[string]string{pagos.MetadatoPago: "payment-1"},
	})
	if err != nil || out.Formulario == nil || out.Formulario.Accion != testURL {
		t.Fatalf("CrearPago: %#v, %v", out, err)
	}
	// La tienda pasa el número de pedido (P-000123), que Redsys no admite como
	// Ds_Merchant_Order: la orden sale del id del pago, y es la que se guarda.
	orden := Orden("payment-1")
	if !ordenValido(orden) || out.Ref != orden || Orden("payment-2") == orden {
		t.Fatalf("orden %q, ref %q", orden, out.Ref)
	}
	var params map[string]string
	if err := decodificarParametros(out.Formulario.Campos["Ds_MerchantParameters"], &params); err != nil || params["DS_MERCHANT_ORDER"] != orden {
		t.Fatalf("parámetros %v, %v", params, err)
	}
	if !Verificar(out.Formulario.Campos["Ds_MerchantParameters"], orden, "sq7HjrUOBfKmC576ILgskD5srU870gJ7", out.Formulario.Campos["Ds_Signature"]) {
		t.Fatal("el formulario no está firmado")
	}
}

func TestReembolsarFirmaYRespuesta(t *testing.T) {
	const key = "sq7HjrUOBfKmC576ILgskD5srU870gJ7"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request map[string]string
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if req.URL.Path != "/sis/rest/trataPeticionREST" || !Verificar(request["Ds_MerchantParameters"], "0001ab12cd34", key, request["Ds_Signature"]) {
			t.Fatalf("petición REST inválida: %#v", request)
		}
		params, _ := json.Marshal(map[string]string{"Ds_Response": "0900", "Ds_Order": "0001ab12cd34", "Ds_Amount": "2910"})
		encoded := base64.StdEncoding.EncodeToString(params)
		signature, _ := Firmar(encoded, "0001ab12cd34", key)
		_ = json.NewEncoder(w).Encode(map[string]string{"Ds_MerchantParameters": encoded, "Ds_Signature": signature})
	}))
	defer srv.Close()
	r, err := New("999008881", "1", key, pagos.ModoPrueba)
	if err != nil {
		t.Fatal(err)
	}
	r.restURL = srv.URL + "/sis/rest/trataPeticionREST"
	if err := r.Reembolsar(context.Background(), "0001ab12cd34", 2910, "refund-1"); err != nil {
		t.Fatalf("Reembolsar() = %v", err)
	}
}
