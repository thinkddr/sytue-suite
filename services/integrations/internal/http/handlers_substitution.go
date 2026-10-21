// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
)

// LIBERTAD DE ELECCIÓN POR PRODUCTO
//
// La suite deja que cada organización decida QUIÉN sirve cada producto: el nativo de
// Sytue o una aplicación conectada. Google Sheets en vez de Sytue Sheets, Teams en vez de
// Rooms, Azure en vez de Cloud.
//
// Qué significa hoy, dicho sin adornos: el lanzador y el conmutador de aplicaciones abren
// el sustituto en lugar del producto Sytue. El handoff por documento —«abre ESTE fichero
// allí»— solo se ofrece cuando el proveedor publica una URL para ello (`document_url` del
// manifiesto), y la federación del dato (búsqueda del Hub, adjuntos, notificaciones) es
// otra capa. La UI tiene que contar esa diferencia, no dejar que el usuario la deduzca.

// substitutionOption es un candidato a sustituir un producto: una conexión ACTIVA cuya
// plantilla declara que puede hacerlo.
type substitutionOption struct {
	InstanceID string `json:"instance_id"`
	Name       string `json:"name"`
	//nolint:tagliatelle // misma forma que los tiles del launcher, consumido tal cual.
	IconURL string `json:"iconUrl"`
	Color   string `json:"color"`
	// OpenURL es adónde llevaría el tile si se eligiera esta opción.
	OpenURL string `json:"open_url"`
	// HasDocumentHandoff dice si además sabe abrir UN documento concreto. La UI lo usa
	// para no prometer un «Abrir en…» que llevaría a la portada del tercero.
	HasDocumentHandoff bool `json:"has_document_handoff"`
}

// productSubstitution es el estado de un producto: quién lo sirve y quién podría.
type productSubstitution struct {
	Product string `json:"product"`
	// Current es la conexión que lo sirve; vacío ⇒ lo sirve el producto Sytue.
	Current string               `json:"current_instance_id,omitempty"`
	Options []substitutionOption `json:"options"`
}

// ListSubstitutions — GET /api/v1/substitutions — el estado completo de la libertad de
// elección: por cada producto que ALGUNA conexión activa puede servir, quién lo sirve hoy
// y qué alternativas hay. Lectura ⇒ cualquier miembro (RLS).
//
// Solo aparecen productos con al menos una alternativa real: enumerar los quince
// productos de la suite con la lista vacía sería una pantalla de ruido donde el
// administrador tendría que buscar las tres filas accionables.
func (h *Handlers) ListSubstitutions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	byProduct, err := h.substitutionOptions(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	current, err := h.store.ListSubstitutions(ctx)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	chosen := make(map[string]string, len(current))
	for _, c := range current {
		chosen[c.Product] = c.InstanceID
	}

	out := make([]productSubstitution, 0, len(byProduct))
	for _, product := range manifest.SubstitutableProducts() {
		opts := byProduct[product]
		if len(opts) == 0 {
			continue
		}
		out = append(out, productSubstitution{Product: product, Current: chosen[product], Options: opts})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"products": out})
}

// setSubstitutionReq: instance_id vacío devuelve el producto al nativo de Sytue. Se usa
// el MISMO endpoint para elegir y para volver atrás porque para quien lo usa es la misma
// decisión —«quién sirve esto»— y separarlo en dos rutas obligaría a la UI a elegir
// verbo según el valor.
type setSubstitutionReq struct {
	InstanceID string `json:"instance_id"`
}

// SetSubstitution — PUT /api/v1/substitutions/{product} — elige quién sirve un producto.
// Escritura ⇒ owner/admin (router): cambia lo que ve TODA la organización al abrir el
// lanzador, no una preferencia personal.
func (h *Handlers) SetSubstitution(w http.ResponseWriter, r *http.Request) {
	product := strings.TrimSpace(chi.URLParam(r, "product"))
	if !manifest.IsSubstitutableProduct(product) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("That suite product does not exist."))
		return
	}
	var req setSubstitutionReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()

	if strings.TrimSpace(req.InstanceID) == "" {
		if err := h.store.ClearSubstitution(ctx, product); err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
		httpx.JSON(w, r, http.StatusOK, map[string]any{"product": product, "current_instance_id": ""})
		return
	}

	// La conexión tiene que DECLARAR que puede servir ese producto. Sin esto, la API
	// dejaría poner Slack como sustituto de Sytue Mail y el lanzador llevaría al correo
	// a un sitio donde no hay correo.
	ok, err := h.instanceSubstitutes(ctx, req.InstanceID, product)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"That app cannot substitute for this product.",
		))
		return
	}
	if err := h.store.SetSubstitution(ctx, product, req.InstanceID); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"product": product, "current_instance_id": req.InstanceID})
}

