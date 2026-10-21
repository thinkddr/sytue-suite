// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"context"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// Una solución de ALTCHA se acepta una vez y la segunda no. Antes el INSERT nombraba
// una columna que la migración no creaba y TODA comprobación fallaba: checkout,
// formularios y desistimientos devolvían altcha_invalid.
func TestUnaFirmaDeAltchaValeUnaSolaVez(t *testing.T) {
	dosRoles(t, func(t *testing.T, s *Store, _ *pg.Pool) {
		ctx := context.Background()
		caduca := time.Now().Add(time.Minute)
		if nueva, err := s.RegistrarAltcha(ctx, "firma-1", caduca); err != nil || !nueva {
			t.Fatalf("primera vez: nueva=%v err=%v", nueva, err)
		}
		if nueva, err := s.RegistrarAltcha(ctx, "firma-1", caduca); err != nil || nueva {
			t.Fatalf("repetida: nueva=%v err=%v; quiero rechazada", nueva, err)
		}
	})
}
