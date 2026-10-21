// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPublicUnlockRateLimited comprueba que el desbloqueo por contraseña (única
// superficie sin auth, que corre Argon2id por intento) queda limitado por token:
// tras agotar la ráfaga del bucket, los intentos extra reciben 429 ANTES de tocar
// Argon2id — cierra el DoS por memoria y la fuerza bruta online.
func TestPublicUnlockRateLimited(t *testing.T) {
	h := newHandlers(&fakeSvc{}) // PublicVerify devuelve ("", nil) ⇒ 303 mientras haya cupo
	const token = "tok-dos-ratelimit-unique"

	got429 := false
	// El bucket arranca con burst=10; la 11ª (y siguientes, dentro del mismo segundo)
	// deben caer en 429 sin llamar a PublicVerify.
	for i := 0; i < 14; i++ {
		req := mkReq(http.MethodPost, "/public/"+token, "password=x", map[string]string{"token": token})
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.PublicUnlock(rec, req)
		if i < 10 && rec.Code == http.StatusTooManyRequests {
			t.Fatalf("intento %d limitado antes de agotar la ráfaga (code=%d)", i, rec.Code)
		}
		if rec.Code == http.StatusTooManyRequests {
			got429 = true
		}
	}
	if !got429 {
		t.Fatal("el desbloqueo no se limitó tras la ráfaga: sin 429 (DoS/fuerza bruta abierto)")
	}
}
