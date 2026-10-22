// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package baseshttp

import (
	"cmp"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/bases/internal/domain"
	"git.sytue.eu/sytue/one/services/bases/internal/store"
)

// COMENTARIOS POR REGISTRO
//
// Un hilo de un nivel por registro. Leer es de quien lee el registro; comentar, del
// comentarista hacia arriba; borrar, de quien lo escribió o de quien modera (editor o
// propietario). Todo eso lo decide el store dentro de su transacción; aquí solo se
// traduce HTTP, se ponen los nombres y se AVISA por el Hub.
//
// Los avisos nunca enseñan un comentario a quien no puede leer el registro: los
// destinatarios del hilo los filtra el store en la misma transacción, y una mención
// (`@ana@empresa.es`, hubnotify.Mencionados) solo avisa si esa dirección es de alguien
// de la organización que tiene acceso al registro.

func (h *Handlers) listarComentarios(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	rid, err2 := id(r, "rid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cs, err := h.Store.ListarComentarios(r.Context(), a, rid)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	h.conNombres(r, cs)
	httpx.JSON(w, r, http.StatusOK, cs)
}

// conNombres pone a cada comentario el nombre de su autor, del directorio de Sytue ID
// con la sesión de quien lee (la verja es su organización). Quien no se resuelve se
// queda sin nombre.
//
// ponytail: una petición por autor distinto y por listado (≤ 500 comentarios, pocos
// autores); un lote en directory:resolve si los hilos crecen.
func (h *Handlers) conNombres(r *http.Request, cs []domain.Comentario) {
	token := authn.Token(r)
	if h.Cfg.IDBaseURL == "" || token == "" {
		return
	}
	nombres := map[string]string{"": ""}
	for i := range cs {
		sub := cs[i].AuthorSub
		if _, visto := nombres[sub]; !visto && len(nombres) <= 50 {
			correo, nombre, _ := peticiones.Persona(r.Context(), h.Cfg.IDBaseURL, token, sub)
			nombres[sub] = cmp.Or(nombre, correo)
		}
		cs[i].AuthorName = nombres[sub]
	}
}

func (h *Handlers) crearComentario(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	rid, err2 := id(r, "rid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		ParentID string `json:"parent_id"`
		Body     string `json:"body"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.ParentID != "" {
		if in.ParentID, err = idDe(in.ParentID); err != nil {
			httpx.Fail(w, r, errorAPI(domain.ErrInvalido))
			return
		}
	}
	c, av, err := h.Store.CrearComentario(r.Context(), a, rid, in.ParentID, in.Body)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	h.avisarComentario(r, a, c, av)
	uno := []domain.Comentario{c}
	h.conNombres(r, uno)
	httpx.JSON(w, r, http.StatusCreated, uno[0])
}

func (h *Handlers) borrarComentario(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	cid, err2 := id(r, "coid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.hecho(w, r, h.Store.BorrarComentario(r.Context(), a, cid))
}

// avisarComentario deja el comentario en la campana de la propietaria de la base y de
// quienes escriben en el hilo (ya filtrados por el store), y la mención en la de cada
// persona mencionada que puede leer el registro. El comentario ya está guardado: aquí
// nada falla hacia fuera.
func (h *Handlers) avisarComentario(r *http.Request, a domain.Actor, c domain.Comentario, av store.AvisoComentario) {
	if h.Hub == nil {
		return
	}
	ctx := r.Context()
	cl, _ := authn.FromContext(ctx)
	aviso := func(para, tipo, titulo string) hubnotify.Notification {
		nombre := cmp.Or(av.BaseName, "Sytue Bases")
		quien := cmp.Or(strings.TrimSpace(cl.Name), cl.Email)
		params := map[string]string{"name": nombre}
		if quien != "" {
			params["actor"] = quien
		}
		return hubnotify.Notification{
			Tenant: a.Tenant, Subject: para, Source: "bases", Type: tipo, Level: "info",
			Title:  titulo + " «" + nombre + "»",
			Body:   validation.TruncateRunes(c.Body, 300),
			URL:    hubnotify.EnlaceRegistroDeBase(h.Cfg.PublicURL, a.Tenant, av.BaseID, av.TableID, c.RecordID),
			Params: params,
		}
	}
	avisados := map[string]bool{a.Sub: true}
	// Las menciones primero: a quien se menciona le llega la mención, no además el comentario.
	if token := authn.Token(r); h.Cfg.IDBaseURL != "" && token != "" {
		for _, e := range hubnotify.Mencionados(c.Body) {
			// De la dirección a la persona con la sesión de quien escribe: Sytue ID solo
			// contesta por gente de SU organización.
			sub, err := peticiones.PorCorreo(ctx, h.Cfg.IDBaseURL, token, e)
			if err != nil || sub == "" || avisados[sub] {
				continue
			}
			if _, err := h.Store.RolEnRegistro(ctx, domain.Actor{Sub: sub, Tenant: a.Tenant}, c.RecordID); err != nil {
				continue // no puede leerlo: avisarle sería enseñárselo
			}
			avisados[sub] = true
			if h.mencionada.Primera(a.Tenant + "|" + a.Sub + "|" + sub + "|" + c.RecordID) {
				h.Hub.SendAsync(ctx, aviso(sub, "bases.mention", "Te han mencionado en"))
			}
		}
	}
	for _, sub := range av.Destinatarios {
		if !avisados[sub] {
			avisados[sub] = true
			h.Hub.SendAsync(ctx, aviso(sub, "bases.comment", "Comentario en"))
		}
	}
}
