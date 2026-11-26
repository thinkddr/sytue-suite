// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package banca

import "strings"

// Motivos por los que un movimiento del banco no lleva factura, en el orden en
// que se ofrecen. Cada uno es una explicación que entiende cualquiera («una
// comisión del banco») y detrás, la cuenta del Plan General Contable a la que va.
var Motivos = []string{
	"comision_bancaria", "intereses", "impuestos", "seguridad_social", "nomina",
	"traspaso", "prestamo", "socios", "otro",
}

// CuentaDeMotivo es la cuenta de la contrapartida del banco para un movimiento
// sin factura. `ingreso` es si entra dinero; `personaFisica`, si quien lleva la
// empresa es un autónomo y no una sociedad.
//
// Criterio contable (PGC):
//   - comisiones del banco, 626; intereses, 662 si se pagan y 769 si se cobran;
//   - un pago a Hacienda cancela lo que se le debe (475) y una devolución, lo que
//     nos debe (470);
//   - la Seguridad Social de una sociedad cancela lo devengado en las nóminas
//     (476); la cuota de un autónomo es gasto suyo (642);
//   - una nómina pagada cancela la remuneración pendiente (465);
//   - un traspaso entre cuentas propias no cambia nada (572 contra 572);
//   - un préstamo, deuda con el banco a corto plazo (520);
//   - el dinero que pone o retira el titular (550) o un socio (551);
//   - lo que no se sabe va a partidas pendientes de aplicación (555), que
//     es donde un contable lo buscará.
func CuentaDeMotivo(motivo string, ingreso, personaFisica bool) (string, bool) {
	switch motivo {
	case "comision_bancaria":
		return "626000", true
	case "intereses":
		return elegir(ingreso, "769000", "662000"), true
	case "impuestos":
		return elegir(ingreso, "470000", "475000"), true
	case "seguridad_social":
		return elegir(personaFisica, "642000", "476000"), true
	case "nomina":
		return "465000", true
	case "traspaso":
		return "572000", true
	case "prestamo":
		return "520000", true
	case "socios":
		return elegir(personaFisica, "550000", "551000"), true
	case "otro":
		return "555000", true
	}
	return "", false
}

func elegir(si bool, a, b string) string {
	if si {
		return a
	}
	return b
}

// MotivoEvidente reconoce los movimientos que se explican solos: una comisión que
// el banco llama comisión, un cargo de la Agencia Tributaria o de la Tesorería de
// la Seguridad Social. Esos no merecen preguntar a nadie. Cualquier otro caso
// devuelve "" y se pregunta.
func MotivoEvidente(m Movimiento) string {
	t := " " + normalizar(m.Descripcion+" "+m.Referencia2) + " "
	switch {
	case contieneAlguna(t, " AEAT ", "AGENCIA TRIBUTARIA", "AGENCIA ESTATAL DE ADMINISTRACION TRIBUTARIA"):
		return "impuestos"
	case contieneAlguna(t, " TGSS ", "TESORERIA GENERAL", "SEGURIDAD SOCIAL"):
		return "seguridad_social"
	case m.ConceptoComun == "17" && m.Centimos < 0 && contieneAlguna(t, "COMISION", "MANTENIMIENTO"):
		return "comision_bancaria"
	}
	return ""
}

// MotivoProbable es la sugerencia para la pregunta: lo que el concepto del banco
// deja intuir, sin aplicarlo.
func MotivoProbable(m Movimiento) string {
	if motivo := MotivoEvidente(m); motivo != "" {
		return motivo
	}
	t := " " + normalizar(m.Descripcion+" "+m.Referencia2) + " "
	switch {
	case contieneAlguna(t, "NOMINA"):
		return "nomina"
	case contieneAlguna(t, "TRASPASO"):
		return "traspaso"
	case m.ConceptoComun == "05" || contieneAlguna(t, "PRESTAMO", "AMORTIZACION"):
		return "prestamo"
	case contieneAlguna(t, "INTERES"):
		return "intereses"
	case m.ConceptoComun == "17":
		return "comision_bancaria"
	}
	return ""
}

func contieneAlguna(t string, trozos ...string) bool {
	for _, x := range trozos {
		if strings.Contains(t, x) {
			return true
		}
	}
	return false
}
