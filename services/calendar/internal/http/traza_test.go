// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendarhttp

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/auditlog"
)

// Cada regla de la traza casa con una ruta que existe (una ruta renombrada
// dejaría de anotarse en silencio), y la lectura admin de la traza se monta.
func TestReglasDeTrazaExisten(t *testing.T) {
	r, ok := NewRouter(Deps{Traza: auditlog.NuevoRegistro(nil, nil)}).(chi.Routes)
	if !ok {
		t.Fatal("el router no es un chi.Routes")
	}
	reglas := append([]auditlog.Regla{{Metodo: http.MethodGet, Ruta: "/api/v1/admin/audit-log"}}, ReglasDeTraza...)
	if faltan := auditlog.ReglasSinRuta(r, reglas); len(faltan) > 0 {
		t.Fatalf("reglas de la traza sin ruta: %+v", faltan)
	}
}
