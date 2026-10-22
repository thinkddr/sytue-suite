// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package baseshttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/guiones"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// SCRIPTS DE USUARIO (bases-2, automatización; ADR 0069)
//
// El servidor NO ejecuta código: guarda versiones inmutables, decide quién puede ejecutar
// cuál, aplica las cuotas y deja el registro. Lo que el script hace lo hace el navegador
// de quien lo lanza, con SU sesión, contra las rutas de siempre de esta API. Todas las
// rutas cuelgan de la base y todo se decide en el store, dentro de su transacción.

// maxCuerpoScript es el código (64 KiB) con su manifiesto y el sobre.
const maxCuerpoScript = guiones.MaxCodigo + 32<<10

func (h *Handlers) montarScripts(r chi.Router) {
	j := r.With(httpx.MaxBodyBytes(maxCuerpoScript), h.enAnfitrion)
	j.Get("/script-quota", h.cuotaScripts)
	j.Get("/bases/{id}/scripts", h.listarScripts)
	j.Post("/bases/{id}/scripts", h.crearScript)
	j.Get("/bases/{id}/scripts/{sid}", h.verScript)
	j.Delete("/bases/{id}/scripts/{sid}", h.borrarScript)
	j.Put("/bases/{id}/scripts/{sid}/scope", h.ambitoScript)
	j.Post("/bases/{id}/scripts/{sid}/versions", h.nuevaVersionScript)
	j.Post("/bases/{id}/scripts/{sid}/versions/{v}/propose", h.proponerVersionScript)
	j.Post("/bases/{id}/scripts/{sid}/versions/{v}/review", h.revisarVersionScript)
	j.Post("/bases/{id}/scripts/{sid}/versions/{v}/runs", h.ejecutarScript)
	j.Get("/bases/{id}/script-runs", h.registroScripts)
	j.Put("/bases/{id}/script-runs/{run}", h.cerrarEjecucionScript)
}

// errorScript traduce los errores propios de los scripts y deja el resto a errorAPI.
func errorScript(err error) error {
	switch {
	case errors.Is(err, domain.ErrScriptSinAprobar):
		return httpx.NewError(http.StatusForbidden, "script_not_approved", err.Error())
	case errors.Is(err, domain.ErrCuotaScripts):
		return httpx.NewError(http.StatusTooManyRequests, "quota_exceeded", err.Error())
	case errors.Is(err, domain.ErrEstadoScript):
		return httpx.NewError(http.StatusConflict, "script_state", err.Error())
	case errors.Is(err, domain.ErrHashNoCoincide):
		return httpx.NewError(http.StatusConflict, "hash_mismatch", err.Error())
	}
	return errorAPI(err)
}

// deScript lee el actor y los ids de la ruta (`claves` además de la base).
func deScript(r *http.Request, claves ...string) (a domain.Actor, ids []string, err error) {
	a, err = actor(r)
	for _, k := range append([]string{"id"}, claves...) {
		v, e := id(r, k)
		err = firstErr(err, e)
		ids = append(ids, v)
	}
	return a, ids, err
}

func numeroDeVersion(r *http.Request) (int, error) {
	n, err := strconv.Atoi(chi.URLParam(r, "v"))
	if err != nil || n < 1 {
		return 0, httpx.ErrNotFound
	}
	return n, nil
}

// cuerpoVersion es {code, manifest}; validar devuelve el manifiesto y su forma canónica.
type cuerpoVersion struct {
	Code     string          `json:"code"`
	Manifest json.RawMessage `json:"manifest"`
}

func (c cuerpoVersion) validar() (domain.ManifiestoScript, []byte, error) {
	if !guiones.CodigoValido(c.Code) {
		return domain.ManifiestoScript{}, nil, domain.ErrInvalido
	}
	m, err := domain.ParseManifiesto(c.Manifest)
	if err != nil {
		return m, nil, err
	}
	canon, err := json.Marshal(m)
	return m, canon, err
}

func ambitoValido(s string) bool { return s == domain.ScriptPrivado || s == domain.ScriptCompartido }

