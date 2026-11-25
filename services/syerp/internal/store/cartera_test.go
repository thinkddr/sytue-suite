// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import "testing"

// Los tramos de la antigüedad: el día del vencimiento aún está al día, y cada tramo
// cierra en su tope (30, 60, 90). Es la regla que repite el CASE de la consulta.
func TestTramoDeDias(t *testing.T) {
	casos := map[int]string{
		-5: TramoAlDia, 0: TramoAlDia, 1: Tramo1a30, 30: Tramo1a30, 31: Tramo31a60,
		60: Tramo31a60, 61: Tramo61a90, 90: Tramo61a90, 91: TramoMas90, 400: TramoMas90,
	}
	for dias, quiero := range casos {
		if got := TramoDeDias(dias); got != quiero {
			t.Errorf("%d días: %s, quería %s", dias, got, quiero)
		}
	}
	if len(TramosAntiguedad) != 5 || TramosAntiguedad[0] != TramoAlDia || TramosAntiguedad[4] != TramoMas90 {
		t.Errorf("orden de los tramos: %v", TramosAntiguedad)
	}
}

// Una cantidad sale como se escribió: sin los ceros que `numeric` arrastra.
func TestSinCerosDeMas(t *testing.T) {
	for entrada, quiero := range map[string]string{
		"12.500": "12.5", "3.000": "3", "40": "40", "0.25": "0.25", "-1.50": "-1.5", "100": "100",
	} {
		if got := sinCerosDeMas(entrada); got != quiero {
			t.Errorf("%s → %s, quería %s", entrada, got, quiero)
		}
	}
}

// En el estado de cuenta, una factura abierta es pendiente (y vencida si lleva días), una
// pagada es cobro, y todas cuentan en lo facturado.
func TestSumarACuenta(t *testing.T) {
	var r ResumenCuenta
	sumarACuenta(&r, DocumentoCuenta{Estado: "confirmado", Total: 100, DiasVencida: 3}, true)
	sumarACuenta(&r, DocumentoCuenta{Estado: "confirmado", Total: 50}, true)
	sumarACuenta(&r, DocumentoCuenta{Estado: "pagado", Total: 20}, false)
	if r != (ResumenCuenta{Facturado: 170, Pagado: 20, Pendiente: 150, Vencido: 100}) {
		t.Errorf("resumen: %+v", r)
	}
}
