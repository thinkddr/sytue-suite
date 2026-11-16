// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los tipos con formato aceptan y rechazan lo mismo que `lib/formatos.test.ts` en la web
// (y que el servicio, que es el que manda).
func TestLosTiposConFormato(t *testing.T) {
	casos := []struct {
		kind, texto string
		want        any
		ok          bool
	}{
		{api.BasesKindURL, "sytue.eu/precios", "https://sytue.eu/precios", true},
		{api.BasesKindURL, "https://Sytue.EU", "https://sytue.eu", true},
		{api.BasesKindURL, "javascript:alert(1)", nil, false},
		{api.BasesKindURL, "ftp://sytue.eu", nil, false},
		{api.BasesKindEmail, "Ana@Ejemplo.ES", "Ana@ejemplo.es", true},
		{api.BasesKindEmail, "Ana <ana@ejemplo.es>", nil, false},
		{api.BasesKindEmail, "ana@localhost", nil, false},
		{api.BasesKindPhone, "+34  600 12 34 56", "+34 600 12 34 56", true},
		{api.BasesKindPhone, "12", nil, false},
		{api.BasesKindCurrency, "12,345", 12.35, true},
		{api.BasesKindRating, "4", 4.0, true},
		{api.BasesKindRating, "0", nil, true},
		{api.BasesKindRating, "6", nil, false},
		{api.BasesKindRating, "2.5", nil, false},
	}
	for _, c := range casos {
		got, ok := valorDeTexto(api.BasesField{Kind: c.kind}, c.texto)
		if ok != c.ok || got != c.want {
			t.Errorf("%s %q: %v %v; se esperaba %v %v", c.kind, c.texto, got, ok, c.want, c.ok)
		}
	}
}

func TestUnImporteYUnaValoracionSeLeenConFormato(t *testing.T) {
	eur := api.BasesField{Kind: api.BasesKindCurrency, Currency: "EUR"}
	if got := strings.ReplaceAll(textoVisible(eur, 1234.5, i18n.Locale("es")), " ", " "); got != "1234,50 €" {
		t.Errorf("importe en es: %q", got)
	}
	if got := textoVisible(api.BasesField{Kind: api.BasesKindRating}, 3.0, i18n.Locale("es")); got != "★★★☆☆" {
		t.Errorf("valoración: %q", got)
	}
	if efectivo(eur).Kind != api.BasesKindNumber || efectivo(api.BasesField{Kind: api.BasesKindEmail}).Kind != api.BasesKindText {
		t.Error("moneda y correo se filtran como número y texto")
	}
	if !alineaALaDerecha(eur) || alineaALaDerecha(api.BasesField{Kind: api.BasesKindRating}) {
		t.Error("el importe va a la derecha y las estrellas no")
	}
	if !tecleable(api.BasesKindURL) || tecleable(api.BasesKindSelect) {
		t.Error("una URL se edita al teclear; una elección, no")
	}
}