func (h *Handlers) listarScripts(w http.ResponseWriter, r *http.Request) {
	a, ids, err := deScript(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	lista, revisa, err := h.Store.ListarScripts(r.Context(), a, ids[0])
	if err != nil {
		httpx.Fail(w, r, errorScript(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"scripts": lista, "can_review": revisa})
}

func (h *Handlers) crearScript(w http.ResponseWriter, r *http.Request) {
	a, ids, err := deScript(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		Name  string `json:"name"`
		Scope string `json:"scope"`
		cuerpoVersion
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.Scope == "" {
		in.Scope = domain.ScriptPrivado
	}
	nombre, err := domain.LimpiarNombre(in.Name)
	if err == nil && !ambitoValido(in.Scope) {
		err = domain.ErrInvalido
	}
	var m domain.ManifiestoScript
	var canon []byte
	if err == nil {
		m, canon, err = in.validar()
	}
	if err != nil {
		httpx.Fail(w, r, errorScript(err))
		return
	}
	sc, v, err := h.Store.CrearScript(r.Context(), a, ids[0], nombre, in.Scope, in.Code, m, canon)
	if err != nil {
		httpx.Fail(w, r, errorScript(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"script": sc, "version": v})
}

func (h *Handlers) verScript(w http.ResponseWriter, r *http.Request) {
	a, ids, err := deScript(r, "sid")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sc, vs, revisa, err := h.Store.VerScript(r.Context(), a, ids[0], ids[1])
	if err != nil {
		httpx.Fail(w, r, errorScript(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"script": sc, "versions": vs, "can_review": revisa})
}

func (h *Handlers) borrarScript(w http.ResponseWriter, r *http.Request) {
	a, ids, err := deScript(r, "sid")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.hechoScript(w, r, h.Store.BorrarScript(r.Context(), a, ids[0], ids[1]))
}

func (h *Handlers) hechoScript(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		httpx.Fail(w, r, errorScript(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) ambitoScript(w http.ResponseWriter, r *http.Request) {
	a, ids, err := deScript(r, "sid")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		Scope string `json:"scope"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !ambitoValido(in.Scope) {
		httpx.Fail(w, r, errorAPI(domain.ErrInvalido))
		return
	}
	h.hechoScript(w, r, h.Store.CambiarAmbitoScript(r.Context(), a, ids[0], ids[1], in.Scope))
}

func (h *Handlers) nuevaVersionScript(w http.ResponseWriter, r *http.Request) {
	a, ids, err := deScript(r, "sid")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in cuerpoVersion
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m, canon, err := in.validar()
	if err != nil {
		httpx.Fail(w, r, errorScript(err))
		return
	}
	v, err := h.Store.NuevaVersionScript(r.Context(), a, ids[0], ids[1], in.Code, m, canon)
	if err != nil {
		httpx.Fail(w, r, errorScript(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"version": v})
}

func (h *Handlers) proponerVersionScript(w http.ResponseWriter, r *http.Request) {
	a, ids, err := deScript(r, "sid")
	n, err2 := numeroDeVersion(r)
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.hechoScript(w, r, h.Store.ProponerVersionScript(r.Context(), a, ids[0], ids[1], n))
}

// revisarVersionScript — {decision: approve|reject, code_hash}: aprobar o rechazar exige
// el hash del código que se revisó.
func (h *Handlers) revisarVersionScript(w http.ResponseWriter, r *http.Request) {
	a, ids, err := deScript(r, "sid")
	n, err2 := numeroDeVersion(r)
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		Decision string `json:"decision"`
		CodeHash string `json:"code_hash"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if (in.Decision != "approve" && in.Decision != "reject") || !guiones.ReHash.MatchString(in.CodeHash) {
		httpx.Fail(w, r, errorAPI(domain.ErrInvalido))
		return
	}
	h.hechoScript(w, r, h.Store.RevisarVersionScript(r.Context(), a, ids[0], ids[1], n, in.CodeHash, in.Decision == "approve"))
}

// ejecutarScript autoriza una ejecución, reserva la cuota y devuelve el código y el
// manifiesto QUE SE AUTORIZARON (el cliente ejecuta exactamente eso) con el id de la
// ejecución, que cerrará con PUT …/script-runs/{run}.
func (h *Handlers) ejecutarScript(w http.ResponseWriter, r *http.Request) {
	a, ids, err := deScript(r, "sid")
	n, err2 := numeroDeVersion(r)
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	aut, err := h.Store.AutorizarEjecucion(r.Context(), a, ids[0], ids[1], n)
	if err != nil {
		httpx.Fail(w, r, errorScript(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, aut)
}

// cerrarEjecucionScript — {outcome, error_code, duration_ms, ops}. Lo informa el cliente
// (el servidor no ve el código correr): es diagnóstico, no prueba. Solo quien abrió la
// ejecución, una vez, y con valores acotados.
func (h *Handlers) cerrarEjecucionScript(w http.ResponseWriter, r *http.Request) {
	a, ids, err := deScript(r)
	run, err2 := strconv.ParseInt(chi.URLParam(r, "run"), 10, 64)
	if err2 != nil || run < 1 {
		err = firstErr(err, httpx.ErrNotFound)
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		Outcome    string `json:"outcome"`
		ErrorCode  string `json:"error_code"`
		DurationMs int    `json:"duration_ms"`
		Ops        int    `json:"ops"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !guiones.ResultadoFinal(in.Outcome) || !guiones.ReCodigoError.MatchString(in.ErrorCode) ||
		in.DurationMs < 0 || in.DurationMs > 2*guiones.TopeMs || in.Ops < 0 || in.Ops > 2*domain.TopeOpsScript {
		httpx.Fail(w, r, errorAPI(domain.ErrInvalido))
		return
	}
	h.hechoScript(w, r, h.Store.CerrarEjecucion(r.Context(), a, ids[0], run, in.Outcome, in.ErrorCode, in.DurationMs, in.Ops))
}

// registroScripts — GET …/script-runs?before=&limit=: lo ejecutado en la base.
func (h *Handlers) registroScripts(w http.ResponseWriter, r *http.Request) {
	a, ids, err := deScript(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	antes, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	limite, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limite < 1 {
		limite = 50
	}
	limite = min(limite, domain.MaxPaginaRuns)
	es, err := h.Store.EjecucionesDeScripts(r.Context(), a, ids[0], max(antes, 0), limite)
	if err != nil {
		httpx.Fail(w, r, errorScript(err))
		return
	}
	sig := int64(0)
	if len(es) == limite {
		sig = es[len(es)-1].ID
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"runs": es, "next_before": sig})
}

// cuotaScripts — GET /script-quota: lo consumido en la última hora y los topes, que son fijos.
func (h *Handlers) cuotaScripts(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	uso, err := h.Store.UsoCuotaScripts(r.Context(), a)
	if err != nil {
		httpx.Fail(w, r, errorScript(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"used":   map[string]any{"user_runs": uso.EjecucionesUsuario, "org_runs": uso.EjecucionesOrg, "user_ms": uso.MsUsuario},
		"limits": map[string]any{"user_runs": guiones.CuotaEjecucionesUsuarioHora, "org_runs": guiones.CuotaEjecucionesOrgHora, "user_ms": guiones.CuotaMsUsuarioHora},
	})
}
