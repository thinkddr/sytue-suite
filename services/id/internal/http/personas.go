// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Encontrar a una persona de CUALQUIER organización de Sytue One por su @usuario
// o por su correo (ADR 0040).
//
// ES LO QUE directory:resolve NO HACE A PROPÓSITO. Aquella puerta traduce correos
// de MI organización y contesta 404 igual a «no existe» que a «es de otra»,
// porque compartir un fichero no tiene por qué contar dónde trabaja alguien.
// Escribirle a alguien de otra empresa sí necesita encontrarle: es la función.
//
// LAS BARRERAS:
//
//  1. EXACTO. Un @usuario entero o una dirección entera; ni prefijos ni listados.
//     Para barrer hay que saber ya lo que se busca.
//  2. LO MÍNIMO. Identificador, nombre visible y @usuario. NO el correo de
//     acceso —puede ser el Gmail de alguien—, ni la organización, ni el rol.
//  3. POR PERSONA Y CON TOPE. El mismo limitador que el directorio: uno a uno y
//     sin tope, esto sería la forma de averiguar qué correos tienen cuenta.
//  4. SOLO CUENTAS ACTIVAS. Una baja o una suspensión no se encuentra.
//
// El camino de vuelta (user_id → nombre) lo usa Sytue Rooms para no fiarse del
// nombre que le mande un navegador al añadir a alguien de fuera.

type resolvePeopleRequest struct {
	// Query es lo que se teclea: «@ana», «ana» o «ana@acme.com».
	Query  string `json:"query"`
	UserID string `json:"user_id"`
}

// handlePeopleResolve — POST /api/v1/people:resolve.
func (s *Server) handlePeopleResolve(w http.ResponseWriter, r *http.Request) {
	var req resolvePeopleRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	var (
		u   domain.User
		err error
	)
	switch id := strings.TrimSpace(req.UserID); {
	case id != "":
		if _, perr := uuid.Parse(id); perr != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid user identifier."))
			return
		}
		u, err = s.store.GetUserByID(ctx, id)
	default:
		q := domain.NormalizeEmail(req.Query)
		if username, ok := domain.UsernameDeLogin(q); ok {
			if !domain.ValidUsername(username) {
				httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Enter a Sytue One username or an email address."))
				return
			}
			u, err = s.store.GetUserByUsername(ctx, username)
			break
		}
		if a, perr := mail.ParseAddress(q); perr != nil || !strings.EqualFold(a.Address, q) {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Enter a Sytue One username or an email address."))
			return
		}
		u, err = s.store.GetUserByAddress(ctx, q)
	}
	if errors.Is(err, domain.ErrNotFound) || (err == nil && !u.Active()) {
		httpx.Fail(w, r, httpx.ErrNotFound.WithMessage("Nobody on Sytue One uses that username or address."))
		return
	}
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	out := map[string]any{"user_id": u.ID, "name": u.Name}
	// Vacío solo en la ventana entre el alta por un binario anterior y el barrido
	// que se lo asigna: se omite antes que mandar un @ sin nombre.
	if u.Username != "" {
		out["username"] = u.Username
	}
	httpx.JSON(w, r, http.StatusOK, out)
}
