// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package drivers construye el registro de drivers de la pasarela.
//
// Existe para que el arranque y los tests usen EXACTAMENTE el mismo conjunto: la
// comprobación de que toda capacidad del catálogo resuelve a un driver real (ver
// db/seeds/seeds_test.go) solo vale si comprueba contra el registro de verdad. Con la
// lista escrita en main.go, un driver añadido allí y no en el test dejaba el hueco por
// el que se coló que el aprovisionamiento no funcionara en ninguna plantilla.
package drivers

import (
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/databricks"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/graph"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/gws"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/holded"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/jira"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/odata"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/odoo"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/powerbi"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/salesforce"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/servicenow"
	sytueoauth2 "git.sytue.eu/sytue/one/services/integrations/internal/oauth2"
	"git.sytue.eu/sytue/one/services/integrations/internal/scimout"
)

// Registry devuelve el registro con todos los drivers de la pasarela, indexados por
// Kind. El despacho es por CAPACIDAD: el manifiesto de cada plantilla nombra el Kind que
// sirve cada una (`provisioning: "scim"`, `data: "jira"`), así que una misma instancia
// puede usar varios drivers a la vez.
//
// `dir` (directorio de Sytue ID: el estado deseado) y `st` (scim_map + sync_runs) solo
// los necesita el driver SCIM; un test que únicamente inspeccione Kinds puede pasar nil.
func Registry(dir scimout.Directory, st scimout.MapStore) *connector.Registry {
	// Caché de access token COMPARTIDA por los drivers que hablan OAuth 2.0. Vive aquí
	// —y no dentro de cada Runtime— porque el Runtime se fabrica por petición: una caché
	// por petición no cachearía nada entre peticiones, que es justo lo que pasaba antes
	// (Microsoft Graph minteaba un token nuevo en cada consulta).
	tokens := sytueoauth2.NewCache()
	return connector.NewRegistry(
		// Aprovisionamiento SCIM 2.0 saliente: crea, actualiza y DESACTIVA (nunca borra)
		// usuarios y grupos en el SaaS, reconciliando por diff contra Sytue ID.
		scimout.NewDriver(dir, st),
		// Datos tabulares (L3) hacia Sytue Sheets. Los que además implementan
		// SearchCapability alimentan la búsqueda federada de Sytue Hub.
		odata.New(),
		odoo.New(),
		holded.New(),
		databricks.New(),
		powerbi.New(),
		graph.New(tokens),
		gws.New(tokens),
		salesforce.New(),
		servicenow.New(),
		jira.New(),
	)
}
