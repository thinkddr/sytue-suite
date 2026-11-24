// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// La retirada de Matrix del correo (ADR 0087, entrega 5): solo `mail`, y solo con
// el conmutador.
func TestMatrixRetiradaSoloDelCorreoYConElConmutador(t *testing.T) {
	st := &Store{}
	for _, p := range []string{"mail", "calendar", "drive"} {
		if st.matrixRetirada(p) {
			t.Fatalf("sin el conmutador, %s retiró Matrix", p)
		}
	}
	st.SetCorreoSinMatrix(true)
	if !st.matrixRetirada("mail") {
		t.Fatal("con el conmutador, el correo sigue con Matrix")
	}
	for _, p := range []string{"calendar", "drive", "notes"} {
		if st.matrixRetirada(p) {
			t.Fatalf("el conmutador del correo retiró Matrix de %s", p)
		}
	}
}

// La lectura de la autoridad de un ámbito de Matrix sin traspaso, con Matrix
// retirada: `retirada` en vez de Matrix; la bóveda si ya hubo traspaso; y sin
// Sytue ID, ninguna respuesta (ni Matrix ni «pasa a la bóveda»).
func TestLecturaDeLaAutoridadConMatrixRetirada(t *testing.T) {
	_, reg, ctx, a := cbBoveda(t)
	scope := a.Scope
	matrix := matrixproof.Authority{Version: 1, Scope: scope, UserID: storage.IsolatedMatrixUserPrefix(scope) + "matrix.test", RootKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", Generation: 4, Status: "active"}
	ultimo := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	sinTraspaso := estadoTraspaso{ultimo: ultimo}
	casos := map[string]ClavesBoveda{
		"sin clave":      clavesFijas{err: ErrSinClaveDeBoveda},
		"un alta":        clavesFijas{reg: reg, motivo: "alta", creada: ultimo.Add(time.Hour)},
		"un vaciado":     clavesFijas{reg: reg, motivo: "vaciado", creada: ultimo.Add(time.Hour)},
		"traspaso viejo": clavesFijas{reg: reg, motivo: motivoTraspaso, creada: ultimo.Add(-time.Hour)},
	}
	for nombre, claves := range casos {
		// Apagado, lo de siempre: Matrix.
		st := &Store{clavesBoveda: claves}
		if got, err := st.autoridadTraspasada(ctx, cbSujeto, "mail", scope, matrix, sinTraspaso); err != nil || got != matrix {
			t.Fatalf("%s, sin el conmutador: %+v, %v", nombre, got, err)
		}
		// Encendido: retirada, y nunca la de Matrix.
		st.SetCorreoSinMatrix(true)
		if got, err := st.autoridadTraspasada(ctx, cbSujeto, "mail", scope, matrix, sinTraspaso); !errors.Is(err, domain.ErrMatrixStorageRetired) || got == matrix {
			t.Fatalf("%s, con Matrix retirada: %+v, %v", nombre, got, err)
		}
	}

	// Sytue ID caído: sin el conmutador, Matrix (y la pausa); con él, ninguna
	// respuesta, y se le pregunta cada vez (sin la pausa de la consulta corta).
	caido := &clavesLentas{err: errors.New("sytue id caído")}
	st := &Store{clavesBoveda: caido}
	st.SetCorreoSinMatrix(true)
	for range 2 {
		if got, err := st.autoridadTraspasada(ctx, cbSujeto, "mail", scope, matrix, sinTraspaso); !errors.Is(err, domain.ErrMatrixUnavailable) || got == matrix {
			t.Fatalf("Sytue ID caído con Matrix retirada: %+v, %v", got, err)
		}
	}
	if caido.veces != 2 {
		t.Fatalf("con Matrix retirada, Sytue ID caído quedó en pausa: %d consultas", caido.veces)
	}

	// Con el traspaso ya escrito, la de la bóveda, como siempre.
	st = &Store{clavesBoveda: clavesFijas{reg: reg, motivo: "continuidad", creada: ultimo.Add(time.Hour)}}
	st.SetCorreoSinMatrix(true)
	got, err := st.autoridadTraspasada(ctx, cbSujeto, "mail", scope, matrix, estadoTraspaso{hecho: true, base: 4, ultimo: ultimo})
	if err != nil || !vaultproof.IsAuthority(got) || got.Generation != 5 {
		t.Fatalf("tras el traspaso, con Matrix retirada: %+v, %v", got, err)
	}
}

// Con Matrix retirada no entra ni cambia una raíz de Matrix del correo, y
// ninguna prueba de Matrix vale; calendario y el conmutador apagado, como antes.
func TestRaicesYPruebasDeMatrixConMatrixRetirada(t *testing.T) {
	_, _, ctx, a := cbBoveda(t)
	epoch := "11111111-1111-4111-8111-111111111111"
	target := storage.PublicationTarget{RoomID: "!privado:matrix.test", DeviceID: "OWNER_DEVICE"}
	deMatrix := a
	deMatrix.Kind, deMatrix.Target = "task.claim", target
	prueba := matrixproof.SignedAction{Action: deMatrix}

	apagado, encendido := &Store{}, &Store{}
	encendido.SetCorreoSinMatrix(true)

	// Alta y confirmación de cualquier raíz (ConfirmMatrixRoot) pasan por aquí.
	if err := encendido.raizMatrixPermitida(ctx, cbSujeto, "mail"); !errors.Is(err, domain.ErrMatrixStorageRetired) {
		t.Fatalf("alta de una raíz de Matrix del correo retirado: %v", err)
	}
	for _, c := range []struct {
		st      *Store
		product string
	}{{apagado, "mail"}, {encendido, "calendar"}} {
		if err := c.st.raizMatrixPermitida(ctx, cbSujeto, c.product); err != nil {
			t.Fatalf("%s (retirada %v): %v", c.product, c.st.correoSinMatrix, err)
		}
	}

	// Rotar o reanudar: rechazado antes de mirar Matrix. Sin el conmutador, o en
	// calendario, se sigue (y aquí, sin Matrix configurado, falla más adelante).
	for _, kind := range []string{"root.rotate", "root.resume"} {
		if _, err := encendido.MatrixRootChangeChallenge(ctx, cbSujeto, "mail", kind, epoch, target); !errors.Is(err, domain.ErrMatrixStorageRetired) {
			t.Fatalf("%s del correo retirado: %v", kind, err)
		}
		if _, err := apagado.MatrixRootChangeChallenge(ctx, cbSujeto, "mail", kind, epoch, target); !errors.Is(err, domain.ErrMatrixUnavailable) {
			t.Fatalf("%s sin el conmutador: %v", kind, err)
		}
		if _, err := encendido.MatrixRootChangeChallenge(ctx, cbSujeto, "calendar", kind, epoch, target); !errors.Is(err, domain.ErrMatrixUnavailable) {
			t.Fatalf("%s de calendario: %v", kind, err)
		}
	}

	// Una acción firmada por Matrix: rechazada como tras un traspaso.
	if _, err := encendido.VerifyMatrixStorageAction(ctx, cbSujeto, "mail", deMatrix, prueba); !errors.Is(err, matrixproof.ErrUnverified) {
		t.Fatalf("acción de Matrix del correo retirado: %v", err)
	}
	if _, err := apagado.VerifyMatrixStorageAction(ctx, cbSujeto, "mail", deMatrix, prueba); !errors.Is(err, domain.ErrMatrixUnavailable) {
		t.Fatalf("acción de Matrix sin el conmutador: %v", err)
	}
	if _, err := encendido.VerifyMatrixStorageAction(ctx, cbSujeto, "calendar", deMatrix, prueba); !errors.Is(err, domain.ErrMatrixUnavailable) {
		t.Fatalf("acción de Matrix de calendario: %v", err)
	}

	// El recibo de una publicación de Matrix, igual.
	recibo := storage.Reference{RoomID: target.RoomID, EventID: "$cifrado"}
	verificar := func(context.Context, storage.Scope, storage.Reference) error {
		t.Fatal("el recibo de Matrix del correo retirado llegó al origen")
		return nil
	}
	if err := encendido.VerifyMatrixStorageReceipt(ctx, cbSujeto, "mail", target, recibo, verificar); !errors.Is(err, matrixproof.ErrUnverified) {
		t.Fatalf("recibo de Matrix del correo retirado: %v", err)
	}
}

// Una prueba de la bóveda no pasa por la retirada: sigue yendo por Sytue ID.
func TestPruebaDeBovedaConMatrixRetirada(t *testing.T) {
	s, reg, ctx, esperada := cbBoveda(t)
	st := &Store{clavesBoveda: clavesFijas{reg: reg}}
	st.SetCorreoSinMatrix(true)
	st.baseBoveda = func(context.Context, string, string) (int64, error) { return 0, nil }
	prueba, err := vaultproof.Sign(s, esperada)
	if err != nil {
		t.Fatal(err)
	}
	if auth, err := st.VerifyMatrixStorageAction(ctx, cbSujeto, "mail", esperada, prueba); err != nil || auth.Validate(esperada) != nil {
		t.Fatalf("la bóveda dejó de valer con Matrix retirada: %+v, %v", auth, err)
	}
}
