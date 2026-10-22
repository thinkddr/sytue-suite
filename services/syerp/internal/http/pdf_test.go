// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// El PDF de una factura se lee con Sytue PDF y sale en orden de lectura: línea a
// línea de arriba abajo aunque los fragmentos lleguen desordenados, y un escaneo
// sin texto es un error, no un texto vacío que la IA intentaría rellenar.
func TestTextoDePDF(t *testing.T) {
	var pedido string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fichero, _, err := r.FormFile("file")
		if err != nil || r.URL.Path != "/api/v1/pdf/text" || r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		cuerpo, _ := io.ReadAll(fichero)
		pedido = string(cuerpo)
		if pedido == "%PDF-escaneo" {
			_, _ = io.WriteString(w, `{"paginas":[[]]}`)
			return
		}
		_, _ = io.WriteString(w, `{"paginas":[[
			{"texto":"121,00 €","x":400,"y":100},
			{"texto":"Anthropic, PBC","x":50,"y":700},
			{"texto":"Total","x":50,"y":101},
			{"texto":"Factura INV-42","x":300,"y":700.5}
		],[{"texto":"Página 2","x":50,"y":700}]]}`)
	}))
	defer srv.Close()

	texto, err := textoDePDF(context.Background(), srv.URL, "tok", []byte("%PDF-1.7"))
	if err != nil || pedido != "%PDF-1.7" {
		t.Fatalf("%v (%q)", err, pedido)
	}
	if want := "Anthropic, PBC Factura INV-42\nTotal 121,00 €\n\nPágina 2"; texto != want {
		t.Fatalf("texto:\n%q\nse esperaba:\n%q", texto, want)
	}
	if _, err := textoDePDF(context.Background(), srv.URL, "tok", []byte("%PDF-escaneo")); err == nil || !strings.Contains(err.Error(), "no tiene texto") {
		t.Fatalf("un escaneo: %v", err)
	}
	if _, err := textoDePDF(context.Background(), srv.URL, "otro", []byte("%PDF-1.7")); err == nil {
		t.Fatal("un 400 de Sytue PDF es un error")
	}
}
