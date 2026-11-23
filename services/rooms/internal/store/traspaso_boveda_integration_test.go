//go:build integration

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// clavesDeSytueID es Sytue ID visto por Rooms: la clave activa de la bóveda, su
// motivo y su fecha. Se cambia entre pasos del test.
type clavesDeSytueID struct {
	clave store.ClaveBoveda
	err   error
}

func (c *clavesDeSytueID) Activa(context.Context, storage.Scope) (store.ClaveBoveda, error) {
	return c.clave, c.err
}

func firmanteDeBoveda(t *testing.T, semilla byte, generacion int64) (*recoverywrap.PQVaultSigner, vaultproof.Registered) {
	t.Helper()
	s, err := recoverywrap.ParsePQVaultSigningKey(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{semilla}, 64)))
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.Public().ID()
	if err != nil {
		t.Fatal(err)
	}
	return s, vaultproof.Registered{Generation: generacion, KeyID: id, Public: s.Public()}
}

func TestTraspasoDeMatrixALaBoveda(t *testing.T) {
	f := newAuthorityFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	challenge := f.challenge(t)
	if _, err := f.st.ConfirmMatrixRoot(ctx, f.scope.Subject, f.scope.Product, f.sign(t, challenge.Action)); err != nil {
		t.Fatal(err)
	}
	matrix, err := f.st.MatrixStorageAuthority(ctx, f.scope.Subject, f.scope.Product)
	if err != nil || matrix.Generation != 1 || vaultproof.IsAuthority(matrix) {
		t.Fatal("sin autoridad de Matrix de partida", matrix, err)
	}
	accion := challenge.Action
	accion.Kind = "task.claim"
	accion.Nonce = uuid.NewString()
	accion.RequestSHA256, _ = matrixproof.RequestDigest(map[string]string{"task": "traspaso"})
	pruebaMatrix := f.sign(t, accion)
	if _, err := f.st.VerifyMatrixStorageAction(ctx, f.scope.Subject, f.scope.Product, accion, pruebaMatrix); err != nil {
		t.Fatal("la prueba de Matrix no valía antes del traspaso", err)
	}

	firmante, reg := firmanteDeBoveda(t, 7, 1)
	sytue := &clavesDeSytueID{}
	f.st.SetClavesBoveda(sytue)
	leer := func() (matrixproof.Authority, error) {
		return f.st.MatrixStorageAuthority(ctx, f.scope.Subject, f.scope.Product)
	}

	// Ni un alta, ni un traspaso anterior al último cambio de Matrix cambian nada:
	// quien abre su correo de Matrix sigue igual. (Sytue ID caído antes del
	// traspaso deja Matrix y abre una pausa: lo prueba la unitaria, para no dejar
	// esta en pausa.)
	for nombre, c := range map[string]*clavesDeSytueID{
		"alta":           {clave: store.ClaveBoveda{Registered: reg, Motivo: "alta", Creada: time.Now().Add(time.Minute)}},
		"vaciado":        {clave: store.ClaveBoveda{Registered: reg, Motivo: "vaciado", Creada: time.Now().Add(time.Minute)}},
		"traspaso viejo": {clave: store.ClaveBoveda{Registered: reg, Motivo: "traspaso", Creada: time.Now().Add(-time.Hour)}},
		"sin clave":      {err: store.ErrSinClaveDeBoveda},
	} {
		*sytue = *c
		if got, err := leer(); err != nil || got != matrix {
			t.Fatalf("%s cambió la autoridad de Matrix: %+v, %v", nombre, got, err)
		}
		if _, err := f.st.VerifyMatrixStorageAction(ctx, f.scope.Subject, f.scope.Product, accion, pruebaMatrix); err != nil {
			t.Fatalf("%s: la prueba de Matrix dejó de valer: %v", nombre, err)
		}
	}

	// Y sin traspaso, una prueba de la bóveda no vale sobre un ámbito de Matrix,
	// aunque Sytue ID tenga su clave activa.
	*sytue = clavesDeSytueID{clave: store.ClaveBoveda{Registered: reg, Motivo: "alta", Creada: time.Now()}}
	sinTraspaso := accion
	sinTraspaso.Generation, sinTraspaso.RootKey, sinTraspaso.Nonce = 1, vaultproof.RootKey(reg.Public), uuid.NewString()
	sinTraspaso.Target = storage.PublicationTarget{RoomID: "!pqv_x:sytue-vault", DeviceID: "pq-x"}
	if prueba, err := vaultproof.Sign(firmante, sinTraspaso); err != nil {
		t.Fatal(err)
	} else if _, err := f.st.VerifyMatrixStorageAction(ctx, f.scope.Subject, f.scope.Product, sinTraspaso, prueba); !errors.Is(err, matrixproof.ErrUnverified) {
		t.Fatal("una bóveda firmó sobre un ámbito de Matrix sin traspaso", err)
	}

	// Un traspaso posterior: la autoridad pasa a la bóveda, contada encima de Matrix.
	*sytue = clavesDeSytueID{clave: store.ClaveBoveda{Registered: reg, Motivo: "traspaso", Creada: time.Now().Add(time.Minute)}}
	boveda, err := leer()
	if err != nil || !vaultproof.IsAuthority(boveda) || boveda.Generation != 2 || boveda.RootKey != vaultproof.RootKey(reg.Public) {
		t.Fatal("el traspaso no dio la autoridad a la bóveda", boveda, err)
	}
	// Desde entonces ninguna prueba ni cambio de Matrix vale.
	if _, err := f.st.VerifyMatrixStorageAction(ctx, f.scope.Subject, f.scope.Product, accion, pruebaMatrix); !errors.Is(err, matrixproof.ErrUnverified) {
		t.Fatal("una prueba de Matrix vale tras el traspaso", err)
	}
	if _, err := f.st.MatrixRootChangeChallenge(ctx, f.scope.Subject, f.scope.Product, "root.rotate", f.epoch, f.target); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("Matrix puede rotar su raíz tras el traspaso", err)
	}
	// La bóveda firma con la generación contada encima.
	deBoveda := accion
	deBoveda.Generation, deBoveda.RootKey, deBoveda.Nonce = 2, vaultproof.RootKey(reg.Public), uuid.NewString()
	deBoveda.Target = storage.PublicationTarget{RoomID: "!pqv_x:sytue-vault", DeviceID: "pq-x"}
	prueba, err := vaultproof.Sign(firmante, deBoveda)
	if err != nil {
		t.Fatal(err)
	}
	if auth, err := f.st.VerifyMatrixStorageAction(ctx, f.scope.Subject, f.scope.Product, deBoveda, prueba); err != nil || auth.Authority != boveda {
		t.Fatal("la prueba de la bóveda no vale tras el traspaso", auth, err)
	}

	// El traspaso queda escrito: una clave posterior por continuidad lo mantiene,
	// con la generación que sigue subiendo; y sin Sytue ID no se vuelve a Matrix.
	_, siguiente := firmanteDeBoveda(t, 8, 2)
	*sytue = clavesDeSytueID{clave: store.ClaveBoveda{Registered: siguiente, Motivo: "continuidad", Creada: time.Now().Add(2 * time.Minute)}}
	if got, err := leer(); err != nil || !vaultproof.IsAuthority(got) || got.Generation != 3 {
		t.Fatal("una continuidad devolvió la autoridad a Matrix", got, err)
	}
	*sytue = clavesDeSytueID{err: errors.New("caído")}
	if _, err := leer(); !errors.Is(err, domain.ErrMatrixUnavailable) {
		t.Fatal("Sytue ID caído tras el traspaso devolvió una autoridad", err)
	}

	// La fila es inmutable y solo hay una.
	for _, consulta := range []string{
		`UPDATE matrix_storage_vault_handovers SET matrix_generation=matrix_generation WHERE tenant=$1 AND subject=$2 AND product=$3`,
		`DELETE FROM matrix_storage_vault_handovers WHERE tenant=$1 AND subject=$2 AND product=$3`,
		`INSERT INTO matrix_storage_vault_handovers(tenant,subject,product,matrix_generation,key_generation,key_id) VALUES($1,$2,$3,1,1,repeat('a',64))`,
	} {
		err := f.h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
			if err := pg.SetTenant(ctx, tx, f.scope.Tenant); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `SELECT set_config('app.matrix_authority_owner',$1,true),set_config('app.matrix_authority_product',$2,true)`, f.scope.Subject, f.scope.Product); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, consulta, f.scope.Tenant, f.scope.Subject, f.scope.Product)
			return err
		})
		if err == nil {
			t.Fatal("la constancia del traspaso se pudo cambiar:", consulta)
		}
	}
}
