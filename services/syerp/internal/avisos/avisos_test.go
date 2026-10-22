// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package avisos

import (
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/syerp/internal/obligaciones"
)

func TestTocanYCorreo(t *testing.T) {
	t.Parallel()
	dia := func(s string) time.Time {
		d, _ := time.Parse(time.DateOnly, s)
		return d
	}
	dom := dia("2026-10-15")
	i303 := obligaciones.Instancia{
		Clave: "es_303", Nombre: "Modelo 303", Periodo: "2026-T3",
		Limite: dia("2026-10-20"), Recomendada: dia("2026-10-08"), LimiteDomiciliacion: &dom,
		Preparacion: []string{"banco", "facturas"},
		Guia:        &obligaciones.Guia{Que: "El IVA del trimestre.", Pasos: []string{"Pulsa «Preparar cifras».", "Preséntalo."}, SiNo: "Recargo."},
	}
	otra := i303
	otra.Clave, otra.Recomendada = "es_111", dia("2026-10-09")

	tocan := Tocan([]obligaciones.Instancia{i303, otra}, dia("2026-09-28"))
	if len(tocan) != 1 || tocan[0].Clave != "es_303" {
		t.Fatalf("el 28 de septiembre toca el aviso del 303 (tu fecha el 8 de octubre): %+v", tocan)
	}

	asunto, cuerpo := Correo("Acme SL", "ES", i303, map[string]int{"banco": 3, "facturas": 1, "nominas": 2}, "https://erp.sytue.eu/erp")
	if asunto != "Quedan 10 días: Modelo 303 · 3T 2026" {
		t.Errorf("asunto: %q", asunto)
	}
	for _, quiero := range []string{
		"Tu fecha: 8 de octubre de 2026", "Si domicilias el pago, hasta el 15 de octubre de 2026", "Plazo legal: 20 de octubre de 2026",
		"- 3 movimientos del banco sin factura", "- 1 factura sin llevar al libro", "1. Pulsa «Preparar cifras».", "https://erp.sytue.eu/erp",
	} {
		if !strings.Contains(cuerpo, quiero) {
			t.Errorf("el correo no dice %q:\n%s", quiero, cuerpo)
		}
	}
	if strings.Contains(cuerpo, "nóminas") {
		t.Error("las nóminas no son preparación del 303: no se mencionan")
	}
}
