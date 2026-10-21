// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package baseshttp

import (
	"cmp"
	"errors"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// PERMISOS: PERSONAS, GRUPOS Y PROPIEDAD
//
// Una PERSONA se concede por su dirección (`email`): el servicio la resuelve en el
// directorio de Sytue ID con la sesión de quien concede, que solo contesta por gente de
// SU organización. Así el propietario no tiene que conocer un `sub` y nadie de fuera
// acaba en la ACL por un identificador pegado a mano.
//
// Un GRUPO se concede por su nombre y aplica a quien lo traiga en el claim `groups` de
// su token (lo emite Sytue ID con los grupos de la persona en la organización activa):
// un cambio de pertenencia se ve cuando el token se renueva, en minutos.
//
// TRANSFERIR la propiedad solo admite una dirección, nunca un `sub` del cliente: que el
// destino es de la organización lo dice Sytue ID, no quien llama.

// personaDeLaOrg resuelve una dirección a (sub, etiqueta) con la sesión de quien llama.
func (h *Handlers) personaDeLaOrg(r *http.Request, email string) (sub, etiqueta string, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	token := authn.Token(r)
	if email == "" || len(email) > 254 || h.Cfg.IDBaseURL == "" || token == "" {
		return "", "", domain.ErrInvalido
	}
	sub, err = peticiones.PorCorreo(r.Context(), h.Cfg.IDBaseURL, token, email)
	if err != nil {
		return "", "", err
	}
	_, nombre, _ := peticiones.Persona(r.Context(), h.Cfg.IDBaseURL, token, sub)
	return sub, cmp.Or(strings.TrimSpace(nombre), email), nil
}

// errEsMiembro: se intentó invitar «de fuera» a alguien de la propia organización.
var errEsMiembro = errors.New("bases: esa persona es de la organización")

// personaDeFuera resuelve un @usuario o una dirección a alguien de OTRA organización
// (people:resolve, ADR 0040: da su `sub` y cómo se llama, nunca su correo ni su
// organización). Si resulta ser de la organización de quien llama, no es «de fuera»:
// se comparte con ella como persona, y aquí es errEsMiembro.
func (h *Handlers) personaDeFuera(r *http.Request, consulta string) (sub, etiqueta string, err error) {
	consulta = strings.TrimSpace(consulta)
	token := authn.Token(r)
	if consulta == "" || len(consulta) > 254 || h.Cfg.IDBaseURL == "" || token == "" {
		return "", "", domain.ErrInvalido
	}
	sub, nombre, err := peticiones.DeFuera(r.Context(), h.Cfg.IDBaseURL, token, consulta)
	if err != nil {
		return "", "", err
	}
	if _, _, err := peticiones.Persona(r.Context(), h.Cfg.IDBaseURL, token, sub); err == nil {
		return "", "", errEsMiembro
	} else if !errors.Is(err, peticiones.ErrSinPersona) {
		// Sin saber si es de la organización no se la trata como de fuera.
		return "", "", err
	}
	return sub, cmp.Or(nombre, consulta), nil
}

type entradaPermiso struct {
	TableID       string `json:"table_id"`
	PrincipalKind string `json:"principal_kind"`
	PrincipalID   string `json:"principal_id"`
	// Email concede a una PERSONA por su dirección (principal_kind «user» sin principal_id).
	Email string `json:"email"`
	// Query concede a alguien de OTRA organización por su @usuario o su dirección
	// (principal_kind «external»): un `sub` de fuera nunca se acepta del cliente.
	Query string     `json:"query"`
	Role  domain.Rol `json:"role"`
}

func (h *Handlers) concederPermiso(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	bid, err2 := id(r, "id")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in entradaPermiso
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p := domain.Permiso{TableID: in.TableID, PrincipalKind: in.PrincipalKind, PrincipalID: in.PrincipalID, Role: in.Role}
	if p.TableID != "" {
		if p.TableID, err = idDe(p.TableID); err != nil {
			httpx.Fail(w, r, errorAPI(domain.ErrInvalido))
			return
		}
	}
	// Una persona de fuera SOLO entra resuelta por el directorio: nunca por un `sub` pegado.
	if (p.PrincipalKind == domain.PrincipalExterno) != (in.Query != "") || (in.Query != "" && (p.PrincipalID != "" || in.Email != "")) {
		httpx.Fail(w, r, errorAPI(domain.ErrInvalido))
		return
	}
	if in.Query != "" {
		if _, err := h.Store.ListarPermisos(r.Context(), a, bid); err != nil {
			httpx.Fail(w, r, errorAPI(err))
			return
		}
		if p.PrincipalID, p.PrincipalLabel, err = h.personaDeFuera(r, in.Query); err != nil {
			httpx.Fail(w, r, errorAPI(err))
			return
		}
	}
	if in.Email != "" {
		if p.PrincipalKind != domain.PrincipalUsuario || p.PrincipalID != "" {
			httpx.Fail(w, r, errorAPI(domain.ErrInvalido))
			return
		}
		// Antes de preguntar al directorio: quien no gestiona los permisos no lo usa de oráculo.
		if _, err := h.Store.ListarPermisos(r.Context(), a, bid); err != nil {
			httpx.Fail(w, r, errorAPI(err))
			return
		}
		if p.PrincipalID, p.PrincipalLabel, err = h.personaDeLaOrg(r, in.Email); err != nil {
			httpx.Fail(w, r, errorAPI(err))
			return
		}
	}
	p, err = h.Store.ConcederPermiso(r.Context(), a, bid, p)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, p)
}

// misGrupos devuelve los grupos de quien llama (los de su token): es de donde el
// diálogo de compartir ofrece nombres. Un grupo al que no pertenece se escribe a mano.
func (h *Handlers) misGrupos(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	grupos := a.Groups
	if grupos == nil {
		grupos = []string{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"groups": grupos})
}

func (h *Handlers) transferirBase(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	bid, err2 := id(r, "id")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		Email string `json:"email"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Solo el propietario llega al directorio (ListarPermisos exige serlo).
	if _, err := h.Store.ListarPermisos(r.Context(), a, bid); err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	nuevo, _, err := h.personaDeLaOrg(r, in.Email)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	nombre, err := h.Store.TransferirBase(r.Context(), a, bid, nuevo)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	if h.Hub != nil {
		cl, _ := authn.FromContext(r.Context())
		quien := cmp.Or(strings.TrimSpace(cl.Name), cl.Email)
		params := map[string]string{"name": nombre}
		if quien != "" {
			params["actor"] = quien
		}
		h.Hub.SendAsync(r.Context(), hubnotify.Notification{
			Tenant: a.Tenant, Subject: nuevo, Source: "bases", Type: "bases.transferred", Level: "info",
			Title:  "Ahora eres propietario de «" + nombre + "»",
			URL:    hubnotify.EnlaceBase(h.Cfg.PublicURL, a.Tenant, bid),
			Params: params,
		})
	}
	w.WriteHeader(http.StatusNoContent)
}
