// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func TestAutoridadConBoveda(t *testing.T) {
	_, reg, ctx, a := cbBoveda(t)
	scope := a.Scope
	matrix := matrixproof.Authority{Version: 1, Scope: scope, UserID: storage.IsolatedMatrixUserPrefix(scope) + "matrix.test", RootKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", Generation: 4, Status: "active"}
	hay := clavesFijas{reg: reg}
	no := clavesFijas{err: ErrSinClaveDeBoveda}
	caida := clavesFijas{err: errors.New("sytue id caído")}

	// Con autoridad de Matrix, manda Matrix aunque haya bóveda.
	got, err := autoridadConBoveda(ctx, scope, matrix, nil, hay)
	if err != nil || got != matrix {
		t.Fatalf("con Matrix: %+v, %v", got, err)
	}
	// Una de Matrix revocada o un fallo de la base no se tapan con la bóveda.
	falla := errors.New("base")
	if _, err := autoridadConBoveda(ctx, scope, matrix, falla, hay); !errors.Is(err, falla) {
		t.Fatalf("un fallo de Matrix se tapa: %v", err)
	}
	// Sin Matrix y con bóveda: la de la bóveda.
	got, err = autoridadConBoveda(ctx, scope, matrixproof.Authority{}, matrixproof.ErrRootEnrollment, hay)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := vaultproof.Authority(scope, reg)
	if got != want || got.UserID != vaultproof.UserID(scope) || got.Generation != reg.Generation {
		t.Fatalf("sin Matrix: %+v", got)
	}
	// Sin ninguna: alta, como antes.
	if _, err := autoridadConBoveda(ctx, scope, matrixproof.Authority{}, matrixproof.ErrRootEnrollment, no); !errors.Is(err, matrixproof.ErrRootEnrollment) {
		t.Fatalf("sin nada: %v", err)
	}
	// Sytue ID caído: nunca «alta».
	if _, err := autoridadConBoveda(ctx, scope, matrixproof.Authority{}, matrixproof.ErrRootEnrollment, caida); !errors.Is(err, domain.ErrMatrixUnavailable) {
		t.Fatalf("Sytue ID caído: %v", err)
	}
	// Sin registro configurado, como antes.
	if _, err := autoridadConBoveda(ctx, scope, matrixproof.Authority{}, matrixproof.ErrRootEnrollment, nil); !errors.Is(err, matrixproof.ErrRootEnrollment) {
		t.Fatalf("sin registro: %v", err)
	}
	// Calendario tiene clave de bóveda pero recibe por Matrix.
	cal := scope
	cal.Product = "calendar"
	if _, err := autoridadConBoveda(ctx, cal, matrixproof.Authority{}, matrixproof.ErrRootEnrollment, hay); !errors.Is(err, matrixproof.ErrRootEnrollment) {
		t.Fatalf("calendario: %v", err)
	}
}

func TestRaizMatrixEnAmbitoDeBoveda(t *testing.T) {
	_, reg, _, a := cbBoveda(t)
	ctx := context.Background()
	scope := a.Scope
	if err := raizMatrixPermitidaEn(ctx, scope, clavesFijas{reg: reg}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("una raíz de Matrix entra en un ámbito de bóveda: %v", err)
	}
	if err := raizMatrixPermitidaEn(ctx, scope, clavesFijas{err: ErrSinClaveDeBoveda}); err != nil {
		t.Fatalf("sin bóveda: %v", err)
	}
	if err := raizMatrixPermitidaEn(ctx, scope, clavesFijas{err: errors.New("caído")}); !errors.Is(err, domain.ErrMatrixUnavailable) {
		t.Fatalf("Sytue ID caído: %v", err)
	}
	if err := raizMatrixPermitidaEn(ctx, scope, nil); err != nil {
		t.Fatalf("sin registro: %v", err)
	}
	cal := scope
	cal.Product = "calendar"
	if err := raizMatrixPermitidaEn(ctx, cal, clavesFijas{reg: reg}); err != nil {
		t.Fatalf("calendario: %v", err)
	}
}
