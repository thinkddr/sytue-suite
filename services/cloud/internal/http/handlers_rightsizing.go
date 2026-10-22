// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/service"
)

// sizingAdvisor es el seam del servicio, tomado por aserción para no tocar el
// fichero de handlers que lleva otra sesión.
type sizingAdvisor interface {
	ProjectSizingAdvice(ctx context.Context, id uuid.UUID) (domain.SizingAdvice, error)
	ProjectResourceMetrics(ctx context.Context, id uuid.UUID, window time.Duration) (service.ProjectMetrics, error)
}

var _ sizingAdvisor = (*service.Service)(nil)

// GetProjectSizing devuelve la recomendación de tamaño de un proyecto según su
// uso REAL.
//
// No hay un endpoint hermano de «aplicar»: aplicarla es cambiar el tamaño del
// proyecto, y eso ya lo hace PATCH /projects/{id}, que valida contra el catálogo,
// audita y respeta la cuota del plan. Un verbo nuevo duplicaría esas tres cosas y
// sería el único camino de cambio de tamaño que podría saltarse alguna. El «1
// clic» vive en la interfaz, no en la API.
//
// «Sin datos suficientes» se devuelve con 200 y su motivo, no con 404 ni con un
// cuerpo vacío: es la respuesta CORRECTA para un proyecto recién creado, y el
// panel tiene que poder decirlo con esas palabras.
func (h *Handlers) GetProjectSizing(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid project id."))
		return
	}
	adv, ok := h.svc.(sizingAdvisor)
	if !ok {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	out, err := adv.ProjectSizingAdvice(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

// GetProjectMetrics devuelve el consumo observado del proyecto (CPU y memoria) en
// la ventana pedida, junto al techo de su tamaño.
//
// Existe aparte de /sizing porque son dos preguntas distintas: el consejo dice
// «qué hago» y esto «qué está pasando». La segunda tiene respuesta aunque la
// primera sea «aún no he medido bastante», y hasta ahora las muestras solo las
// leía el motor de recomendación: el panel podía afirmar que sobraba tamaño sin
// poder enseñar el consumo del que salía esa conclusión.
//
// Serie VACÍA con 200, no 404: un proyecto recién creado —o un clúster sin
// metrics-server— no es un error, y la pantalla tiene que poder decirlo.
func (h *Handlers) GetProjectMetrics(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid project id."))
		return
	}
	adv, ok := h.svc.(sizingAdvisor)
	if !ok {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	// `hours` fuera de rango no es un error: se acota. Quien pide 10.000 horas
	// quiere «todo lo que tengas», y devolverle un 400 no le acerca a eso.
	hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	if hours <= 0 {
		hours = 24
	}
	out, err := adv.ProjectResourceMetrics(r.Context(), id, time.Duration(hours)*time.Hour)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, out)
}
