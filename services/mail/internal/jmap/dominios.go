// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"net/http"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// RutaDominios da los dominios de correo dados de alta en Sytue Mail. La pide
// Calendar para saber qué asistentes son de la suite (les llega la invitación a su
// bandeja) y cuáles de fuera (se les manda por correo iMIP). Va bajo /internal, que
// el Ingress no publica, y con el secreto de la suite: la lista de organizaciones que
// tienen correo aquí no es pública.
const RutaDominios = "/internal/domains"

// listarDominios son los de todas las direcciones, más el principal y el de los
// canales, que son de la suite aunque todavía no tengan ninguna cuenta.
func (a *API) listarDominios(w http.ResponseWriter, r *http.Request) {
	ds, err := a.store.Dominios(r.Context())
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	for _, d := range []string{a.cfg.PrimaryDomain, a.cfg.ChannelMailDomain} {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
			ds = append(ds, d)
		}
	}
	slices.Sort(ds)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"domains": slices.Compact(ds)})
}
