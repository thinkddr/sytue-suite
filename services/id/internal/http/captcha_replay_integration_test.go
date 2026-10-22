// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// ID-01: una solución de captcha ALTCHA no se puede reusar entre réplicas. El
// verificador recordaba las soluciones usadas en la memoria del proceso, así que
// con 2-12 réplicas en producción una misma solución valía una vez por réplica.
// El arreglo cablea un almacén COMPARTIDO (store.RegistrarCaptchaResuelto, tabla
// captcha_used) con altcha.ConRepeticiones.
package http

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/altcha"
)

func TestCaptchaNoSeReusaEntreReplicas(t *testing.T) {
	l := newLiveServer(t)
	clave := deriveCaptchaKey(nil) // misma clave en todas las réplicas (deriva de la KEK)

	// Dos verificadores = dos réplicas, con el MISMO almacén compartido (la base).
	rep1 := altcha.New(clave, altcha.ConRepeticiones(l.st.RegistrarCaptchaResuelto))
	rep2 := altcha.New(clave, altcha.ConRepeticiones(l.st.RegistrarCaptchaResuelto))

	reto, err := rep1.CreateChallenge(1000, time.Minute)
	if err != nil {
		t.Fatalf("CreateChallenge: %v", err)
	}
	solucion, err := altcha.SolveChallenge(reto)
	if err != nil {
		t.Fatalf("SolveChallenge: %v", err)
	}

	// La réplica 1 la acepta una vez.
	if ok, err := rep1.Verify(l.ctx, solucion); err != nil || !ok {
		t.Fatalf("la primera verificación debe pasar: ok=%v err=%v", ok, err)
	}
	// La réplica 2, con el almacén compartido, la RECHAZA como repetida.
	if ok, err := rep2.Verify(l.ctx, solucion); ok || err == nil {
		t.Fatalf("ID-01: la misma solución se reusó en otra réplica (ok=%v err=%v)", ok, err)
	}
	// Y la propia réplica 1 tampoco la vuelve a aceptar.
	if ok, _ := rep1.Verify(l.ctx, solucion); ok {
		t.Fatal("la solución no debe aceptarse dos veces en la misma réplica")
	}

	// Contraste: con la memoria de proceso (lo de antes), dos réplicas distintas
	// aceptarían CADA UNA la misma solución — justo lo que el almacén compartido
	// cierra. Se deja como documentación del fallo que se corrige.
	mem1 := altcha.New(clave)
	mem2 := altcha.New(clave)
	reto2, _ := mem1.CreateChallenge(1000, time.Minute)
	sol2, _ := altcha.SolveChallenge(reto2)
	if ok, _ := mem1.Verify(l.ctx, sol2); !ok {
		t.Fatal("control: mem1 debería aceptar")
	}
	if ok, _ := mem2.Verify(l.ctx, sol2); !ok {
		t.Fatal("control: con memoria de proceso la otra réplica también acepta (el fallo que arregla captcha_used)")
	}
}
