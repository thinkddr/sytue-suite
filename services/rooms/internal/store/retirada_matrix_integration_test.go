//go:build integration

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// La retirada de la autoridad de Matrix del correo (ADR 0087, entrega 5), de
// punta a punta sobre la base: una cuenta de Matrix sin traspaso lee «retirada»,
// Matrix ya no firma ni cambia de raíz, el traspaso sigue convirtiendo, y apagar
// el conmutador lo deja todo como estaba.
func TestMatrixRetiradaDelCorreo(t *testing.T) {
	f := newAuthorityFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	challenge := f.challenge(t)
	confirmacion := f.sign(t, challenge.Action)
	if _, err := f.st.ConfirmMatrixRoot(ctx, f.scope.Subject, f.scope.Product, confirmacion); err != nil {
		t.Fatal(err)
	}
	matrix, err := f.st.MatrixStorageAuthority(ctx, f.scope.Subject, f.scope.Product)
	if err != nil || vaultproof.IsAuthority(matrix) {
		t.Fatal("sin autoridad de Matrix de partida", matrix, err)
	}
	accion := challenge.Action
	accion.Kind = "task.claim"
	accion.Nonce = uuid.NewString()
	accion.RequestSHA256, _ = matrixproof.RequestDigest(map[string]string{"task": "retirada"})
	pruebaMatrix := f.sign(t, accion)
	recibo := storage.Reference{RoomID: f.target.RoomID, EventID: "$cifrado"}
	origen := 0
	verificar := func(context.Context, storage.Scope, storage.Reference) error { origen++; return nil }

	firmante, reg := firmanteDeBoveda(t, 7, 1)
	sytue := &clavesDeSytueID{err: store.ErrSinClaveDeBoveda}
	f.st.SetClavesBoveda(sytue)
	leer := func() (matrixproof.Authority, error) {
		return f.st.MatrixStorageAuthority(ctx, f.scope.Subject, f.scope.Product)
	}

	// Encendido: ni un alta, ni un vaciado, ni un traspaso viejo, ni nada dan Matrix.
	f.st.SetCorreoSinMatrix(true)
	for nombre, c := range map[string]clavesDeSytueID{
		"sin clave":      {err: store.ErrSinClaveDeBoveda},
		"alta":           {clave: store.ClaveBoveda{Registered: reg, Motivo: "alta", Creada: time.Now().Add(time.Minute)}},
		"vaciado":        {clave: store.ClaveBoveda{Registered: reg, Motivo: "vaciado", Creada: time.Now().Add(time.Minute)}},
		"traspaso viejo": {clave: store.ClaveBoveda{Registered: reg, Motivo: "traspaso", Creada: time.Now().Add(-time.Hour)}},
	} {
		*sytue = c
		if got, err := leer(); !errors.Is(err, domain.ErrMatrixStorageRetired) || got == matrix {
			t.Fatalf("%s: la autoridad de Matrix no está retirada: %+v, %v", nombre, got, err)
		}
	}
	// Sin Sytue ID no se sabe si ya hubo traspaso: ni Matrix ni «retirada».
	*sytue = clavesDeSytueID{err: errors.New("caído")}
	if _, err := leer(); !errors.Is(err, domain.ErrMatrixUnavailable) {
		t.Fatal("Sytue ID caído con Matrix retirada:", err)
	}
	*sytue = clavesDeSytueID{err: store.ErrSinClaveDeBoveda}

	// Matrix ya no firma, ni cambia de raíz, ni da recibos.
	if _, err := f.st.VerifyMatrixStorageAction(ctx, f.scope.Subject, f.scope.Product, accion, pruebaMatrix); !errors.Is(err, matrixproof.ErrUnverified) {
		t.Fatal("una prueba de Matrix vale con Matrix retirada:", err)
	}
	if err := f.st.VerifyMatrixStorageReceipt(ctx, f.scope.Subject, f.scope.Product, f.target, recibo, verificar); !errors.Is(err, matrixproof.ErrUnverified) || origen != 0 {
		t.Fatal("un recibo de Matrix vale con Matrix retirada:", err, origen)
	}
	for _, kind := range []string{"root.rotate", "root.resume"} {
		if _, err := f.st.MatrixRootChangeChallenge(ctx, f.scope.Subject, f.scope.Product, kind, f.epoch, f.target); !errors.Is(err, domain.ErrMatrixStorageRetired) {
			t.Fatalf("%s con Matrix retirada: %v", kind, err)
		}
	}
	if _, err := f.st.MatrixRootChallenge(ctx, f.scope.Subject, f.scope.Product, f.epoch, f.target); !errors.Is(err, domain.ErrMatrixStorageRetired) {
		t.Fatal("alta de raíz con Matrix retirada:", err)
	}
	if _, err := f.st.ConfirmMatrixRoot(ctx, f.scope.Subject, f.scope.Product, confirmacion); !errors.Is(err, domain.ErrMatrixStorageRetired) {
		t.Fatal("confirmar una raíz con Matrix retirada:", err)
	}

	// Apagado, todo como antes: nada de lo anterior dejó rastro.
	f.st.SetCorreoSinMatrix(false)
	if got, err := leer(); err != nil || got != matrix {
		t.Fatal("apagar el conmutador no devolvió la autoridad de Matrix", got, err)
	}
	if _, err := f.st.VerifyMatrixStorageAction(ctx, f.scope.Subject, f.scope.Product, accion, pruebaMatrix); err != nil {
		t.Fatal("apagado, la prueba de Matrix dejó de valer:", err)
	}
	if err := f.st.VerifyMatrixStorageReceipt(ctx, f.scope.Subject, f.scope.Product, f.target, recibo, verificar); err != nil || origen != 1 {
		t.Fatal("apagado, el recibo de Matrix dejó de valer:", err, origen)
	}
	if _, err := f.st.MatrixRootChangeChallenge(ctx, f.scope.Subject, f.scope.Product, "root.rotate", f.epoch, f.target); err != nil {
		t.Fatal("apagado, Matrix no puede rotar su raíz:", err)
	}

	// Encendido otra vez: el traspaso sigue convirtiendo, contado encima de Matrix.
	f.st.SetCorreoSinMatrix(true)
	*sytue = clavesDeSytueID{clave: store.ClaveBoveda{Registered: reg, Motivo: "traspaso", Creada: time.Now().Add(time.Minute)}}
	boveda, err := leer()
	if err != nil || !vaultproof.IsAuthority(boveda) || boveda.Generation != 2 || boveda.RootKey != vaultproof.RootKey(reg.Public) {
		t.Fatal("con Matrix retirada, el traspaso no dio la autoridad a la bóveda", boveda, err)
	}
	deBoveda := accion
	deBoveda.Generation, deBoveda.RootKey, deBoveda.Nonce = 2, vaultproof.RootKey(reg.Public), uuid.NewString()
	deBoveda.Target = storage.PublicationTarget{RoomID: "!pqv_x:sytue-vault", DeviceID: "pq-x"}
	prueba, err := vaultproof.Sign(firmante, deBoveda)
	if err != nil {
		t.Fatal(err)
	}
	if auth, err := f.st.VerifyMatrixStorageAction(ctx, f.scope.Subject, f.scope.Product, deBoveda, prueba); err != nil || auth.Authority != boveda {
		t.Fatal("la bóveda no firma tras el traspaso con Matrix retirada", auth, err)
	}
	if _, err := f.st.VerifyMatrixStorageAction(ctx, f.scope.Subject, f.scope.Product, accion, pruebaMatrix); !errors.Is(err, matrixproof.ErrUnverified) {
		t.Fatal("una prueba de Matrix vale tras el traspaso:", err)
	}
	// El traspaso quedó escrito: con una clave posterior por continuidad (que no es
	// de traspaso) y el conmutador apagado, el ámbito sigue siendo de la bóveda.
	_, siguiente := firmanteDeBoveda(t, 8, 2)
	*sytue = clavesDeSytueID{clave: store.ClaveBoveda{Registered: siguiente, Motivo: "continuidad", Creada: time.Now().Add(2 * time.Minute)}}
	f.st.SetCorreoSinMatrix(false)
	if got, err := leer(); err != nil || !vaultproof.IsAuthority(got) || got.Generation != 3 {
		t.Fatal("el traspaso con Matrix retirada no quedó escrito", got, err)
	}
}

