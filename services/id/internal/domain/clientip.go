// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "git.sytue.eu/sytue/one/platform/pkg/clientip"

// La lista de proxies de confianza VIVE AHORA EN platform/pkg/clientip, porque
// la necesitaba también el middleware de registro de httpx y estaba encerrada
// aquí (auditoría del 2026-09-21, docs/security.md §6.3). Estos alias existen
// para que el resto de Sytue ID —http, scim, sus tests— siga nombrándola por el
// dominio, que es donde tiene sentido de negocio.
type TrustedProxies = clientip.TrustedProxies

var (
	DefaultTrustedProxies = clientip.DefaultTrustedProxies
	NoTrustedProxies      = clientip.NoTrustedProxies
	ParseTrustedProxies   = clientip.ParseTrustedProxies
	WithTrustedProxies    = clientip.WithTrustedProxies
	TrustedProxiesFrom    = clientip.TrustedProxiesFrom
)
