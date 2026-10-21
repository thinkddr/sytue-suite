// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"testing"

	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// El aviso sale UNA vez, en la subida que cruza el 90 %, y no en cada una de después.
func TestLaCuotaAvisaSoloAlCruzarElUmbral(t *testing.T) {
	for _, c := range []struct {
		usado, sumados int64
		quiero         bool
	}{
		{usado: 89, sumados: 10, quiero: false}, // se queda por debajo
		{usado: 90, sumados: 5, quiero: true},   // 85 → 90: cruza
		{usado: 95, sumados: 1, quiero: false},  // ya estaba por encima
		{usado: 100, sumados: 100, quiero: true},
		{usado: 95, sumados: 0, quiero: false},
	} {
		if got := cruzaUmbral(domain.Quota{LimitBytes: 100, UsedBytes: c.usado}, c.sumados); got != c.quiero {
			t.Errorf("usado %d tras subir %d: avisa=%v, quiero %v", c.usado, c.sumados, got, c.quiero)
		}
	}
	if cruzaUmbral(domain.Quota{LimitBytes: 0, UsedBytes: 10}, 10) {
		t.Error("sin límite no hay umbral que cruzar")
	}
}
