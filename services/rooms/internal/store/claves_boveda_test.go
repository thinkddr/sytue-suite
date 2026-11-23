// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

type clavesFijas struct {
	reg    vaultproof.Registered
	motivo string
	creada time.Time
	err    error
}

func (c clavesFijas) Activa(context.Context, storage.Scope) (ClaveBoveda, error) {
	return ClaveBoveda{Registered: c.reg, Motivo: c.motivo, Creada: c.creada}, c.err
}

const cbSujeto = "0f3c9a52-6d1e-4c1a-9b8e-2a7d5c4e1f00"

func cbBoveda(t *testing.T) (*recoverywrap.PQVaultSigner, vaultproof.Registered, context.Context, matrixproof.Action) {
	t.Helper()
	s, err := recoverywrap.ParsePQVaultSigningKey(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 64)))
	if err != nil {
		t.Fatal(err)
	}
	keyID, err := s.Public().ID()
	if err != nil {
		t.Fatal(err)
	}
	reg := vaultproof.Registered{Generation: 1, KeyID: keyID, Public: s.Public()}
	tn, err := tenancy.New("acme")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := matrixproof.RequestDigest(map[string]string{"r": "x"})
	if err != nil {
		t.Fatal(err)
	}
	ahora := time.Now()
	a := matrixproof.Action{
		Version: 1, Generation: 1, Scope: storage.Scope{Tenant: "acme", Subject: cbSujeto, Product: "mail"},
		Epoch: "11111111-1111-4111-8111-111111111111", Kind: "key.enroll",
		Target:  storage.PublicationTarget{RoomID: "!pqv_x:sytue-vault", DeviceID: "pq-x"},
		RootKey: base64.RawStdEncoding.EncodeToString(s.Public().Ed25519), RequestSHA256: digest,
		Nonce: "n", IssuedAt: ahora.UnixMilli(), ExpiresAt: ahora.Add(time.Minute).UnixMilli(),
	}
	return s, reg, tenancy.Into(context.Background(), tn), a
}

// Una prueba de bóveda va por Sytue ID y nunca toca la autoridad de Matrix.
func TestVerifyMatrixStorageActionConBoveda(t *testing.T) {
	s, reg, ctx, esperada := cbBoveda(t)
	prueba, err := vaultproof.Sign(s, esperada)
	if err != nil {
		t.Fatal(err)
	}
	// Sin base ni Matrix: la vía de bóveda solo pregunta sobre qué generación de
	// Matrix se cuenta (0: el ámbito no tiene autoridad de Matrix).
	st := &Store{baseBoveda: func(context.Context, string, string) (int64, error) { return 0, nil }}
	if _, err := st.VerifyMatrixStorageAction(ctx, cbSujeto, "mail", esperada, prueba); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("sin Sytue ID configurado: %v; quiero ErrForbidden", err)
	}
	st.SetClavesBoveda(clavesFijas{reg: reg})
	auth, err := st.VerifyMatrixStorageAction(ctx, cbSujeto, "mail", esperada, prueba)
	if err != nil || auth.Validate(esperada) != nil {
		t.Fatalf("autorización: %+v, %v", auth, err)
	}
	st.SetClavesBoveda(clavesFijas{err: ErrSinClaveDeBoveda})
	if _, err := st.VerifyMatrixStorageAction(ctx, cbSujeto, "mail", esperada, prueba); !errors.Is(err, matrixproof.ErrUnverified) {
		t.Fatalf("sin clave registrada: %v", err)
	}
	st.SetClavesBoveda(clavesFijas{err: errors.New("caído")})
	if _, err := st.VerifyMatrixStorageAction(ctx, cbSujeto, "mail", esperada, prueba); !errors.Is(err, domain.ErrMatrixUnavailable) {
		t.Fatalf("Sytue ID caído: %v", err)
	}
	// Para otra persona del mismo tenant, la acción no casa con el ámbito.
	st.SetClavesBoveda(clavesFijas{reg: reg})
	if _, err := st.VerifyMatrixStorageAction(ctx, "otra-persona", "mail", esperada, prueba); err == nil {
		t.Fatal("una acción de otra persona autorizada")
	}
}

func TestClavesBovedaIDPreguntaASytueID(t *testing.T) {
	_, reg, _, _ := cbBoveda(t)
	var pedido map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/vault-keys/lookup" || r.Header.Get("Authorization") != "Bearer s3creto" {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&pedido)
		if pedido["product"] == "drive" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"generation": reg.Generation, "key_id": reg.KeyID, "public_key": reg.Public, "reason": "traspaso", "created_at": "2026-11-23T10:00:00Z"})
	}))
	defer srv.Close()
	if NuevasClavesBovedaID(srv.URL, "") != nil {
		t.Fatal("sin secreto hay cliente")
	}
	c := NuevasClavesBovedaID(srv.URL+"/", "s3creto")
	scope := storage.Scope{Tenant: "acme", Subject: cbSujeto, Product: "mail"}
	got, err := c.Activa(context.Background(), scope)
	if err != nil || got.KeyID != reg.KeyID || got.Generation != 1 || got.Motivo != "traspaso" || !got.Creada.Equal(time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("activa: %+v, %v", got, err)
	}
	if pedido["subject"] != cbSujeto || pedido["tenant"] != "acme" {
		t.Fatalf("pedido = %v", pedido)
	}
	scope.Product = "drive"
	if _, err := c.Activa(context.Background(), scope); !errors.Is(err, ErrSinClaveDeBoveda) {
		t.Fatalf("404: %v", err)
	}
	if _, err := NuevasClavesBovedaID(srv.URL, "otro").Activa(context.Background(), scope); err == nil {
		t.Fatal("un 403 tomado por respuesta")
	}
}