// ResolvedSubstitutions — GET /api/v1/substitutions/resolved — el mapa que consume el
// SHELL: producto → adónde abrirlo. Solo lleva lo sustituido; lo que no aparece lo sirve
// Sytue, así que el front no necesita conocer el catálogo completo para resolver un tile.
//
// Endpoint aparte del de administración a propósito: éste lo pide CADA carga de la shell
// de cada usuario, así que devuelve lo mínimo y no monta la lista de alternativas.
func (h *Handlers) ResolvedSubstitutions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	subs, err := h.store.ListSubstitutions(ctx)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	resolved := make(map[string]map[string]string, len(subs))
	for _, sub := range subs {
		inst, m, cfg, err := h.loadInstanceManifest(ctx, sub.InstanceID)
		if err != nil || inst.Status != domain.StatusActive {
			// Una sustitución que apunta a una conexión desactivada NO se sirve: el
			// producto vuelve al nativo en silencio, que es lo único que deja a la
			// persona con una aplicación que funciona.
			continue
		}
		decl := m.SubstitutionFor(sub.Product)
		if decl == nil {
			continue
		}
		href := h.substituteHref(decl, cfg)
		if href == "" {
			continue
		}
		entry := map[string]string{"instance_id": inst.ID, "name": inst.Name, "href": href}
		if decl.DocumentURL != "" {
			if tmpl, err := manifest.RenderURLKeeping(decl.DocumentURL, cfg, "document_id"); err == nil {
				entry["document_href"] = tmpl
			}
		}
		resolved[sub.Product] = entry
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"products": resolved})
}

// substitutionOptions recorre las conexiones ACTIVAS y agrupa por producto las que
// declaran poder sustituirlo. Una consulta de instancias y una de plantillas por clave
// (cacheada por el store), sin N+1 sobre el catálogo.
func (h *Handlers) substitutionOptions(ctx context.Context) (map[string][]substitutionOption, error) {
	insts, err := h.store.ListInstances(ctx)
	if err != nil {
		return nil, toAPIError(err)
	}
	out := map[string][]substitutionOption{}
	for i := range insts {
		inst := &insts[i]
		if inst.Status != domain.StatusActive {
			continue
		}
		_, m, cfg, err := h.loadInstanceManifest(ctx, inst.ID)
		if err != nil {
			continue
		}
		for j := range m.Substitutes {
			decl := &m.Substitutes[j]
			href := h.substituteHref(decl, cfg)
			if href == "" {
				continue // sin adónde abrir, no es una alternativa ofrecible
			}
			out[decl.Product] = append(out[decl.Product], substitutionOption{
				InstanceID: inst.ID, Name: inst.Name, IconURL: m.Icon, Color: m.Color,
				OpenURL: href, HasDocumentHandoff: decl.DocumentURL != "",
			})
		}
	}
	return out, nil
}

// instanceSubstitutes comprueba que una conexión declara poder servir un producto.
func (h *Handlers) instanceSubstitutes(ctx context.Context, instanceID, product string) (bool, error) {
	_, m, _, err := h.loadInstanceManifest(ctx, instanceID)
	if err != nil {
		return false, err
	}
	return m.SubstitutionFor(product) != nil, nil
}

// substituteHref resuelve adónde abrir el sustituto: la URL declarada para ese producto
// o, si no la hay, la URL de lanzamiento que la conexión ya resolvió al activarse.
func (h *Handlers) substituteHref(decl *manifest.Substitution, cfg map[string]string) string {
	if decl.OpenURL != "" {
		if u, err := manifest.RenderURL(decl.OpenURL, cfg); err == nil {
			return u
		}
		return ""
	}
	return cfg[manifest.KeyLaunchURL]
}
