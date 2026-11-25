// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package vaultproof

import (
	"errors"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

func TestVaultproofAutoridadDeLaBoveda(t *testing.T) {
	_, reg := firmante(t, 1)
	scope := accionDePrueba(t, reg, "key.enroll").Scope
	a, err := Authority(scope, reg)
	if err != nil {
		t.Fatal(err)
	}
	if a.RequireActive(scope, reg.Generation) != nil || a.UserID != UserID(scope) || a.RootKey != RootKey(reg.Public) {
		t.Fatalf("autoridad = %+v", a)
	}
	if !IsAuthority(a) {
		t.Fatal("la autoridad de una bóveda no se reconoce")
	}
	matrix := a
	matrix.UserID = storage.IsolatedMatrixUserPrefix(scope) + "matrix.sytue.eu"
	if IsAuthority(matrix) || IsAuthority(matrixproof.Authority{}) {
		t.Fatal("una autoridad de Matrix pasa por la de una bóveda")
	}
	mala := reg
	mala.KeyID = "otra"
	if _, err := Authority(scope, mala); !errors.Is(err, ErrUnverified) {
		t.Fatalf("una clave cuyo id no cuadra da autoridad: %v", err)
	}
	if _, err := Authority(storage.Scope{}, reg); !errors.Is(err, ErrUnverified) {
		t.Fatalf("un ámbito inválido da autoridad: %v", err)
	}
}

// El navegador calcula la misma identidad (pqVaultUserID en
// packages/offline/src/vault/pq-vault.ts, con este mismo literal en su prueba).
func TestVaultproofIdentidadLiteral(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "0f3c9a52-6d1e-4c1a-9b8e-2a7d5c4e1f00", Product: "mail"}
	if got := UserID(scope); got != "@sytue_storage_mail_acme_0f3c9a52-6d1e-4c1a-9b8e-2a7d5c4e1f00:sytue-vault" {
		t.Fatalf("UserID = %q", got)
	}
}

type casoFirma func() (*recoverywrap.PQVaultSigner, matrixproof.Intent, matrixproof.Action, time.Time)

func TestVaultproofFirmaSoloLoPedido(t *testing.T) {
	s, reg := firmante(t, 1)
	esperada := accionDePrueba(t, reg, "task.ack")
	prueba, err := SignIntent(s, esperada.Intent(), esperada, ahora)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(esperada, prueba, reg, ahora); err != nil {
		t.Fatalf("lo firmado por SignIntent no verifica: %v", err)
	}
	otra, _ := firmante(t, 2)
	casos := map[string]casoFirma{
		"el reto cambia el destino": func() (*recoverywrap.PQVaultSigner, matrixproof.Intent, matrixproof.Action, time.Time) {
			a := esperada
			a.Target.DeviceID = "pq-otro"
			return s, esperada.Intent(), a, ahora
		},
		"el reto cambia la petición": func() (*recoverywrap.PQVaultSigner, matrixproof.Intent, matrixproof.Action, time.Time) {
			a := esperada
			a.RequestSHA256 = strings.Repeat("0", 64)
			return s, esperada.Intent(), a, ahora
		},
		"caducada": func() (*recoverywrap.PQVaultSigner, matrixproof.Intent, matrixproof.Action, time.Time) {
			return s, esperada.Intent(), esperada, ahora.Add(2 * time.Minute)
		},
		"raíz que no es la de esta clave": func() (*recoverywrap.PQVaultSigner, matrixproof.Intent, matrixproof.Action, time.Time) {
			return otra, esperada.Intent(), esperada, ahora
		},
		"acción de raíz": func() (*recoverywrap.PQVaultSigner, matrixproof.Intent, matrixproof.Action, time.Time) {
			a := esperada
			a.Kind = "root.enroll"
			return s, a.Intent(), a, ahora
		},
		"sin clave": func() (*recoverywrap.PQVaultSigner, matrixproof.Intent, matrixproof.Action, time.Time) {
			return nil, esperada.Intent(), esperada, ahora
		},
	}
	for nombre, c := range casos {
		quien, intent, a, cuando := c()
		if _, err := SignIntent(quien, intent, a, cuando); !errors.Is(err, ErrUnverified) {
			t.Errorf("%s: %v; quiero ErrUnverified", nombre, err)
		}
	}
}
