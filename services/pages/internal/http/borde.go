// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/dominio"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// montarBorde registra el ask de Caddy, GET /_borde/permitido (§6.5).
//
// El cubo solo cubre lo que va a la base: un host del mapa se contesta de memoria y sin
// cubo. Antes el cubo era uno para todo el ask, y quien lo inundaba con dominios
// inventados dejaba a Caddy sin poder emitir ni renovar los certificados de las webs
// publicadas (WEB-11).
func montarBorde(g grupos, h *Handlers) {
	cubo := httpx.NewTokenBucket(20, 20)
	g.borde.Get("/_borde/permitido", func(w http.ResponseWriter, r *http.Request) {
		h.bordePermitido(w, r, cubo)
	})
}

func (h *Handlers) bordePermitido(w http.ResponseWriter, r *http.Request, cubo httpx.RateLimiter) {
	dom := r.URL.Query().Get("domain")
	if dom == "" {
		http.Error(w, "missing domain", http.StatusNotFound)
		return
	}

	host, err := dominio.Normalizar(dom)
	if err != nil {
		http.Error(w, "invalid domain", http.StatusNotFound)
		return
	}

	// 1. Si está en el mapa vigente de hosts publicados (§8.2, §8.3)
	if h.Lector != nil && h.Lector.Listo() {
		if _, _, ok := h.Lector.ObtenerPorHost(host); ok {
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	// 2. Si es un dominio verified pendiente de activar (§8.3)
	if h.Store != nil {
		if res, err := cubo.Allow(r.Context(), "borde"); err != nil || !res.Allowed {
			http.Error(w, "busy", http.StatusTooManyRequests)
			return
		}
		if verificado, err := h.Store.EsDominioVerificado(r.Context(), host); err == nil && verificado {
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	http.Error(w, "domain not allowed", http.StatusNotFound)
}
