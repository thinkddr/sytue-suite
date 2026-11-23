// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var (
	// ErrReautenticacion: el servicio pide identificarse otra vez (RFC 9470,
	// `insufficient_user_authentication`). Se vuelve a entrar en Sytue ID y se repite.
	ErrReautenticacion = errors.New("api: hace falta volver a identificarse")
	// ErrFuenteCambiada: la fuente ya no está congelada en la época leída.
	ErrFuenteCambiada = errors.New("api: la fuente del servidor ha cambiado")
)

// SoltarFuentePrivada es «soltar la fuente» (ADR 0086): devuelve a `legacy` la
// fuente congelada o privada de `scope` en el servicio `servicio` (su raíz,
// ServiceURL). Lee la época que hay ahora y pide soltar esa, con la
// confirmación explícita en el cuerpo: lo que se escribió en Matrix después de
// congelarse se pierde, y quien llama ya lo ha confirmado con la persona.
//
// Si la fuente ya está en `legacy`, no hace nada. Sin una identificación
// reciente devuelve ErrReautenticacion.
func SoltarFuentePrivada(ctx context.Context, servicio string, s Session, scope storage.Scope) error {
	if !scope.Valid() {
		return storage.ErrRecord
	}
	c := New(servicio+"/api/v1", s, nil)
	state, err := privateMigrationState(ctx, c, scope, scope.Product)
	if err != nil {
		return err
	}
	switch {
	case state.Mode == "legacy":
		return nil
	case (state.Mode != "frozen" && state.Mode != "private") || !storage.MigrationEpoch(state.Epoch):
		return storage.ErrRecord
	}
	request, err := privateAppRequest(scope, scope.Product, "/private-storage/migration/release")
	if err != nil {
		return err
	}
	request.Method = http.MethodPost
	request.Header.Set("Content-Type", "application/json")
	request.Body, _ = json.Marshal(struct {
		Epoch   string `json:"epoch"`
		Discard bool   `json:"discard_private_writes"`
	}{state.Epoch, true})
	answer, err := JSON[struct {
		Mode string `json:"mode"`
	}](ctx, c, request)
	var failure *Error
	switch {
	case errors.As(err, &failure) && failure.Status == http.StatusUnauthorized && failure.Code() == "insufficient_user_authentication":
		return ErrReautenticacion
	case errors.As(err, &failure) && failure.Status == http.StatusConflict && failure.Code() == "private_storage_release_stale":
		return ErrFuenteCambiada
	case err != nil:
		return err
	case answer.Mode != "legacy":
		return storage.ErrRecord
	}
	return nil
}
