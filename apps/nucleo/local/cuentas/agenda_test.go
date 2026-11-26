// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cuentas

import (
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// Las cuentas de calendario y contactos (`local/agenda`) salen con sus servidores por
// servicio, y el portero los abre con el propósito de cada uno.
func TestCuentasDeAgenda(t *testing.T) {
	ajustes, datos := t.TempDir(), t.TempDir()
	escribir(t, filepath.Join(ajustes, "agenda.json"), `{"cuentas":[{"id":"a1","proveedor":"icloud","rotulo":"iCloud","usuario":"ana@icloud.com","servicios":["calendario","contactos"],
"servidores":[{"servicio":"calendario","host":"caldav.icloud.com"},{"servicio":"calendario","host":"p42-caldav.icloud.com"},{"servicio":"contactos","host":"contacts.icloud.com"},{"servicio":"calendario","host":".icloud.com"}]},
{"id":"a2","proveedor":"google","rotulo":"Google","servicios":["calendario","contactos"],"servidores":[{"servicio":"calendario","host":"www.googleapis.com"}],"oauth":"oauth2.googleapis.com","estado":"reconectar"}]}`)
	r := Abrir(ajustes, datos)
	cs := r.Cuentas()
	if len(cs) != 2 || cs[0].Origen != OrigenAgenda || len(cs[0].Servidores) != 3 || cs[1].Estado != EstadoReconectar {
		t.Fatalf("cuentas: %+v", cs)
	}
	ds := r.Destinos()
	quiere := map[string]red.Proposito{"p42-caldav.icloud.com": red.Calendario, "contacts.icloud.com": red.Contactos, "oauth2.googleapis.com": red.OAuth}
	for host, pr := range quiere {
		ok := false
		for _, d := range ds {
			if d.Host == host && d.Proposito == pr {
				ok = true
			}
		}
		if !ok {
			t.Fatalf("falta %s (%s) en %+v", host, pr, ds)
		}
	}
	for _, d := range ds {
		if d.Host == "icloud.com" || strings.HasPrefix(d.Host, ".") {
			t.Fatalf("un comodín de la cuenta llegó al portero: %+v", d)
		}
	}
}
