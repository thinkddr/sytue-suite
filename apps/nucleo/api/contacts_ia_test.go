// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestLeerFichaComoLaWeb es `fichaDe` de lib/ai.ts: el JSON dentro de un bloque de
// código y con prosa alrededor vale; sin nombre no hay ficha; los correos que no lo son
// y los teléfonos de menos de seis cifras se caen; un correo suelto en vez de una lista
// se acepta; y los repetidos se quitan.
func TestLeerFichaComoLaWeb(t *testing.T) {
	f, ok := LeerFicha("Aquí tienes:\n```json\n{\"nombre\":\" Ana Ruiz \",\"organizacion\":\"Acme S.L.\"," +
		"\"correos\":[\"ana.ruiz@acme.es\",\"acme.es\",\"ana.ruiz@acme.es\"],\"telefonos\":[\"+34 600 11 22 33\",\"123\",\"tel: 600\"]," +
		"\"nota\":\"Directora Comercial\"}\n```")
	if !ok || f.Nombre != "Ana Ruiz" || f.Organizacion != "Acme S.L." || f.Nota != "Directora Comercial" ||
		!slices.Equal(f.Correos, []string{"ana.ruiz@acme.es"}) || !slices.Equal(f.Telefonos, []string{"+34 600 11 22 33"}) {
		t.Fatalf("%+v, %v", f, ok)
	}
	if f, ok := LeerFicha(`{"nombre":"Bruno","correos":"bruno@acme.es"}`); !ok || !slices.Equal(f.Correos, []string{"bruno@acme.es"}) {
		t.Errorf("un correo suelto: %+v, %v", f, ok)
	}
	for _, malo := range []string{`{"organizacion":"Acme"}`, `{"nombre":"  "}`, "no sé", ""} {
		if _, ok := LeerFicha(malo); ok {
			t.Errorf("%q ha dado una ficha", malo)
		}
	}
	// La instrucción es la de la web, letra a letra.
	if web := webref.Read(t, "apps/frontend/apps/contacts/src/lib/ai.ts"); !strings.Contains(web, "'Omite las claves que el texto no diga.'") ||
		!strings.Contains(web, "{nombre, organizacion, correos, telefonos, nota}") {
		t.Error("lib/ai.ts ha cambiado la instrucción: revisar InstruccionFicha")
	}
}
