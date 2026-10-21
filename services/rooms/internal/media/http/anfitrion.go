// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package meethttp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
)

// La llamada de una conversación con personas de OTRA organización (ADR 0040)
// vive en la organización anfitriona, igual que la conversación. Estos dos
// middlewares llevan allí la petición de quien viene de fuera; lo que decide si
// puede es lo de siempre (requireRoomAccess pregunta a la mensajería por el canal).

type cambioDeOrganizacion func(r *http.Request, sala, canal uuid.UUID) (*http.Request, error)

// maxCuerpoOrigen acota lo que se lee del cuerpo para encontrar el canal: el de
// /rooms/from-source son tres campos.
const maxCuerpoOrigen = 1 << 16

// porSala: la sala viene en la ruta ({id}).
func porSala(cambiar cambioDeOrganizacion) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if cambiar == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, err := uuid.Parse(chi.URLParam(r, "id"))
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			if r, err = cambiar(r, id, uuid.Nil); err != nil {
				httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// porCanalDeOrigen: la sala aún no se conoce y lo que viene es su canal de origen,
// en la query (by-source) o en el cuerpo JSON (from-source). El cuerpo se lee y se
// DEVUELVE entero a la petición, para que el handler lo decodifique como siempre.
func porCanalDeOrigen(cambiar cambioDeOrganizacion) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if cambiar == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			kind, ref := r.URL.Query().Get("kind"), r.URL.Query().Get("ref_id")
			if r.Method == http.MethodPost && r.Body != nil {
				cuerpo, err := io.ReadAll(io.LimitReader(r.Body, maxCuerpoOrigen))
				if err != nil {
					httpx.Fail(w, r, httpx.ErrBadRequest)
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(cuerpo))
				var origen struct {
					Kind  string `json:"kind"`
					RefID string `json:"ref_id"`
				}
				_ = json.Unmarshal(cuerpo, &origen) // si no es JSON, el handler responde 400
				kind, ref = origen.Kind, origen.RefID
			}
			canal, err := uuid.Parse(ref)
			if domain.SourceKind(kind) != domain.SourceChat || err != nil {
				next.ServeHTTP(w, r)
				return
			}
			if r, err = cambiar(r, uuid.Nil, canal); err != nil {
				httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
