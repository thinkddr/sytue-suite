// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/syerp/internal/ia"
)

// personaEquipo es quien puede llevar una tarea: el selector de responsable.
type personaEquipo struct {
	Sub    string `json:"sub"`
	Nombre string `json:"nombre"`
	Correo string `json:"correo"`
}

// Equipo — GET /api/v1/equipo: la gente de la organización (el directorio de Sytue
// ID, con el token de quien pide) para el selector de «Responsable» de una tarea. Es
// lo que deja asignar una tarea a otra persona desde la web y desde Fyne, y con ello
// que le llegue el aviso.
func (h *Handlers) Equipo(w http.ResponseWriter, r *http.Request) {
	out, err := h.equipo(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// `yo` es quien pregunta: el selector lo pone el primero como «Yo (su nombre)» y no
	// lo repite entre los demás.
	httpx.JSON(w, r, http.StatusOK, map[string]any{"personas": out, "yo": sub(r)})
}

// equipo es el directorio de la organización visto con el token de quien pide: la lista
// del selector. El responsable de una tarea se valida aparte, uno a uno (CrearActividad).
func (h *Handlers) equipo(r *http.Request) ([]personaEquipo, error) {
	cuerpo, codigo := pedirListado(r.Context(), strings.TrimRight(h.cfg.IDBaseURL, "/")+"/api/v1/directory?limit=500", tokenDe(r))
	switch codigo {
	case "":
	case ia.ErrorSinPermiso:
		return nil, httpx.ErrForbidden
	default:
		return nil, httpx.ErrUnavailable
	}
	lista, _ := cuerpo["items"].([]any)
	out := make([]personaEquipo, 0, len(lista))
	for _, x := range lista {
		o, _ := x.(map[string]any)
		p := personaEquipo{Sub: textoDeJSON(o["user_id"]), Nombre: textoDeJSON(o["name"]), Correo: textoDeJSON(o["email"])}
		if p.Sub != "" {
			out = append(out, p)
		}
	}
	return out, nil
}
