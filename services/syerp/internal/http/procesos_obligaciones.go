// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package syerphttp

import (
	"net/http"
	"slices"
	"strconv"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/syerp/internal/obligaciones"
	"git.sytue.eu/sytue/one/services/syerp/internal/permisos"
)

func (h *Handlers) ProcesoObligacion(w http.ResponseWriter, r *http.Request) {
	instancia, desde, hasta, ok := h.instanciaProceso(w, r)
	if !ok {
		return
	}
	clave, periodo := instancia.Clave, instancia.Periodo
	if r.Method == http.MethodGet {
		out, err := h.store.ProcesoObligacion(r.Context(), clave, periodo)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		httpx.JSON(w, r, http.StatusOK, out)
		return
	}
	if !h.puede(r.Context(), permisos.Obligaciones, permisos.Usuario) {
		httpx.Fail(w, r, errSinPermiso)
		return
	}
	var in struct {
		Fase       int    `json:"fase"`
		Referencia string `json:"referencia"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.Fase >= 1 && in.Fase <= 4 {
		pendientes, err := h.store.Pendientes(r.Context(), desde, hasta)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		for _, n := range h.vistas([]obligaciones.Instancia{*instancia}, httpx.LanguageFromRequest(r), pendientes)[0].Pendientes {
			if n > 0 {
				httpx.Fail(w, r, httpx.ErrUnprocessable.WithMessage("resuelve los pendientes antes de continuar"))
				return
			}
		}
	}
	docs := make([]string, 0, len(instancia.Documentos))
	for _, d := range instancia.Documentos {
		docs = append(docs, d.Clave)
	}
	out, err := h.store.AvanzarProceso(r.Context(), clave, periodo, in.Fase, in.Referencia, docs, sub(r))
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

// PendientesProcesoObligacion devuelve solo las facturas del período que se
// puede leer; preparar obligaciones no concede acceso a otros módulos.
func (h *Handlers) PendientesProcesoObligacion(w http.ResponseWriter, r *http.Request) {
	instancia, desde, hasta, ok := h.instanciaProceso(w, r)
	if !ok {
		return
	}
	if !slices.Contains(instancia.Preparacion, "facturas") {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	modelos := make([]string, 0, 2)
	for _, modelo := range []string{"factura_venta", "factura_compra"} {
		if h.accesoRegistro(r.Context(), modelo, permisos.Lectura) {
			modelos = append(modelos, modelo)
		}
	}
	if len(modelos) == 0 {
		httpx.Fail(w, r, errSinPermiso)
		return
	}
	limite := 30
	if s := r.URL.Query().Get("limite"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 100 {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
		limite = n
	}
	pag, err := h.store.FacturasPendientesProceso(r.Context(), desde, hasta, modelos, r.URL.Query().Get("cursor"), limite)
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, pag)
}

func (h *Handlers) instanciaProceso(w http.ResponseWriter, r *http.Request) (*obligaciones.Instancia, time.Time, time.Time, bool) {
	_, perfil, ok := h.perfil(w, r)
	if !ok {
		return nil, time.Time{}, time.Time{}, false
	}
	clave, periodo := r.URL.Query().Get("clave"), r.URL.Query().Get("periodo")
	desde, hasta, ok := obligaciones.RangoDePeriodo(periodo)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return nil, time.Time{}, time.Time{}, false
	}
	inst, err := h.obligaciones.Calendario(perfil, desde.Year())
	if err != nil {
		httpx.Fail(w, r, err)
		return nil, time.Time{}, time.Time{}, false
	}
	for i := range inst {
		if inst[i].Clave == clave && inst[i].Periodo == periodo {
			return &inst[i], desde, hasta, true
		}
	}
	httpx.Fail(w, r, httpx.ErrNotFound)
	return nil, time.Time{}, time.Time{}, false
}
