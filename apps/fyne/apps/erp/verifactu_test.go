// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLaTarjetaDeVerifactuMideLoDeLaWeb: activada, sin activar (con «Activar») y con un
// motivo que lo impide, a 1280; y activada a 360, donde los enlaces bajan de línea y se
// quedan a la izquierda.
//
// VISTO EN ROJO (frontal, 2026-09-23): empujando los enlaces a la derecha también cuando
// bajan de línea (sin mirar si comparten la del hueco), en el móvil «Declaración
// responsable» sale pegada al borde derecho y no a la izquierda.
func TestLaTarjetaDeVerifactuMideLoDeLaWeb(t *testing.T) {
	desde := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	tarjeta := func(e api.ERPEstadoVerifactu) *TarjetaVF {
		return &TarjetaVF{tr: traductor(t), loc: "es", Estado: &e, Administrador: true}
	}
	activa := api.ERPEstadoVerifactu{Activo: true, Desde: &desde, Pendientes: 1, Rechazadas: 1, Entorno: "pruebas"}
	for _, c := range []struct {
		que         string
		e           api.ERPEstadoVerifactu
		ancho, alto float32
	}{
		// TarjetaVerifactu real, Chromium/Figtree/rem16, 2026-10-04.
		{"activada", activa, 978, 66},
		{"sin activar", api.ERPEstadoVerifactu{Entorno: "pruebas"}, 978, 140.5},
		{"con motivo", api.ERPEstadoVerifactu{Motivo: "sin_certificado", Entorno: "produccion"}, 978, 130.5},
		{"activada en el móvil", activa, 332, 118},
		{"sin activar en el móvil", api.ERPEstadoVerifactu{Entorno: "pruebas"}, 332, 210},
	} {
		_, alto := tarjeta(c.e).Objeto(tema.Claro, c.ancho)
		cerca(t, "la tarjeta "+c.que, alto, c.alto)
	}
	o, _ := tarjeta(activa).Objeto(tema.Claro, 332)
	o.Resize(fyne.NewSize(332, 118))
	recorrer(o, func(x fyne.CanvasObject) {
		if e, ok := x.(interface{ AccessibilityLabel() string }); ok && e.AccessibilityLabel() == "Declaración responsable" {
			if x.Position().X > 20 {
				t.Errorf("en el móvil el enlace baja a la izquierda; está en x = %.2f", x.Position().X)
			}
		}
	})
}

// TestLaTarjetaDeVerifactuVaEncimaDeLasFacturas: solo en las facturas de cliente de una
// organización española y fuera de la papelera.
func TestLaTarjetaDeVerifactuVaEncimaDeLasFacturas(t *testing.T) {
	p, w := pantallaArnes(t, fyne.NewSize(1280, 800))
	defer w.Close()
	p.Organizacion, p.SabeOrg = &api.ERPOrganizacion{Pais: "ES"}, true
	p.verifactu = &api.ERPEstadoVerifactu{Entorno: "pruebas"}
	p.Permisos.Administrador = true
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-verifactu.png")
	if !contiene(textosDe(p.vista(tema.Claro)), "VERI*FACTU") {
		t.Error("las facturas de cliente no llevan la tarjeta")
	}
	p.Organizacion.Pais = "FR"
	if contiene(textosDe(p.vista(tema.Claro)), "VERI*FACTU") {
		t.Error("fuera de España no hay VERI*FACTU")
	}
}
