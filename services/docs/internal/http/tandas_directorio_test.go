// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"fmt"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
)

// Las tandas caben en los topes de Sytue ID y no se dejan a nadie: una dirección con
// más candidatos de los que caben va repartida en varias.
func TestTandasDirectorio(t *testing.T) {
	candidatos := map[string]map[string]bool{}
	for i := range 150 {
		candidatos[fmt.Sprintf("p%03d@x.es", i)] = map[string]bool{fmt.Sprintf("s%d", i): true}
	}
	muchos := map[string]bool{}
	for i := range 2500 {
		muchos[fmt.Sprintf("m%04d", i)] = true
	}
	candidatos["zzz@x.es"] = muchos

	vistos := map[string]map[string]bool{}
	for _, c := range tandasDirectorio(candidatos) {
		n := 0
		for _, subs := range c.Candidatos {
			n += len(subs)
			if len(subs) > peticiones.MaxCandidatosPorDireccion {
				t.Fatalf("una dirección lleva %d candidatos en una tanda", len(subs))
			}
		}
		if len(c.Emails) != 0 || len(c.Candidatos) > peticiones.MaxDireccionesDirectorio || n > peticiones.MaxCandidatosDirectorio {
			t.Fatalf("una tanda se pasa de los topes: %d direcciones, %d con candidatos, %d candidatos",
				len(c.Emails), len(c.Candidatos), n)
		}
		for e, subs := range c.Candidatos {
			if vistos[e] == nil {
				vistos[e] = map[string]bool{}
			}
			for _, s := range subs {
				vistos[e][s] = true
			}
		}
	}
	for e, subs := range candidatos {
		for s := range subs {
			if !vistos[e][s] {
				t.Fatalf("el candidato %s de %s no se preguntó", s, e)
			}
		}
	}
}
