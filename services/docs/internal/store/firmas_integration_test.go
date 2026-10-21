// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

func preparadaDePrueba(sujeto string, caduca time.Time) store.FirmaPreparada {
	return store.FirmaPreparada{
		Sujeto: sujeto, Caduca: caduca, Huella: bytes.Repeat([]byte{1}, 32),
		HuecoInicio: 100, HuecoFin: 200, Atributos: []byte{0x31, 0}, Certificados: []byte{0x30, 0},
	}
}

// TestUnaFirmaPreparadaSoloLaCompletaQuienLaPreparoYUnaVez es el check de la
// ligadura en la base: otra persona, otra organización, una segunda vuelta a la
// vez y una vuelta después de gastarla contestan lo mismo, «no está».
func TestUnaFirmaPreparadaSoloLaCompletaQuienLaPreparoYUnaVez(t *testing.T) {
	st := newStore(t)
	acme, globex := tenantCtx("acme"), tenantCtx("globex")
	id, err := st.GuardarFirmaPreparada(acme, preparadaDePrueba("ana", time.Now().Add(10*time.Minute)), 5)
	if err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	f, err := st.TomarFirmaPreparada(acme, id, "ana", time.Minute)
	if err != nil || f.HuecoInicio != 100 || f.HuecoFin != 200 || len(f.Huella) != 32 {
		t.Fatalf("Tomar: %+v %v", f, err)
	}
	if _, err := st.TomarFirmaPreparada(acme, id, "ana", time.Minute); !errors.Is(err, store.ErrFirmaNoDisponible) {
		t.Errorf("dos vueltas a la vez: err = %v", err)
	}
	if err := st.SoltarFirmaPreparada(acme, id, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.TomarFirmaPreparada(acme, id, "ana", time.Minute); !errors.Is(err, store.ErrFirmaNoDisponible) {
		t.Errorf("soltar con OTRA reserva no puede liberarla: err = %v", err)
	}
	if err := st.SoltarFirmaPreparada(acme, id, f.Reserva); err != nil {
		t.Fatal(err)
	}
	if _, err := st.TomarFirmaPreparada(acme, id, "berta", time.Minute); !errors.Is(err, store.ErrFirmaNoDisponible) {
		t.Errorf("otra persona: err = %v", err)
	}
	if _, err := st.TomarFirmaPreparada(globex, id, "ana", time.Minute); !errors.Is(err, store.ErrFirmaNoDisponible) {
		t.Errorf("otra organización, mismo sujeto: err = %v", err)
	}
	f, err = st.TomarFirmaPreparada(acme, id, "ana", time.Minute)
	if err != nil {
		t.Fatalf("tras soltarla, se retoma: %v", err)
	}
	if err := st.GastarFirmaPreparada(acme, id); err != nil {
		t.Fatal(err)
	}
	if err := st.SoltarFirmaPreparada(acme, id, f.Reserva); err != nil {
		t.Fatal(err)
	}
	if _, err := st.TomarFirmaPreparada(acme, id, "ana", time.Minute); !errors.Is(err, store.ErrFirmaNoDisponible) {
		t.Errorf("gastada: err = %v", err)
	}
}

func TestLasFirmasPreparadasCaducanYSePurgan(t *testing.T) {
	st := newStore(t)
	acme, globex := tenantCtx("acme"), tenantCtx("globex")
	vieja, err := st.GuardarFirmaPreparada(acme, preparadaDePrueba("ana", time.Now().Add(-time.Second)), 5)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.TomarFirmaPreparada(acme, vieja, "ana", time.Minute); !errors.Is(err, store.ErrFirmaNoDisponible) {
		t.Errorf("caducada: err = %v", err)
	}
	if _, err := st.GuardarFirmaPreparada(globex, preparadaDePrueba("otro", time.Now().Add(-time.Second)), 5); err != nil {
		t.Fatal(err)
	}
	viva, err := st.GuardarFirmaPreparada(globex, preparadaDePrueba("otro", time.Now().Add(time.Hour)), 5)
	if err != nil {
		t.Fatal(err)
	}
	// La caducada de globex la barrió su propio Guardar; la de acme sigue ahí,
	// porque cada organización solo barre las suyas. La purga es la que cruza.
	n, err := st.PurgarFirmasCaducadas(acme)
	if err != nil {
		t.Fatalf("Purgar: %v", err)
	}
	if n < 1 {
		t.Errorf("la purga no borró ninguna caducada (%d): ¿ve las filas la transacción de mantenimiento?", n)
	}
	if _, err := st.TomarFirmaPreparada(globex, viva, "otro", time.Minute); err != nil {
		t.Errorf("la purga se ha llevado una viva: %v", err)
	}
}

func TestNadieAbreFirmasPreparadasSinFin(t *testing.T) {
	st := newStore(t)
	acme := tenantCtx("acme")
	for range 2 {
		if _, err := st.GuardarFirmaPreparada(acme, preparadaDePrueba("ana", time.Now().Add(time.Hour)), 2); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.GuardarFirmaPreparada(acme, preparadaDePrueba("ana", time.Now().Add(time.Hour)), 2); !errors.Is(err, store.ErrDemasiadasFirmas) {
		t.Errorf("la tercera con tope 2: err = %v", err)
	}
	if _, err := st.GuardarFirmaPreparada(acme, preparadaDePrueba("berta", time.Now().Add(time.Hour)), 2); err != nil {
		t.Errorf("el tope es por persona: %v", err)
	}
}
