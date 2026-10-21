// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
)

// myApp es un tile «como producto Sytue» de una app conectada: la MISMA forma que un
// SuiteApp del launcher (key/name/description/href/iconUrl/color) + su estado. href es
// la URL de lanzamiento resuelta al activar (SSO IdP-initiated o URL directa).
type myApp struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Href        string `json:"href"`
	//nolint:tagliatelle // misma forma que SuiteApp del launcher (camelCase), consumido tal cual por el portal.
	IconURL string `json:"iconUrl"`
	Color   string `json:"color"`
	Status  string `json:"status"`
}

// MyApps — GET /api/v1/my-apps — las apps conectadas y ACTIVAS de la org como tiles de
// producto, para que el portal las muestre junto a las apps propias. Lectura ⇒ cualquier
// miembro (RLS). Sin N+1: una consulta de instancias + una del catálogo, unidas en
// memoria.
func (h *Handlers) MyApps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	instances, err := h.store.ListInstances(ctx)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	templates, err := h.store.ListTemplates(ctx)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}

	// Índice de manifiestos por clave de plantilla (parseo único; catálogo pequeño).
	byKey := make(map[string]*manifest.Manifest, len(templates))
	for _, t := range templates {
		m, err := manifest.Parse(t.Manifest)
		if err != nil {
			log.From(ctx).Warn("integrations: manifiesto de catálogo inválido, se omite del launcher", "template", t.Key, "error", err)
			continue
		}
		byKey[t.Key] = m
	}

	apps := make([]myApp, 0, len(instances))
	for _, inst := range instances {
		if inst.Status != domain.StatusActive {
			continue // solo las activas son lanzables como producto.
		}
		m, ok := byKey[inst.TemplateKey]
		if !ok {
			continue // plantilla retirada del catálogo: no hay cómo pintarla.
		}
		cfg := map[string]string{}
		if len(inst.Config) > 0 {
			_ = json.Unmarshal(inst.Config, &cfg)
		}
		href := cfg[manifest.KeyLaunchURL]
		if href == "" {
			continue // sin URL de lanzamiento resuelta no es un tile útil.
		}
		name := inst.Name
		if name == "" {
			name = m.Name
		}
		apps = append(apps, myApp{
			Key:         inst.ID,
			Name:        name,
			Description: m.Description,
			Href:        href,
			IconURL:     svgDataURI(m.Icon),
			Color:       m.Color,
			Status:      string(inst.Status),
		})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"apps": apps})
}

// svgDataURI empaqueta un SVG inline como data URI (base64) apto para <img src>. Base64
// evita cualquier problema de escapado (p. ej. '#' de los colores) en el atributo.
func svgDataURI(svg string) string {
	if svg == "" {
		return ""
	}
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}