// Una cuenta nueva sigue yendo a la bóveda (pero ya no puede dar de alta Matrix),
// y la de la bóveda sigue igual.
func TestMatrixRetiradaCuentaNuevaYDeBoveda(t *testing.T) {
	f := newAuthorityFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	sytue := &clavesDeSytueID{err: store.ErrSinClaveDeBoveda}
	f.st.SetClavesBoveda(sytue)
	f.st.SetCorreoSinMatrix(true)
	if _, err := f.st.MatrixStorageAuthority(ctx, f.scope.Subject, f.scope.Product); !errors.Is(err, matrixproof.ErrRootEnrollment) {
		t.Fatal("una cuenta nueva no lee «alta» con Matrix retirada:", err)
	}
	if _, err := f.st.MatrixRootChallenge(ctx, f.scope.Subject, f.scope.Product, f.epoch, f.target); !errors.Is(err, domain.ErrMatrixStorageRetired) {
		t.Fatal("una cuenta nueva da de alta una raíz de Matrix con Matrix retirada:", err)
	}
	_, reg := firmanteDeBoveda(t, 9, 1)
	*sytue = clavesDeSytueID{clave: store.ClaveBoveda{Registered: reg, Motivo: "alta", Creada: time.Now()}}
	got, err := f.st.MatrixStorageAuthority(ctx, f.scope.Subject, f.scope.Product)
	if err != nil || !vaultproof.IsAuthority(got) || got.Generation != 1 {
		t.Fatal("la autoridad de la bóveda cambió con Matrix retirada", got, err)
	}
}

