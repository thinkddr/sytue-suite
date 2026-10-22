// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package config

// Secretos de desarrollo de las puertas entre Sytue Web y los servicios con los que
// habla (PAGES-ENCARGO §2.1). Los importan LOS DOS extremos —Pages y el servicio que
// abre la puerta—, así que en dev casan sin configurar nada. Están escritos aquí, así
// que son públicos: fuera de dev SecretoOpcional los rechaza y el binario no arranca.
const (
	// DevPagesPublicadorSecret es la verja de las publicaciones y exportaciones de
	// Pages (`X-Sytue-Publicador`), entre el BFF de la app y el servicio.
	DevPagesPublicadorSecret = "dev-pages-publicador-secret" //nolint:gosec // G101: es el de desarrollo, público a propósito; fuera de dev se rechaza
	// DevIDEstadoOrgsSecret abre `POST /internal/orgs/status` de Sytue ID.
	DevIDEstadoOrgsSecret = "dev-id-estado-orgs-secret" //nolint:gosec // G101: ídem
	// DevSyERPPedidosSecret abre las rutas internas de pedidos de tienda de SyERP.
	DevSyERPPedidosSecret = "dev-syerp-pedidos-tienda-secret" //nolint:gosec // G101: ídem
	// DevAIPagesSecret abre `/internal/v1/assist` y `/internal/v1/models` de Sytue AI.
	DevAIPagesSecret = "dev-ai-pages-secret" //nolint:gosec // G101: ídem
	// DevAnalyticsPagesSecret abre `POST /internal/v1/sites` de analytics-api.
	DevAnalyticsPagesSecret = "dev-analytics-pages-secret" //nolint:gosec // G101: ídem
)
