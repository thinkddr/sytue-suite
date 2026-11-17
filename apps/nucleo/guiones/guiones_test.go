// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package guiones

import (
	"strings"
	"testing"
)

func TestDecidirCuota(t *testing.T) {
	casos := []struct {
		u     UsoCuota
		maxMs int
		want  string
	}{
		{UsoCuota{}, TopeMs, CuotaLibre},
		{UsoCuota{EjecucionesUsuario: CuotaEjecucionesUsuarioHora - 1, EjecucionesOrg: CuotaEjecucionesOrgHora - 1}, 100, CuotaLibre},
		{UsoCuota{EjecucionesUsuario: CuotaEjecucionesUsuarioHora}, 100, CuotaUsuario},
		{UsoCuota{EjecucionesOrg: CuotaEjecucionesOrgHora}, 100, CuotaOrg},
		{UsoCuota{MsUsuario: CuotaMsUsuarioHora - 100}, 100, CuotaLibre},
		{UsoCuota{MsUsuario: CuotaMsUsuarioHora - 99}, 100, CuotaTiempo},
	}
	for i, c := range casos {
		if got := DecidirCuota(c.u, c.maxMs); got != c.want {
			t.Errorf("caso %d: %q, se esperaba %q", i, got, c.want)
		}
	}
}

func TestPuedeEjecutar(t *testing.T) {
	for _, estado := range []string{EstadoBorrador, EstadoPendiente, EstadoAprobada, EstadoRechazada} {
		if !PuedeEjecutar("ana", "ana", false, estado) {
			t.Errorf("la autora no puede probar su versión %s", estado)
		}
		// Privado: nadie más, ni aprobado.
		if PuedeEjecutar("ana", "luis", false, estado) {
			t.Errorf("otra persona ejecuta un script privado (%s)", estado)
		}
		if got := PuedeEjecutar("ana", "luis", true, estado); got != (estado == EstadoAprobada) {
			t.Errorf("compartido en %s: %v", estado, got)
		}
	}
}

func TestCodigoHashYFormas(t *testing.T) {
	if CodigoValido("") || CodigoValido(strings.Repeat("x", MaxCodigo+1)) || !CodigoValido(strings.Repeat("x", MaxCodigo)) {
		t.Error("cotas del código")
	}
	if h := HashCodigo("a"); h != "ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb" || !ReHash.MatchString(h) {
		t.Errorf("hash %q", h)
	}
	for _, mal := range []string{"Con Espacio", "ñ", strings.Repeat("a", 65), "a\nb"} {
		if ReCodigoError.MatchString(mal) {
			t.Errorf("código de error admitido: %q", mal)
		}
	}
	if !ReCodigoError.MatchString("") || !ReCodigoError.MatchString("max_ops") {
		t.Error("código de error válido rechazado")
	}
	if ResultadoFinal(ResEjecutando) || ResultadoFinal("otro") || !ResultadoFinal(ResLimite) {
		t.Error("resultados finales")
	}
}
