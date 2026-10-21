// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package casthttp

import (
	"net/http"
	"strconv"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
)

// Calidades: cuánto ocupará cada una antes de subir (web, escritorio, «Subir desde
// Drive») y las de un vídeo ya creado (Gestionar). La estimación sale SOLO de
// aquí (domain.Estimar), con las mismas tasas con las que codifica el
// transcodificador.

type opcionDTO struct {
	Quality     int   `json:"quality"`
	Allowed     bool  `json:"allowed"`
	Recommended bool  `json:"recommended"`
	Bytes       int64 `json:"bytes"`
}

type estimacionDTO struct {
	Options       []opcionDTO `json:"options"`
	Recommended   []int       `json:"recommended"`
	OriginalBytes int64       `json:"original_bytes"`
	DurationMS    int64       `json:"duration_ms"`
	// Approximate: la duración se dedujo del tamaño del original, no se midió.
	Approximate bool `json:"approximate"`
}

func estimacionDTOde(e domain.Estimacion) estimacionDTO {
	out := estimacionDTO{
		Options: make([]opcionDTO, 0, len(e.Opciones)), Recommended: e.Recomendadas,
		OriginalBytes: e.BytesOriginal, DurationMS: e.DuracionMS, Approximate: e.Aproximada,
	}
	if out.Recommended == nil {
		out.Recommended = []int{}
	}
	for _, o := range e.Opciones {
		out.Options = append(out.Options, opcionDTO{Quality: o.Lado, Allowed: o.Permitida, Recommended: o.Recomendada, Bytes: o.Bytes})
	}
	return out
}

// Estimar — GET /api/v1/videos:estimate?size_bytes=&duration_ms=&width=&height=.
// Solo size_bytes es obligatorio; lo demás lo pone quien lo sepa (el navegador
// lee duración y dimensiones del fichero antes de subirlo).
func (h *Handlers) Estimar(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	tam, err := entero(q.Get("size_bytes"), true)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("size_bytes must be a non-negative integer."))
		return
	}
	dur, err1 := entero(q.Get("duration_ms"), false)
	ancho, err2 := entero(q.Get("width"), false)
	alto, err3 := entero(q.Get("height"), false)
	if err1 != nil || err2 != nil || err3 != nil || ancho > 1<<16 || alto > 1<<16 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("duration_ms, width and height must be non-negative integers."))
		return
	}
	var duracion *int64
	if dur > 0 {
		duracion = &dur
	}
	httpx.JSON(w, r, http.StatusOK, estimacionDTOde(domain.Estimar(tam, duracion, int(ancho), int(alto))))
}

// entero lee un entero no negativo de la query; vacío es 0 salvo que sea obligatorio.
func entero(s string, obligatorio bool) (int64, error) {
	if s == "" && !obligatorio {
		return 0, nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, httpx.ErrBadRequest
	}
	return n, nil
}

type calidadesVideoDTO struct {
	estimacionDTO
	calidadesDTO
	// Pending: lo generado aún no es lo elegido (hay un trabajo por hacer o en curso).
	Pending bool `json:"pending"`
}

// Calidades — GET /api/v1/videos/{id}/qualities (quien lo edita).
func (h *Handlers) Calidades(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, e, err := h.svc.CalidadesDe(r.Context(), quienDe(r), id)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, calidadesVideoDTOde(v, e))
}

func calidadesVideoDTOde(v domain.Video, e domain.Estimacion) calidadesVideoDTO {
	return calidadesVideoDTO{estimacionDTO: estimacionDTOde(e), calidadesDTO: calidadesDTOde(v), Pending: service.PideTrabajoDeCalidades(v)}
}

// ElegirCalidades — PUT /api/v1/videos/{id}/qualities {qualities: [1080, 720]}
// → 202 con las calidades del vídeo. Lo que se añade se genera y lo que se quita
// se borra en un trabajo de la cola.
func (h *Handlers) ElegirCalidades(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		Qualities []int `json:"qualities"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, err := h.svc.ElegirCalidades(r.Context(), quienDe(r), id, in.Qualities)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusAccepted, calidadesVideoDTOde(v, service.EstimacionDe(v)))
}
