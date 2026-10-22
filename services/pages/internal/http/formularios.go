// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
	"git.sytue.eu/sytue/one/services/pages/internal/tienda"
)

// montarFormularios registra la bandeja de formularios del editor (§6.3): los
// formularios de la web, sus respuestas, su CSV y a quién avisan. El envío desde las
// webs va en tienda_publico.go.
//
// Todo es de quien administra la organización, como los pedidos (WEB-02): las
// respuestas son datos personales de quien escribe en la web, y a quién se avisa es
// adónde salen esos avisos.
func montarFormularios(g grupos, h *Handlers) {
	g.api.Route("/sites/{id}/forms", func(r chi.Router) {
		r.Use(h.Verifier.RequireRole("owner", "admin"))
		r.Get("/", h.listarFormularios)
		r.Put("/{formId}/notify", h.guardarAvisosFormulario)
		r.Get("/{formId}/submissions", h.listarEnvios)
		r.Get("/{formId}/submissions.csv", h.exportarEnviosCSV)
	})
}

func (h *Handlers) listarFormularios(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, _ := tenancy.From(r.Context())
	forms, err := h.Tienda.Formularios(r.Context(), t.Slug, siteID)
	if err != nil {
		httpx.Fail(w, r, errorFormulario(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"forms": forms})
}

func (h *Handlers) guardarAvisosFormulario(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		Notify []string `json:"notify"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_json").Wrap(err))
		return
	}
	t, _ := tenancy.From(r.Context())
	if err := h.Tienda.GuardarAvisosFormulario(r.Context(), t.Slug, siteID, chi.URLParam(r, "formId"), in.Notify); err != nil {
		httpx.Fail(w, r, errorFormulario(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handlers) listarEnvios(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, _ := tenancy.From(r.Context())
	envios, err := h.Tienda.EnviosFormulario(r.Context(), t.Slug, siteID, chi.URLParam(r, "formId"))
	if err != nil {
		httpx.Fail(w, r, errorFormulario(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"submissions": envios})
}

// exportarEnviosCSV entrega las respuestas de un formulario en CSV UTF-8, con los
// rótulos de los campos en el idioma de `?lang=`.
func (h *Handlers) exportarEnviosCSV(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	formID := chi.URLParam(r, "formId")
	t, _ := tenancy.From(r.Context())
	envios, err := h.Tienda.EnviosFormulario(r.Context(), t.Slug, siteID, formID)
	if err != nil {
		httpx.Fail(w, r, errorFormulario(err))
		return
	}
	forms, err := h.Tienda.Formularios(r.Context(), t.Slug, siteID)
	if err != nil {
		httpx.Fail(w, r, errorFormulario(err))
		return
	}
	var campos []ir.CampoFormulario
	for _, f := range forms {
		if f.FormID == formID {
			campos = f.Fields
		}
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=form-"+formID+".csv")
	w.Header().Set("Cache-Control", "no-store")
	_ = csv.NewWriter(w).WriteAll(filasEnviosCSV(campos, envios, r.URL.Query().Get("lang")))
}

// filasEnviosCSV son la cabecera y una fila por respuesta: fecha, idioma, los campos
// del formulario en vivo en su orden y, después, los que solo traen respuestas de
// versiones anteriores, por nombre.
//
// Todo lo que viene de la respuesta, y los rótulos, pasa por validation.CSVSafe: lo
// escribe una persona sin sesión en la web, y quien abre el fichero es un
// administrador, en una hoja de cálculo, que ejecuta como fórmula una celda que
// empiece por = + - o @. La fecha la formatea este código.
func filasEnviosCSV(campos []ir.CampoFormulario, envios []domain.FormSubmission, idioma string) [][]string {
	s := validation.CSVSafe
	conocidos := make(map[string]bool, len(campos))
	for _, c := range campos {
		conocidos[c.ID] = true
	}
	var extra []string
	for _, e := range envios {
		for k := range e.Data {
			if !conocidos[k] {
				conocidos[k] = true
				extra = append(extra, k)
			}
		}
	}
	sort.Strings(extra)

	ids := make([]string, 0, len(campos)+len(extra))
	cabecera := make([]string, 0, 2+len(campos)+len(extra))
	cabecera = append(cabecera, "created_at", "language")
	for _, c := range campos {
		ids = append(ids, c.ID)
		cabecera = append(cabecera, s(tienda.EtiquetaCampo(c, idioma)))
	}
	for _, k := range extra {
		ids = append(ids, k)
		cabecera = append(cabecera, s(k))
	}
	filas := make([][]string, 0, 1+len(envios))
	filas = append(filas, cabecera)
	for _, e := range envios {
		fila := make([]string, 0, 2+len(ids))
		fila = append(fila, e.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"), s(e.Language))
		for _, id := range ids {
			fila = append(fila, s(valorCSV(e.Data[id])))
		}
		filas = append(filas, fila)
	}
	return filas
}

func valorCSV(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprint(x)
	}
}

func errorFormulario(err error) error {
	switch {
	case errors.Is(err, domain.ErrFormularioNoExiste):
		return httpx.ErrNotFound.WithCode("form_not_found").Wrap(err)
	case errors.Is(err, domain.ErrAvisadosInvalidos):
		return httpx.NewError(http.StatusUnprocessableEntity, "invalid_notify", "pages: una dirección no es válida o hay más de 10")
	}
	return errorAPI(err)
}
