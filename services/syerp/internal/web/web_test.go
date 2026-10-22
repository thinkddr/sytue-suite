// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package web

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestComprobaciones(t *testing.T) {
	vies := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ms/IE/vat/6388047V":
			_, _ = w.Write([]byte(`{"isValid":true,"userError":"VALID","name":"GOOGLE IRELAND LIMITED","address":"---"}`))
		case "/ms/DE/vat/123456789":
			_, _ = w.Write([]byte(`{"isValid":false,"userError":"MS_UNAVAILABLE","name":"---"}`))
		default:
			_, _ = w.Write([]byte(`{"isValid":false,"userError":"INVALID"}`))
		}
	}))
	defer vies.Close()
	c := &Comprobador{
		hc: vies.Client(), vies: vies.URL,
		mx: func(_ context.Context, d string) ([]*net.MX, error) {
			if d == "anthropic.com" {
				return []*net.MX{{Host: "mx.anthropic.com."}}, nil
			}
			return nil, errors.New("sin MX")
		},
		hosts: func(context.Context, string) ([]string, error) { return nil, errors.New("no resuelve") },
	}
	ctx := context.Background()

	if r := c.IVA(ctx, "ie 6388047v"); !r.Comprobado || !r.Valido || r.Nombre != "GOOGLE IRELAND LIMITED" || r.Direccion != "" {
		t.Fatalf("un NIF-IVA irlandés válido trae su nombre y sin «---»: %+v", r)
	}
	if r := c.IVA(ctx, "IE1234567X"); !r.Comprobado || r.Valido {
		t.Fatalf("uno inventado es un no rotundo: %+v", r)
	}
	if r := c.IVA(ctx, "DE123456789"); r.Comprobado {
		t.Fatalf("un país que no contesta no dice que sea falso: %+v", r)
	}
	if r := c.IVA(ctx, "ESB12345678"); r.Comprobado {
		t.Fatalf("un NIF español no se manda al VIES: %+v", r)
	}
	if p, n, ok := PartirNIFIVA("GR-094014201"); !ok || p != "EL" || n != "094014201" {
		t.Fatalf("Grecia es EL en el VIES: %s %s %v", p, n, ok)
	}
	if !c.Correo(ctx, "billing@anthropic.com") || c.Correo(ctx, "facturas@no-existe-123.es") || c.Correo(ctx, "Ana <a@anthropic.com>") {
		t.Fatal("un correo vale si su dominio recibe correo, y solo la dirección")
	}
	if c.URL(ctx, "http://anthropic.com") || c.URL(ctx, "javascript:alert(1)") {
		t.Fatal("solo https")
	}
}
