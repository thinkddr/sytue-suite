// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

type recibosFijos struct {
	err     error
	llamado int
}

func (r *recibosFijos) Verificar(context.Context, storage.Scope, storage.PublicationTarget, storage.Reference) error {
	r.llamado++
	return r.err
}

func TestVerificarReciboEligeElCamino(t *testing.T) {
	ctx := context.Background()
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	boveda := matrixproof.Authority{Version: 1, Scope: scope, UserID: vaultproof.UserID(scope), RootKey: strings.Repeat("A", 43), Generation: 2, Status: "active"}
	matrix := boveda
	matrix.UserID = storage.IsolatedMatrixUserPrefix(scope) + "matrix.sytue.eu"
	target := storage.PublicationTarget{RoomID: "!pqv_A", DeviceID: "pq-x"}
	receipt := storage.Reference{RoomID: "!pqv_A", EventID: "$pqe_B"}
	porMatrix := 0
	viaMatrix := func() error { porMatrix++; return nil }

	r := &recibosFijos{}
	if err := verificarRecibo(ctx, scope, boveda, nil, target, receipt, r, viaMatrix); err != nil || r.llamado != 1 || porMatrix != 0 {
		t.Fatalf("bóveda: %v, Drive %d, Matrix %d", err, r.llamado, porMatrix)
	}
	if err := verificarRecibo(ctx, scope, matrix, nil, target, receipt, r, viaMatrix); err != nil || r.llamado != 1 || porMatrix != 1 {
		t.Fatalf("Matrix: %v, Drive %d, Matrix %d", err, r.llamado, porMatrix)
	}
	if err := verificarRecibo(ctx, scope, matrixproof.Authority{}, matrixproof.ErrRootEnrollment, target, receipt, r, viaMatrix); err != nil || porMatrix != 2 {
		t.Fatalf("sin autoridad: %v", err)
	}
	no := &recibosFijos{err: matrixproof.ErrUnverified}
	if err := verificarRecibo(ctx, scope, boveda, nil, target, receipt, no, viaMatrix); !errors.Is(err, matrixproof.ErrUnverified) {
		t.Fatalf("Drive dice que no: %v", err)
	}
	if err := verificarRecibo(ctx, scope, boveda, nil, target, receipt, nil, viaMatrix); !errors.Is(err, domain.ErrMatrixUnavailable) {
		t.Fatalf("sin Drive configurado: %v", err)
	}
	revocada := boveda
	revocada.Status = "revoked"
	if err := verificarRecibo(ctx, scope, revocada, nil, target, receipt, r, viaMatrix); !errors.Is(err, matrixproof.ErrUnverified) {
		t.Fatalf("revocada: %v", err)
	}
	ajeno := storage.Reference{RoomID: "!otra", EventID: "$pqe_B"}
	if err := verificarRecibo(ctx, scope, boveda, nil, target, ajeno, r, viaMatrix); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("recibo de otra sala: %v", err)
	}
	if porMatrix != 2 {
		t.Fatalf("un recibo de bóveda acabó en Matrix (%d)", porMatrix)
	}
}

func TestRecibosBovedaDrive(t *testing.T) {
	if NuevosRecibosBovedaDrive("http://drive", "") != nil {
		t.Fatal("sin secreto hay recibos de bóveda")
	}
	estado := http.StatusNoContent
	var visto map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/pq-vault/receipt" || r.Header.Get("Authorization") != "Bearer s3cr3t" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&visto)
		w.WriteHeader(estado)
	}))
	defer srv.Close()
	c := NuevosRecibosBovedaDrive(srv.URL+"/", "s3cr3t")
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	target := storage.PublicationTarget{RoomID: "!pqv_A", DeviceID: "pq-x"}
	receipt := storage.Reference{RoomID: "!pqv_A", EventID: "$pqe_B"}
	ctx := context.Background()
	if err := c.Verificar(ctx, scope, target, receipt); err != nil || visto["receipt"] == nil {
		t.Fatalf("204: %v (%v)", err, visto)
	}
	estado = http.StatusNotFound
	if err := c.Verificar(ctx, scope, target, receipt); !errors.Is(err, matrixproof.ErrUnverified) {
		t.Fatalf("404: %v", err)
	}
	for _, s := range []int{http.StatusOK, http.StatusUnauthorized, http.StatusServiceUnavailable} {
		estado = s
		if err := c.Verificar(ctx, scope, target, receipt); !errors.Is(err, domain.ErrMatrixUnavailable) {
			t.Fatalf("%d: %v", s, err)
		}
	}
}
