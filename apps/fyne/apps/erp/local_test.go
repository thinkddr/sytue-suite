// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"net/http"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El código que solo contesta el ERP de Sytue Office se cuenta con su texto, en el idioma
// de quien mira; los de la suite siguen saliendo del catálogo.
func TestErrorNoDisponibleLocal(t *testing.T) {
	for idioma, quiere := range map[string]string{"es": ErroresLocales["no_disponible_local"][0], "en": ErroresLocales["no_disponible_local"][1]} {
		tr, err := Traductor(i18n.Locale(idioma))
		if err != nil {
			t.Fatal(err)
		}
		e := &api.Error{Status: http.StatusNotImplemented, Body: `{"error":{"code":"no_disponible_local","message":"x"}}`}
		if got := MensajeDeError(e, tr); got != quiere {
			t.Fatalf("%s: %q", idioma, got)
		}
		if _, ok := Errores["no_disponible_local"]; ok {
			t.Fatal("la tabla de la web no lleva el código local")
		}
	}
}

// «Llevar al libro» está siempre salvo que la ficha diga SinAsentar (el valor cero es
// el de la edición conectada).
func TestSinAsentarOcultaLlevarAlLibro(t *testing.T) {
	r := facturasArnes().Registros[0]
	cuantos := func(sin bool) int {
		f := NuevaFicha(traductor(t), facturaArnes(), &r, nil, nil)
		f.SinAsentar = sin
		obj, _ := f.pie()
		return len(obj.(*fyne.Container).Objects)
	}
	if con, sin := cuantos(false), cuantos(true); con != sin+1 {
		t.Fatalf("con asentar %d botones, sin asentar %d", con, sin)
	}
}
