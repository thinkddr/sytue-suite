// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubhttp

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/auditlog"
)

// ReglasDeTraza son los accesos que deja la traza del Hub (docs/security.md §1.7,
// platform/pkg/auditlog): la exportación de TODO lo de la persona en la suite
// («Tus datos»), la búsqueda federada y la resolución de un intent entre apps.
// De la búsqueda se anota que hubo una, nunca lo que se buscó.
// TestReglasDeTrazaExisten comprueba que cada regla sigue casando con una ruta.
var ReglasDeTraza = []auditlog.Regla{
	{Metodo: http.MethodGet, Ruta: "/api/v1/export", Accion: "export"},
	{Metodo: http.MethodGet, Ruta: "/api/v1/search", Accion: "hub.search"},
	{Metodo: http.MethodPost, Ruta: "/api/v1/intents", Accion: "hub.intent"},
}
