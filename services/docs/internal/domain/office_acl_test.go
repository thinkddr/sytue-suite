// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"context"
	"errors"
	"testing"
	"time"
)

// La misma frontera del dominio atiende POST /ops y los lotes WS: no depende
// de ocultar un botón ni del tipo de lienzo (Docus, Calcus o Diapos).
func TestOfficeMatrizDeEscrituraPorRolYTipo(t *testing.T) {
	for _, kind := range []Kind{KindDocument, KindSpreadsheet, KindPresentation} {
		for _, role := range []Role{RoleViewer, RoleCommenter, RoleEditor, RoleOwner, "", "unknown"} {
			t.Run(string(kind)+"/"+string(role), func(t *testing.T) {
				store := &fakeStore{doc: &Document{ID: "d1", Kind: kind, OwnerSub: "owner"}, perms: []Permission{{PrincipalKind: PrincipalUser, PrincipalID: "ana", Role: role}}}
				_, _, err := NewService(store).ApplyOps(context.Background(), Subject{Sub: "ana"}, "d1", []Op{{Kind: OpSet, Key: "doc:test"}}, nil)
				if role == RoleEditor || role == RoleOwner {
					if err != nil || len(store.appended) != 1 {
						t.Fatalf("editor autorizado no escribió: %v", err)
					}
				} else if !errors.Is(err, ErrForbidden) || len(store.appended) != 0 {
					t.Fatalf("bypass de escritura para %q: %v, %d ops", role, err, len(store.appended))
				}
			})
		}
	}
}

func TestOfficeEnlacesNoConcedenRolesInvalidosNiPropiedad(t *testing.T) {
	for _, role := range []Role{RoleOwner, "", "unknown"} {
		if _, ok := ResolveShare(&ShareLink{Role: role}, func(*ShareLink) bool { return false }); ok {
			t.Fatalf("enlace concedió %q", role)
		}
		svc := NewService(&fakeStore{shareRes: &ShareResolution{DocumentID: "d1", Role: role}})
		if _, err := svc.ResolveShare(context.Background(), "token"); !errors.Is(err, ErrShareExpired) {
			t.Fatalf("servicio concedió %q: %v", role, err)
		}
	}
	if RoleViewer.AtLeast(Role("unknown")) || Role("unknown").AtLeast(Role("unknown")) {
		t.Fatal("un rol desconocido se tomó por autorizado")
	}
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	link := ShareLink{Role: RoleViewer, ExpiresAt: &now}
	if link.Active(now) {
		t.Fatal("el enlace sigue activo en su instante de caducidad")
	}
}
