// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http_test

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/auditlog"
	docshttp "git.sytue.eu/sytue/one/services/docs/internal/http"
)

// Cada regla de la traza casa con una ruta que existe (una ruta renombrada
// dejaría de anotarse en silencio), y la lectura admin de la traza se monta.
func TestReglasDeTrazaExisten(t *testing.T) {
	handler := docshttp.New(docshttp.Deps{Traza: auditlog.NuevoRegistro(nil, nil)})
	router := chi.NewRouter()
	handler.Mount(router)
	reglas := append([]auditlog.Regla{{Metodo: http.MethodGet, Ruta: "/api/v1/admin/audit-log"}}, docshttp.ReglasDeTraza...)
	if faltan := auditlog.ReglasSinRuta(router, reglas); len(faltan) > 0 {
		t.Fatalf("reglas de la traza sin ruta: %+v", faltan)
	}
}
