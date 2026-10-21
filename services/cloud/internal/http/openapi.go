// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"bytes"
	"net/http"
	"strconv"
	"time"

	cloud "git.sytue.eu/sytue/one/services/cloud"
)

// openapiMaxAge es la caché del contrato: solo cambia al desplegar una versión
// nueva, y una hora evita que los generadores de SDK lo repidan en cada ejecución.
const openapiMaxAge = time.Hour

// ServeOpenAPI devuelve el contrato de la API pública.
//
// El fichero existía completo (OpenAPI 3.1, ~48 rutas) y NO se servía por ningún
// sitio, así que no había forma de generar clientes ni de explorar la API sin leer
// el código Go — teniendo el producto un CLI y un provider de Terraform que salen
// justamente de ese contrato.
//
// Es PÚBLICO a propósito, como el discovery de OIDC: describe la superficie, no
// expone ningún dato de ningún tenant, y exigir credenciales para leerlo solo
// estorbaría a quien quiere generar un cliente.
func (h *Handlers) ServeOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/yaml; charset=utf-8")
	w.Header().Set("cache-control", "public, max-age="+strconv.Itoa(int(openapiMaxAge.Seconds())))
	// ServeContent aporta Range y If-Modified-Since gratis; con modtime cero no
	// emite Last-Modified (el contrato no tiene fecha propia, solo versión).
	http.ServeContent(w, r, "openapi.yaml", time.Time{}, bytes.NewReader(cloud.OpenAPISpec))
}
