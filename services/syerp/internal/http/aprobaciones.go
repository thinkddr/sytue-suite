// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
	"git.sytue.eu/sytue/one/services/syerp/internal/permisos"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// APROBACIONES: a partir de cierto importe, una segunda persona.
//
// Los verbos (0092) separan quién puede asentar. Esto añade lo que un verbo no
// puede decir: que una factura de cincuenta mil euros no la mete una sola
// persona en el libro aunque tenga el verbo.

var (
	errRequiereAprobacion = httpx.NewError(http.StatusConflict, "requiere_aprobacion",
		"This amount is over the approval threshold for its module: ask someone else to approve it first.")
	errCuatroOjos = httpx.NewError(http.StatusForbidden, "cuatro_ojos",
		"You cannot approve a document you created or whose approval you requested.")
	errAprobacionResuelta = httpx.NewError(http.StatusConflict, "aprobacion_resuelta",
		"That approval has already been resolved.")
)

// Umbrales — GET /api/v1/admin/umbrales — los umbrales por módulo.
func (h *Handlers) Umbrales(w http.ResponseWriter, r *http.Request) {
	us, err := h.store.Umbrales(r.Context())
	if err != nil {
		httpx.Fail(w, r, errorDeLectura(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"umbrales": us,
		// La divisa de la organización, para que la pantalla no tenga que
		// adivinar en qué moneda se escribe un umbral nuevo.
		"moneda_por_defecto": domain.MonedaPorDefecto,
		"modulos":            h.modulosConRotulo(httpx.LanguageFromRequest(r)),
	})
}

// GuardarUmbral — PUT /api/v1/admin/umbrales/{modulo}.
func (h *Handlers) GuardarUmbral(w http.ResponseWriter, r *http.Request) {
	var c struct {
		// En CÉNTIMOS enteros. Negativo = quitar el umbral.
		Importe *int64 `json:"importe"`
		Moneda  string `json:"moneda"`
	}
	if err := httpx.DecodeJSON(w, r, &c); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	modulo := chi.URLParam(r, "modulo")
	v := validation.New()
	if h.moduloDesconocido(modulo) {
		v.Add("modulo", "módulo desconocido "+modulo)
	}
	if c.Importe == nil {
		v.Add("importe", "obligatorio (en céntimos enteros; negativo para quitarlo)")
	}
	moneda := strings.ToUpper(strings.TrimSpace(c.Moneda))
	if moneda == "" {
		moneda = domain.MonedaPorDefecto
	}
	if len(moneda) != 3 {
		// Un importe sin divisa es un número suelto, y comparar números sueltos
		// de monedas distintas es el error que este campo existe para evitar.
		v.Add("moneda", "código ISO de tres letras")
	}
	if err := v.Err(); err != nil {
		httpx.Fail(w, r, errorDeValidacion(err))
		return
	}
	if err := h.store.GuardarUmbral(r.Context(), modulo, *c.Importe, moneda, sub(r)); err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) moduloDesconocido(modulo string) bool {
	for _, m := range h.cat.Modulos() {
		if m.Clave == modulo {
			return false
		}
	}
	return true
}

// Aprobaciones — GET /api/v1/aprobaciones?estado=pendiente.
func (h *Handlers) Aprobaciones(w http.ResponseWriter, r *http.Request) {
	estado := r.URL.Query().Get("estado")
	switch estado {
	case "", "pendiente", "aprobada", "rechazada":
	default:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("estado: pendiente|aprobada|rechazada"))
		return
	}
	as, err := h.store.Aprobaciones(r.Context(), estado)
	if err != nil {
		httpx.Fail(w, r, errorDeLectura(err))
		return
	}
	// La cola enseña importes: los de nóminas solo a quien puede leer nóminas
	// (NEG-17).
	visibles := make([]store.Aprobacion, 0, len(as))
	for _, a := range as {
		if h.accesoRegistro(r.Context(), a.Modelo, permisos.Lectura) {
			visibles = append(visibles, a)
		}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"aprobaciones": visibles})
}

// PedirAprobacion — POST /api/v1/{modelo}/{id}/aprobacion.
//
// El importe NO viene del cliente: se calcula del documento, que es el mismo
// que iría al libro. Un importe de fuera sería la forma más fácil de pasar por
// debajo del umbral.
func (h *Handlers) PedirAprobacion(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	if m.Contable == nil {
		httpx.Fail(w, r, errNoContable)
		return
	}
	id := chi.URLParam(r, "id")
	reg, err := h.store.Obtener(r.Context(), m.Clave, id)
	if err != nil {
		httpx.Fail(w, r, errorDeLectura(err))
		return
	}
	etiqueta := reg.Etiqueta
	if etiqueta == "" {
		etiqueta = reg.Ref
	}
	a, err := domain.AsientoDe(m, etiqueta, reg.Data)
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	sol, err := h.store.PedirAprobacion(r.Context(), m, id, store.ImporteDelAsiento(a), a.Moneda, sub(r))
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	h.avisarAprobadores(r.Context(), sol, etiqueta)
	httpx.JSON(w, r, http.StatusCreated, sol)
}