// Calendario no se toca: registra clave de bóveda pero recibe por Matrix.
func TestMatrixRetiradaNoTocaCalendario(t *testing.T) {
	f := newAuthorityFixtureFor(t, "calendar")
	ctx := tenantCtx(f.scope.Tenant)
	_, reg := firmanteDeBoveda(t, 7, 1)
	f.st.SetClavesBoveda(&clavesDeSytueID{clave: store.ClaveBoveda{Registered: reg, Motivo: "alta", Creada: time.Now()}})
	f.st.SetCorreoSinMatrix(true)
	challenge := f.challenge(t)
	if _, err := f.st.ConfirmMatrixRoot(ctx, f.scope.Subject, f.scope.Product, f.sign(t, challenge.Action)); err != nil {
		t.Fatal("calendario no da de alta su raíz de Matrix:", err)
	}
	matrix, err := f.st.MatrixStorageAuthority(ctx, f.scope.Subject, f.scope.Product)
	if err != nil || vaultproof.IsAuthority(matrix) || matrix.Generation != 1 {
		t.Fatal("calendario no lee su autoridad de Matrix", matrix, err)
	}
	accion := challenge.Action
	accion.Kind = "task.claim"
	accion.Nonce = uuid.NewString()
	accion.RequestSHA256, _ = matrixproof.RequestDigest(map[string]string{"task": "calendario"})
	if _, err := f.st.VerifyMatrixStorageAction(ctx, f.scope.Subject, f.scope.Product, accion, f.sign(t, accion)); err != nil {
		t.Fatal("calendario no firma con Matrix:", err)
	}
	if _, err := f.st.MatrixRootChangeChallenge(ctx, f.scope.Subject, f.scope.Product, "root.rotate", f.epoch, f.target); err != nil {
		t.Fatal("calendario no puede rotar su raíz de Matrix:", err)
	}
}

// Las vallas de dispositivo de Matrix del correo se rechazan como tras un traspaso.
func TestMatrixRetiradaRechazaVallasDeMatrix(t *testing.T) {
	f, proof := deviceFenceFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	f.st.SetClavesBoveda(&clavesDeSytueID{err: store.ErrSinClaveDeBoveda})
	f.st.SetCorreoSinMatrix(true)
	if _, err := f.st.PlanMatrixStorageDeviceFence(ctx, proof); !errors.Is(err, matrixproof.ErrUnverified) {
		t.Fatal("una valla de Matrix vale con Matrix retirada:", err)
	}
	f.st.SetCorreoSinMatrix(false)
	if planned, err := f.st.PlanMatrixStorageDeviceFence(ctx, proof); err != nil || planned.Status != "planned" {
		t.Fatal("apagado, la valla de Matrix dejó de valer:", planned, err)
	}
}
