// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/autorizacion"
)

// Reclamos son los claims del access token de Sytue ID con que una pantalla decide qué
// pinta. Viven en `nucleo/autorizacion`, que las pantallas importan sin enlazar la sesión.
type Reclamos = autorizacion.Reclamos

// ReclamosDe lee los claims de un JWT (autorizacion.ReclamosDe).
func ReclamosDe(token string) Reclamos { return autorizacion.ReclamosDe(token) }

// Reclamos son los del access token vigente.
func (s *Session) Reclamos(ctx context.Context) (Reclamos, error) {
	token, err := s.AccessToken(ctx)
	if err != nil {
		return Reclamos{}, err
	}
	return ReclamosDe(token), nil
}

// EsCuentaPersonal dice, SIN red, si la sesión es de una cuenta personal: lo lee del
// access token que hay guardado, aunque esté caducado (la clase de cuenta no cambia al
// renovarlo). Sin sesión, no. Es para lo que se pinta en el acto, como el menú de cuenta.
func (s *Session) EsCuentaPersonal() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ReclamosDe(s.tokens.Access).EsCuentaPersonal()
}