// moduloAprobador es el módulo cuyo verbo `aprobar` exige la ruta de resolver
// (exigirVerboContable): avisar con otro criterio mandaría el aviso a quien
// luego no puede pulsar el botón.
const moduloAprobador = "contabilidad"

// avisarAprobadores deja en la campana de cada persona que puede aprobar la
// solicitud recién abierta. Ni a quien la pide ni a quien creó el documento: los
// cuatro ojos les impiden aprobarla, y un aviso que no pueden atender es ruido.
// Si el Hub no responde, la solicitud sigue en la cola igual.
func (h *Handlers) avisarAprobadores(ctx context.Context, a store.Aprobacion, etiqueta string) {
	if h.hub == nil {
		return
	}
	cs, err := h.store.CandidatosAprobacion(ctx, moduloAprobador)
	if err != nil {
		return
	}
	t, _ := tenancy.From(ctx)
	actor := ""
	if c, ok := authn.FromContext(ctx); ok {
		actor = cmp.Or(strings.TrimSpace(c.Name), c.Email, c.Subject)
	}
	for _, sub := range aprobadores(cs, moduloAprobador, a.SolicitadaPor, a.CreadaPor) {
		h.hub.SendAsync(ctx, avisoDeAprobacion(h.cfg.PublicURL, t.Slug, sub, actor, etiqueta, a))
	}
}

// aprobadores filtra, con las reglas de la ruta de aprobar, a quien puede
// resolver una solicitud y no queda fuera por los cuatro ojos.
func aprobadores(cs []store.Candidato, modulo string, excluir ...string) []string {
	var out []string
	for _, c := range cs {
		if c.Sub == "" || slices.Contains(excluir, c.Sub) {
			continue
		}
		puestos := map[string]string{}
		if c.Nivel != "" {
			puestos[modulo] = c.Nivel
		}
		listas := map[string][]string{}
		if c.Verbos != nil {
			listas[modulo] = *c.Verbos
		}
		if permisos.PuedeVerbo(c.Roles, puestos, listas, modulo, permisos.Aprobar) {
			out = append(out, c.Sub)
		}
	}
	return out
}

// avisoDeAprobacion es el aviso a una persona que puede aprobar: quién lo pide,
// qué documento y por cuánto, y el enlace abre la cola de aprobaciones.
func avisoDeAprobacion(publicURL, tenant, para, actor, etiqueta string, a store.Aprobacion) hubnotify.Notification {
	importe := strconv.FormatFloat(float64(a.Importe)/100, 'f', 2, 64) + " " + a.Moneda
	return hubnotify.Notification{
		Tenant:  tenant,
		Subject: para,
		Source:  "erp",
		Type:    "erp.approval.requested",
		Level:   "info",
		Title:   actor + " asks you to approve “" + etiqueta + "”",
		Body:    importe,
		URL:     hubnotify.EnlaceAprobacionesERP(publicURL, tenant),
		Params:  map[string]string{"actor": actor, "name": etiqueta, "resource": a.ID},
	}
}

// ResolverAprobacion — POST /api/v1/aprobaciones/{id}/{aprobar|rechazar}.
func (h *Handlers) ResolverAprobacion(estado string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var c struct {
			Motivo string `json:"motivo"`
		}
		if err := httpx.DecodeJSON(w, r, &c); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		motivo := validation.TruncateRunes(strings.TrimSpace(c.Motivo), 1000)
		if estado == "rechazada" && motivo == "" {
			// Rechazar sin decir por qué deja a quien lo pidió sin saber qué
			// arreglar, y el documento vuelve igual mañana.
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("motivo: say why you are rejecting it."))
			return
		}
		a, err := h.store.ResolverAprobacion(r.Context(), chi.URLParam(r, "id"), estado, motivo, sub(r))
		switch {
		case errors.Is(err, store.ErrCuatroOjos):
			httpx.Fail(w, r, errCuatroOjos)
		case errors.Is(err, store.ErrAprobacionResuelta):
			httpx.Fail(w, r, errAprobacionResuelta)
		case err != nil:
			httpx.Fail(w, r, errorDeEscritura(err))
		default:
			httpx.JSON(w, r, http.StatusOK, a)
		}
	}
}
