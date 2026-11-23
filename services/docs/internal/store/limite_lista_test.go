// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"math"
	"testing"
)

// El tamaño de página lo pide el cliente: valores hostiles (enormes o negativos)
// caen al de por defecto y nunca dimensionan una reserva.
func TestLimiteListaAcotaLoQuePideElCliente(t *testing.T) {
	casos := map[int]int{
		math.MaxInt: limiteListaPorDefecto,
		math.MinInt: limiteListaPorDefecto,
		1 << 40:     limiteListaPorDefecto,
		201:         limiteListaPorDefecto,
		0:           limiteListaPorDefecto,
		-1:          limiteListaPorDefecto,
		1:           1,
		200:         200,
	}
	for pedido, quiero := range casos {
		if got := limiteLista(pedido); got != quiero {
			t.Errorf("limiteLista(%d) = %d; quería %d", pedido, got, quiero)
		}
	}
}
