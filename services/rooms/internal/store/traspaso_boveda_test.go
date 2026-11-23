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
)

func TestTraspasoVigenteSoloConMotivoYPosteriorAMatrix(t *testing.T) {
	_, reg, _, _ := cbBoveda(t)
	cambio := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	clave := func(motivo string, creada time.Time) ClaveBoveda {
		return ClaveBoveda{Registered: reg, Motivo: motivo, Creada: creada}
	}
	if !traspasoVigente(clave(motivoTraspaso, cambio.Add(time.Second)), cambio) {
		t.Fatal("un traspaso posterior al último cambio de Matrix no vale")
	}
	for nombre, c := range map[string]ClaveBoveda{
		"anterior a Matrix":   clave(motivoTraspaso, cambio.Add(-time.Second)),
		"a la vez que Matrix": clave(motivoTraspaso, cambio),
		"sin fecha":           clave(motivoTraspaso, time.Time{}),
		"un alta":             clave("alta", cambio.Add(time.Hour)),
		"un vaciado":          clave("vaciado", cambio.Add(time.Hour)),
		"una continuidad":     clave("continuidad", cambio.Add(time.Hour)),
	} {
		if traspasoVigente(c, cambio) {
			t.Errorf("%s pasa por traspaso", nombre)
		}
	}
}

func TestLaGeneracionDeLaBovedaSeCuentaEncimaDeLaDeMatrix(t *testing.T) {
	_, reg, _, a := cbBoveda(t)
	clave := ClaveBoveda{Registered: reg, Motivo: motivoTraspaso}
	got, err := autoridadSobre(a.Scope, clave, 5)
	if err != nil || got.Generation != 6 || !vaultproof.IsAuthority(got) {
		t.Fatalf("sobre Matrix 5: %+v, %v", got, err)
	}
	if got, err := autoridadSobre(a.Scope, clave, 0); err != nil || got.Generation != reg.Generation {
		t.Fatalf("sin Matrix: %+v, %v", got, err)
	}
	for _, base := range []int64{-1, maxBaseBoveda} {
		if _, err := autoridadSobre(a.Scope, clave, base); !errors.Is(err, matrixproof.ErrUnverified) {
			t.Errorf("base %d: %v", base, err)
		}
	}
}

// La prueba de una bóveda solo vale donde la bóveda es la autoridad, y firmada
// para la generación contada encima de la de Matrix.
func TestPruebaDeBovedaSobreUnAmbitoDeMatrix(t *testing.T) {
	s, reg, ctx, esperada := cbBoveda(t)
	st := &Store{clavesBoveda: clavesFijas{reg: reg}}

	// Autoridad de Matrix sin traspaso: la bóveda no vale aunque Sytue ID tenga clave.
	st.baseBoveda = func(context.Context, string, string) (int64, error) { return 0, matrixproof.ErrUnverified }
	prueba, err := vaultproof.Sign(s, esperada)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.VerifyMatrixStorageAction(ctx, cbSujeto, "mail", esperada, prueba); !errors.Is(err, matrixproof.ErrUnverified) {
		t.Fatalf("una bóveda firmó sobre Matrix sin traspaso: %v", err)
	}

	// Con el traspaso hecho sobre Matrix 4: la generación 1 ya no es la suya; la 5 sí.
	st.baseBoveda = func(context.Context, string, string) (int64, error) { return 4, nil }
	if _, err := st.VerifyMatrixStorageAction(ctx, cbSujeto, "mail", esperada, prueba); !errors.Is(err, matrixproof.ErrUnverified) {
		t.Fatalf("la generación de Sytue ID sin contar encima de Matrix: %v", err)
	}
	encima := esperada
	encima.Generation = 5
	prueba, err = vaultproof.Sign(s, encima)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := st.VerifyMatrixStorageAction(ctx, cbSujeto, "mail", encima, prueba)
	if err != nil || auth.Authority.Generation != 5 || auth.Validate(encima) != nil {
		t.Fatalf("tras el traspaso: %+v, %v", auth, err)
	}
}

// clavesLentas es un Sytue ID que tarda: contesta `clave`/`err` pasado `espera`,
// o falla en cuanto vence el plazo de quien pregunta.
type clavesLentas struct {
	espera time.Duration
	clave  ClaveBoveda
	err    error
	veces  int
}

func (c *clavesLentas) Activa(ctx context.Context, _ storage.Scope) (ClaveBoveda, error) {
	c.veces++
	select {
	case <-time.After(c.espera):
		return c.clave, c.err
	case <-ctx.Done():
		return ClaveBoveda{}, ctx.Err()
	}
}

func TestConsultaDeTraspasoNoRetrasaElCorreoDeMatrix(t *testing.T) {
	_, reg, ctx, a := cbBoveda(t)
	scope := a.Scope
	matrix := matrixproof.Authority{Version: 1, Scope: scope, UserID: storage.IsolatedMatrixUserPrefix(scope) + "matrix.test", RootKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", Generation: 4, Status: "active"}
	ahora := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	lento := &clavesLentas{espera: time.Hour}
	st := &Store{clavesBoveda: lento}
	st.traspasoID.ahora = func() time.Time { return ahora }
	st.traspasoID.plazo = 20 * time.Millisecond

	// Sytue ID lento: se contesta Matrix dentro del plazo corto, no en 3 s.
	inicio := time.Now()
	got, err := st.autoridadTraspasada(ctx, cbSujeto, "mail", scope, matrix, estadoTraspaso{})
	if err != nil || got != matrix || time.Since(inicio) > time.Second {
		t.Fatalf("con Sytue ID lento: %+v, %v, %v", got, err, time.Since(inicio))
	}
	// Y durante la pausa no se le vuelve a preguntar: Matrix al momento.
	if got, err := st.autoridadTraspasada(ctx, cbSujeto, "mail", scope, matrix, estadoTraspaso{}); err != nil || got != matrix || lento.veces != 1 {
		t.Fatalf("en pausa se preguntó otra vez: %d veces, %v", lento.veces, err)
	}
	// Pasada la pausa, se pregunta de nuevo; sano y sin traspaso, en cada lectura.
	ahora = ahora.Add(pausaConsultaTraspaso + time.Second)
	sano := &clavesLentas{clave: ClaveBoveda{Registered: reg, Motivo: "alta", Creada: ahora}}
	st.clavesBoveda = sano
	for range 3 {
		if got, err := st.autoridadTraspasada(ctx, cbSujeto, "mail", scope, matrix, estadoTraspaso{}); err != nil || got != matrix {
			t.Fatalf("sano, sin traspaso: %+v, %v", got, err)
		}
	}
	if sano.veces != 3 {
		t.Fatalf("un «no hay traspaso» se recordó: %d consultas", sano.veces)
	}
	// «No tiene clave» es una respuesta, no una avería: no abre la pausa.
	sin := &clavesLentas{err: ErrSinClaveDeBoveda}
	st.clavesBoveda = sin
	for range 2 {
		if _, err := st.traspasoID.consultar(ctx, scope, sin); !errors.Is(err, ErrSinClaveDeBoveda) {
			t.Fatal("sin clave:", err)
		}
	}
	// Si quien se cansa es el que llama, Sytue ID no queda en pausa.
	cancelado, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := st.traspasoID.consultar(cancelado, scope, lento); err == nil {
		t.Fatal("una consulta cancelada salió bien")
	}
	if _, err := st.traspasoID.consultar(ctx, scope, sin); errors.Is(err, errSytueIDEnPausa) {
		t.Fatal("la cancelación de quien llama abrió la pausa")
	}
}
