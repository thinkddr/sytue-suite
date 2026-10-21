// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// Quién modera al borrar un comentario: solo editor y propietario. La regla en sí
// (autor o moderador) vive en la sentencia del almacén y la prueba la de integración.
func TestBorrarComentarioPasaElRolDeModeracion(t *testing.T) {
	for _, c := range []struct {
		rol       domain.Role
		moderador bool
	}{
		{domain.RoleCommenter, false}, {domain.RoleEditor, true}, {domain.RoleOwner, true},
	} {
		fs := &fakeStore{
			doc:   &domain.Document{ID: "d1", Kind: domain.KindDocument, OwnerSub: "otro"},
			perms: []domain.Permission{{PrincipalKind: domain.PrincipalUser, PrincipalID: "owner", Role: c.rol}},
		}
		rec := httptest.NewRecorder()
		newHandler(fs).deleteComment(rec, req(t, "DELETE", "/v1/x", ownerClaims(), map[string]string{"id": "d1", "commentId": "c1"}, ""))
		if fs.borrado == nil || fs.borrado.by != "owner" || fs.borrado.moderador != c.moderador {
			t.Errorf("rol %s: se pidió %+v, moderador esperado %v", c.rol, fs.borrado, c.moderador)
		}
	}
}
